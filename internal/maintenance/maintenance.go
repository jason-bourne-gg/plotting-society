// Package maintenance owns the one-time maintenance charge: the per-sector
// rates the site office sets, and the bills raised from them.
//
// The charge is unitary — a plot pays its own area at its sector's rate — so a
// 1,540 sq ft plot at Rs 11.04/sq ft owes Rs 17,002 and a 3,422 sq ft plot owes
// Rs 37,779. Nothing is a flat fee, because plot areas here range from
// 1,130 to 4,035 sq ft and a flat fee would be visibly unfair at both ends.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

var ErrNotFound = errors.New("not found")

// ReferenceAreaSqft is the plot size the site office quotes rates against.
// Showing "1,540 sq ft → ₹17,002" next to a rate is how someone checks at a
// glance that they typed the right number.
const ReferenceAreaSqft = 1540.0

// Rate is one sector's charge. A nil Sector is the society-wide fallback.
type Rate struct {
	ID          uuid.UUID `json:"id"`
	Sector      *string   `json:"sector"`
	RatePerSqft float64   `json:"ratePerSqft"`
	// ReferenceAmount is the rate applied to ReferenceAreaSqft, precomputed so
	// every client shows the same figure.
	ReferenceAmount float64   `json:"referenceAmount"`
	PlotCount       int       `json:"plotCount"`
	BilledCount     int       `json:"billedCount"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

// Round to paise. Money is never left to float drift.
func amountFor(area, rate float64) float64 {
	return math.Round(area*rate*100) / 100
}

// List returns every sector in the society with the rate that applies to it,
// including sectors that have no rate of their own and fall back to the
// default — otherwise the site office cannot see what an unset sector charges.
func (s *Store) List(ctx context.Context, societyID uuid.UUID) ([]Rate, float64, error) {
	var fallback float64
	err := s.db.QueryRow(ctx, `
		SELECT rate_per_sqft FROM maintenance_rates
		 WHERE society_id = $1 AND sector IS NULL`, societyID).Scan(&fallback)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT p.phase,
		       COALESCE(r.rate_per_sqft, $2) AS rate,
		       COALESCE(r.id, '00000000-0000-0000-0000-000000000000'::uuid),
		       COALESCE(r.updated_at, now()),
		       count(*)::int AS plots,
		       count(d.id)::int AS billed
		  FROM plots p
		  LEFT JOIN maintenance_rates r
		         ON r.society_id = p.society_id AND r.sector = p.phase
		  LEFT JOIN maintenance_dues d ON d.plot_id = p.id
		 WHERE p.society_id = $1 AND p.phase IS NOT NULL
		 GROUP BY p.phase, r.rate_per_sqft, r.id, r.updated_at
		 ORDER BY p.phase`, societyID, fallback)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Rate{}
	for rows.Next() {
		var r Rate
		var sector string
		if err := rows.Scan(&sector, &r.RatePerSqft, &r.ID, &r.UpdatedAt,
			&r.PlotCount, &r.BilledCount); err != nil {
			return nil, 0, err
		}
		r.Sector = &sector
		r.ReferenceAmount = amountFor(ReferenceAreaSqft, r.RatePerSqft)
		out = append(out, r)
	}
	return out, fallback, rows.Err()
}

// SetRate upserts one sector's rate, or the fallback when sector is empty.
func (s *Store) SetRate(ctx context.Context, societyID uuid.UUID, sector string, rate float64, by uuid.UUID) error {
	if sector == "" {
		_, err := s.db.Exec(ctx, `
			INSERT INTO maintenance_rates (society_id, sector, rate_per_sqft, updated_by)
			VALUES ($1, NULL, $2, $3)
			ON CONFLICT (society_id) WHERE sector IS NULL
			DO UPDATE SET rate_per_sqft = EXCLUDED.rate_per_sqft,
			              updated_by = EXCLUDED.updated_by, updated_at = now()`,
			societyID, rate, by)
		return err
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO maintenance_rates (society_id, sector, rate_per_sqft, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (society_id, sector) WHERE sector IS NOT NULL
		DO UPDATE SET rate_per_sqft = EXCLUDED.rate_per_sqft,
		              updated_by = EXCLUDED.updated_by, updated_at = now()`,
		societyID, sector, rate, by)
	return err
}

// GenerateBills raises a one-time bill for every sold plot that has none, at
// whatever rate its sector currently carries.
//
// Plots already billed are skipped rather than re-priced: a bill that has been
// sent, and possibly paid, must not silently change because the rate moved.
func (s *Store) GenerateBills(ctx context.Context, societyID uuid.UUID) (raised int, total float64, err error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT p.id, p.area_sqft, p.phase,
		       COALESCE(r.rate_per_sqft, d.rate_per_sqft)
		  FROM plots p
		  LEFT JOIN maintenance_rates r
		         ON r.society_id = p.society_id AND r.sector = p.phase
		  LEFT JOIN maintenance_rates d
		         ON d.society_id = p.society_id AND d.sector IS NULL
		 WHERE p.society_id = $1
		   AND p.status = 'sold'
		   AND p.area_sqft IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM maintenance_dues m WHERE m.plot_id = p.id)`,
		societyID)
	if err != nil {
		return 0, 0, err
	}

	type bill struct {
		plotID uuid.UUID
		area   float64
		sector *string
		rate   float64
	}
	var bills []bill
	for rows.Next() {
		var b bill
		if err := rows.Scan(&b.plotID, &b.area, &b.sector, &b.rate); err != nil {
			rows.Close()
			return 0, 0, err
		}
		bills = append(bills, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, b := range bills {
		amount := amountFor(b.area, b.rate)
		if _, err := tx.Exec(ctx, `
			INSERT INTO maintenance_dues (plot_id, period_label, amount_due, amount_paid,
			                              due_date, rate_per_sqft, area_sqft, sector)
			VALUES ($1, 'One-time maintenance', $2, 0, CURRENT_DATE + 30, $3, $4, $5)`,
			b.plotID, amount, b.rate, b.area, b.sector); err != nil {
			return 0, 0, fmt.Errorf("bill plot %s: %w", b.plotID, err)
		}
		raised++
		total += amount
	}
	return raised, total, tx.Commit(ctx)
}

// ------------------------------------------------------------------ handler

type Handler struct {
	store *Store
	guard *access.Guard
}

func NewHandler(store *Store, guard *access.Guard) *Handler {
	return &Handler{store: store, guard: guard}
}

func (h *Handler) Routes(mux *http.ServeMux, a *auth.Authenticator) {
	staff := func(fn httpx.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(fn, auth.RequireStaff()))
	}
	mux.Handle("GET /api/societies/{societyId}/maintenance-rates", staff(h.list))
	mux.Handle("PUT /api/societies/{societyId}/maintenance-rates", staff(h.setRate))
	mux.Handle("POST /api/societies/{societyId}/maintenance-bills", staff(h.generate))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	societyID, caller, err := h.scope(r)
	if err != nil {
		return err
	}

	rates, fallback, err := h.store.List(r.Context(), societyID)
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That society has no maintenance rate set.")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	_ = caller

	return httpx.JSON(w, http.StatusOK, map[string]any{
		"rates":             rates,
		"defaultRate":       fallback,
		"defaultReference":  amountFor(ReferenceAreaSqft, fallback),
		"referenceAreaSqft": ReferenceAreaSqft,
	})
}

func (h *Handler) setRate(w http.ResponseWriter, r *http.Request) error {
	societyID, caller, err := h.scope(r)
	if err != nil {
		return err
	}

	var req struct {
		// Empty sector sets the society-wide fallback.
		Sector      string  `json:"sector"`
		RatePerSqft float64 `json:"ratePerSqft"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if req.RatePerSqft < 0 {
		return httpx.Invalid(map[string]string{"ratePerSqft": "A rate cannot be negative."})
	}
	// A plot here is 1,130-4,035 sq ft, so anything past Rs 500/sq ft is a
	// typo — a slipped decimal point would bill lakhs per plot.
	if req.RatePerSqft > 500 {
		return httpx.Invalid(map[string]string{
			"ratePerSqft": "That is over ₹500/sq ft. Check the decimal point.",
		})
	}

	if err := h.store.SetRate(r.Context(), societyID,
		strings.TrimSpace(req.Sector), req.RatePerSqft, caller.UserID); err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{
		"ratePerSqft":     req.RatePerSqft,
		"referenceAmount": amountFor(ReferenceAreaSqft, req.RatePerSqft),
	})
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) error {
	societyID, _, err := h.scope(r)
	if err != nil {
		return err
	}

	raised, total, err := h.store.GenerateBills(r.Context(), societyID)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{
		"raised": raised,
		"total":  total,
	})
}

// scope parses the society id and confirms the caller's builder owns it.
func (h *Handler) scope(r *http.Request) (uuid.UUID, auth.Identity, error) {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return uuid.Nil, auth.Identity{}, httpx.BadRequest("Not a valid society id.")
	}
	caller := auth.MustFromContext(r.Context())
	if err := h.guard.Society(r.Context(), caller.Role, caller.BuilderID, societyID); err != nil {
		return uuid.Nil, auth.Identity{}, err
	}
	return societyID, caller, nil
}
