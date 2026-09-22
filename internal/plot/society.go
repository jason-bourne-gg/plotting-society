package plot

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
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

// Societies returns only what the caller is entitled to see: a super admin
// sees everything, staff see their own builder's projects, and an owner sees
// the societies they actually hold a plot in. Returning the full list would
// leak every builder's project names to any signed-in user.
func (s *Store) Societies(ctx context.Context, role domain.Role, builderID *uuid.UUID, userID uuid.UUID) ([]Society, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+societyColumns+`
		  FROM societies s JOIN builders b ON b.id = s.builder_id
		 WHERE $1 = true
		    OR ($2::uuid IS NOT NULL AND s.builder_id = $2)
		    OR EXISTS (SELECT 1 FROM plots p WHERE p.society_id = s.id AND p.owner_id = $3)
		 ORDER BY s.name`,
		role == domain.RoleSuperAdmin, builderID, userID)
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


// SocietyRoutes registers the authenticated society listing.
//
// There is deliberately no /api/societies/<something>/{slug} route here: any
// such pattern overlaps GET /api/societies/{societyId}/plots, and Go's ServeMux
// panics at registration when two patterns overlap with neither more specific.
// Slug lookup for guests lives at /api/public/society/{slug} instead.
func (h *Handler) SocietyRoutes(mux *http.ServeMux, a *auth.Authenticator) {
	mux.Handle("GET /api/societies", a.RequireAuth(httpx.Handler(h.listSocieties)))
}

func (h *Handler) listSocieties(w http.ResponseWriter, r *http.Request) error {
	caller := auth.MustFromContext(r.Context())
	list, err := h.store.Societies(r.Context(), caller.Role, caller.BuilderID, caller.UserID)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"societies": list})
}

