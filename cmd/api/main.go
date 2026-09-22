// Command api is the Plotting Society HTTP server.
//
// One binary, no sidecars: it applies its own migrations at boot and serves
// every route. That is what makes it deployable to a free scale-to-zero
// container platform and equally to a single small VM.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/config"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/fund"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
	"github.com/jason-bourne-gg/plotting-society/internal/lead"
	"github.com/jason-bourne-gg/plotting-society/internal/media"
	"github.com/jason-bourne-gg/plotting-society/internal/plot"
	"github.com/jason-bourne-gg/plotting-society/internal/query"
	"github.com/jason-bourne-gg/plotting-society/internal/update"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logLevel := slog.LevelDebug
	if cfg.IsProduction() {
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}

	tokens := auth.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTTL)
	authenticator := auth.NewAuthenticator(tokens)

	mux := http.NewServeMux()

	// Health check: Cloud Run and any uptime monitor hit this. It touches the
	// database so a wedged pool surfaces as unhealthy rather than as 200s.
	mux.Handle("GET /healthz", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := db.Ping(pingCtx); err != nil {
			return httpx.Internal(err)
		}
		return httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))

	// One guard answers "may this caller act on this record?" for every
	// staff-only route. RequireStaff proves the caller is a builder; the guard
	// proves they are *this* builder.
	guard := access.NewGuard(db)

	authStore := auth.NewStore(db)
	auth.NewHandler(authStore, tokens, guard, cfg.PublicBaseURL+"/invite").Routes(mux, authenticator)

	plotHandler := plot.NewHandler(plot.NewStore(db), guard)
	plotHandler.Routes(mux, authenticator)
	plotHandler.SocietyRoutes(mux, authenticator)

	query.NewHandler(query.NewStore(db), guard).Routes(mux, authenticator)
	// Guest surface: public society view plus the enquiry form.
	lead.NewHandler(lead.NewStore(db), guard).Routes(mux, authenticator)
	fund.NewHandler(fund.NewStore(db), guard).Routes(mux, authenticator)
	update.NewHandler(update.NewStore(db), guard).Routes(mux, authenticator)
	media.NewHandler(media.NewSigner(cfg.S3)).Routes(mux, authenticator)

	handler := httpx.Chain(mux,
		httpx.Recoverer,
		httpx.RequestLogger,
		httpx.SecurityHeaders,
		httpx.CORS(cfg.CORSOrigins),
	)

	// Cloud Run injects PORT; honour it over HTTP_ADDR when present.
	addr := cfg.HTTPAddr
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr, "env", cfg.Env)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	// Let in-flight requests finish before the container goes away.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
