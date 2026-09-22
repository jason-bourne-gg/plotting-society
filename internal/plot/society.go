package plot

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

type Society struct {
	ID             uuid.UUID `json:"id"`
	BuilderID      uuid.UUID `json:"builderId"`
	BuilderName    string    `json:"builderName"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	City           string    `json:"city,omitempty"`
	Address        string    `json:"address,omitempty"`
	LayoutImageURL string    `json:"layoutImageUrl,omitempty"`
	LayoutWidth    *int      `json:"layoutWidth,omitempty"`
	LayoutHeight   *int      `json:"layoutHeight,omitempty"`
	RERANumber     string    `json:"reraNumber,omitempty"`
}

const societyColumns = `
	s.id, s.builder_id, COALESCE(b.name,''), s.name, s.slug, COALESCE(s.city,''),
	COALESCE(s.address,''), COALESCE(s.layout_image_url,''), s.layout_width,
	s.layout_height, COALESCE(s.rera_number,'')`

func (s *Store) Societies(ctx context.Context) ([]Society, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+societyColumns+`
		  FROM societies s JOIN builders b ON b.id = s.builder_id
		 ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Society{}
	for rows.Next() {
		var soc Society
		if err := rows.Scan(&soc.ID, &soc.BuilderID, &soc.BuilderName, &soc.Name, &soc.Slug,
			&soc.City, &soc.Address, &soc.LayoutImageURL, &soc.LayoutWidth,
			&soc.LayoutHeight, &soc.RERANumber); err != nil {
			return nil, err
		}
		out = append(out, soc)
	}
	return out, rows.Err()
}

func (s *Store) SocietyBySlug(ctx context.Context, slug string) (Society, error) {
	var soc Society
	err := s.db.QueryRow(ctx, `
		SELECT `+societyColumns+`
		  FROM societies s JOIN builders b ON b.id = s.builder_id
		 WHERE s.slug = $1`, slug,
	).Scan(&soc.ID, &soc.BuilderID, &soc.BuilderName, &soc.Name, &soc.Slug, &soc.City,
		&soc.Address, &soc.LayoutImageURL, &soc.LayoutWidth, &soc.LayoutHeight, &soc.RERANumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return Society{}, ErrNotFound
	}
	return soc, err
}

func (h *Handler) SocietyRoutes(mux *http.ServeMux, a *auth.Authenticator) {
	mux.Handle("GET /api/societies", a.RequireAuth(httpx.Handler(h.listSocieties)))
	mux.Handle("GET /api/societies/by-slug/{slug}", a.Optional(httpx.Handler(h.societyBySlug)))
}

func (h *Handler) listSocieties(w http.ResponseWriter, r *http.Request) error {
	list, err := h.store.Societies(r.Context())
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"societies": list})
}

func (h *Handler) societyBySlug(w http.ResponseWriter, r *http.Request) error {
	soc, err := h.store.SocietyBySlug(r.Context(), r.PathValue("slug"))
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("No society with that address.")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, soc)
}
