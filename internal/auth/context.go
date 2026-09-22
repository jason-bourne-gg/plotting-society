package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

type ctxKey struct{}

// Identity is the authenticated caller, derived from the access token.
type Identity struct {
	UserID    uuid.UUID
	BuilderID *uuid.UUID
	Role      domain.Role
	Name      string
}

// FromContext returns the caller. The second result is false on public routes.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// MustFromContext is for handlers already behind RequireAuth.
func MustFromContext(ctx context.Context) Identity {
	id, ok := FromContext(ctx)
	if !ok {
		panic("auth.MustFromContext called on an unauthenticated route")
	}
	return id
}

// Authenticator builds the middleware that reads and verifies bearer tokens.
type Authenticator struct {
	tokens *TokenIssuer
}

func NewAuthenticator(tokens *TokenIssuer) *Authenticator {
	return &Authenticator{tokens: tokens}
}

// RequireAuth rejects anything without a valid, unexpired access token.
func (a *Authenticator) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := a.identify(r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, identity)))
	})
}

// Optional attaches the caller when a token is present but never rejects.
// Used by the plot map, which shows more detail to a signed-in owner.
func (a *Authenticator) Optional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if identity, err := a.identify(r); err == nil {
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, identity))
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole gates a route on a predicate over the caller's role, so the rule
// lives in domain.Role rather than being spelled out at each call site.
func RequireRole(allow func(domain.Role) bool) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := FromContext(r.Context())
			if !ok {
				httpx.WriteError(w, r, httpx.Unauthorized("Sign in to continue."))
				return
			}
			if !allow(identity.Role) {
				httpx.WriteError(w, r, httpx.Forbidden("Your account cannot perform this action."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireStaff and RequireBuilderAdmin are the two gates used in practice.
func RequireStaff() httpx.Middleware {
	return RequireRole(func(r domain.Role) bool { return r.CanManageSociety() })
}

func RequireBuilderAdmin() httpx.Middleware {
	return RequireRole(func(r domain.Role) bool { return r.CanManageBuilder() })
}

func (a *Authenticator) identify(r *http.Request) (Identity, error) {
	header := r.Header.Get("Authorization")
	raw, found := strings.CutPrefix(header, "Bearer ")
	if !found || strings.TrimSpace(raw) == "" {
		return Identity{}, httpx.Unauthorized("Sign in to continue.")
	}

	claims, err := a.tokens.ParseAccess(strings.TrimSpace(raw))
	if err != nil {
		return Identity{}, httpx.Unauthorized("Your session has expired. Sign in again.")
	}

	userID, err := parseUUID(claims.Subject)
	if err != nil {
		return Identity{}, httpx.Unauthorized("Malformed session token.")
	}

	identity := Identity{UserID: userID, Role: claims.Role, Name: claims.Name}
	if claims.BuilderID != "" {
		if bid, err := parseUUID(claims.BuilderID); err == nil {
			identity.BuilderID = &bid
		}
	}
	if !identity.Role.Valid() {
		return Identity{}, httpx.Unauthorized("Malformed session token.")
	}
	return identity, nil
}
