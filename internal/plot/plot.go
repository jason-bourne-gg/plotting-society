// Package plot serves the society layout map and the per-plot detail page.
//
// The map is the product's centre of gravity: it is the one thing an owner
// living 800km away cannot get today without phoning the sales office.
package plot

import (
	"encoding/json"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

var ErrNotFound = errors.New("not found")

// MapPlot is the public shape. It deliberately carries no owner name or phone:
// the layout map is visible to every signed-in owner, and one neighbour must
// not be able to scrape the contact details of all the others.
type MapPlot struct {
	ID        uuid.UUID `json:"id"`
	PlotNo    string    `json:"plotNo"`
	Phase     string    `json:"phase,omitempty"`
	AreaSqft  *float64  `json:"areaSqft,omitempty"`
	Facing    string    `json:"facing,omitempty"`
	IsCorner  bool      `json:"isCorner"`
	Status    string    `json:"status"`
	Price     *float64  `json:"price,omitempty"`
	// jsonb. json.RawMessage, not []byte — see the note in internal/lead.
	MapShape  json.RawMessage `json:"mapShape,omitempty"`
	IsMine    bool      `json:"isMine"`
}

// Detail adds the fields only the plot's own owner or the builder may see.
type Detail struct {
	MapPlot
	Notes     string  `json:"notes,omitempty"`
	OwnerName string  `json:"ownerName,omitempty"`
	Dues      []Due   `json:"dues,omitempty"`
	Documents []Doc   `json:"documents,omitempty"`
}

type Due struct {
	ID          uuid.UUID `json:"id"`
	PeriodLabel string    `json:"periodLabel"`
	AmountDue   float64   `json:"amountDue"`
	AmountPaid  float64   `json:"amountPaid"`
	DueDate     *string   `json:"dueDate,omitempty"`
	PaidOn      *string   `json:"paidOn,omitempty"`
	ReceiptURL  string    `json:"receiptUrl,omitempty"`
	// Snapshotted when the bill was raised, so the owner can be shown the
	// working — "1,291.68 sq ft x Rs 5.00" — rather than a bare total they
	// have to take on trust.
	RatePerSqft *float64 `json:"ratePerSqft,omitempty"`
	AreaSqft    *float64 `json:"areaSqft,omitempty"`
}

type Doc struct {
	ID      uuid.UUID `json:"id"`
	DocType string    `json:"docType"`
	Title   string    `json:"title"`
	URL     string    `json:"url"`
}

// Summary is the headline the society page opens with.
type Summary struct {
	Total     int `json:"total"`
	Sold      int `json:"sold"`
	Available int `json:"available"`
	Booked    int `json:"booked"`
	OnHold    int `json:"onHold"`
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

// ListForMap returns every plot in the society. viewer may be uuid.Nil.
func (s *Store) ListForMap(ctx context.Context, societyID, viewer uuid.UUID) ([]MapPlot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, plot_no, COALESCE(phase,''), area_sqft, COALESCE(facing,''),
		       is_corner, status, price, map_shape,
		       (owner_id IS NOT NULL AND owner_id = $2) AS is_mine
		  FROM plots
		 WHERE society_id = $1
		 ORDER BY phase NULLS FIRST, plot_no`, societyID, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plots := []MapPlot{}
	for rows.Next() {
		var p MapPlot
		if err := rows.Scan(&p.ID, &p.PlotNo, &p.Phase, &p.AreaSqft, &p.Facing,
			&p.IsCorner, &p.Status, &p.Price, &p.MapShape, &p.IsMine); err != nil {
			return nil, err
		}
		plots = append(plots, p)
	}
	return plots, rows.Err()
}

func (s *Store) Summary(ctx context.Context, societyID uuid.UUID) (Summary, error) {
	var sum Summary
	err := s.db.QueryRow(ctx, `
		SELECT count(*)::int,
		       count(*) FILTER (WHERE status = 'sold')::int,
		       count(*) FILTER (WHERE status = 'available')::int,
		       count(*) FILTER (WHERE status = 'booked')::int,
		       count(*) FILTER (WHERE status = 'on_hold')::int
		  FROM plots WHERE society_id = $1`, societyID,
	).Scan(&sum.Total, &sum.Sold, &sum.Available, &sum.Booked, &sum.OnHold)
	return sum, err
}

func (s *Store) Get(ctx context.Context, plotID uuid.UUID) (Detail, uuid.UUID, uuid.UUID, error) {
	var d Detail
	var ownerID *uuid.UUID
	var societyID uuid.UUID

	err := s.db.QueryRow(ctx, `
		SELECT p.id, p.plot_no, COALESCE(p.phase,''), p.area_sqft, COALESCE(p.facing,''),
		       p.is_corner, p.status, p.price, p.map_shape, COALESCE(p.notes,''),
		       p.owner_id, p.society_id, COALESCE(u.name,'')
		  FROM plots p
		  LEFT JOIN users u ON u.id = p.owner_id
		 WHERE p.id = $1`, plotID,
	).Scan(&d.ID, &d.PlotNo, &d.Phase, &d.AreaSqft, &d.Facing, &d.IsCorner,
		&d.Status, &d.Price, &d.MapShape, &d.Notes, &ownerID, &societyID, &d.OwnerName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, uuid.Nil, uuid.Nil, ErrNotFound
	}
	if err != nil {
		return Detail{}, uuid.Nil, uuid.Nil, err
	}

	owner := uuid.Nil
	if ownerID != nil {
		owner = *ownerID
	}
	return d, owner, societyID, nil
}

func (s *Store) DuesForPlot(ctx context.Context, plotID uuid.UUID) ([]Due, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, period_label, amount_due, amount_paid,
		       to_char(due_date,'YYYY-MM-DD'), to_char(paid_on,'YYYY-MM-DD'),
		       COALESCE(receipt_url,''), rate_per_sqft, area_sqft
		  FROM maintenance_dues WHERE plot_id = $1 ORDER BY due_date DESC NULLS LAST`, plotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dues := []Due{}
	for rows.Next() {
		var d Due
		if err := rows.Scan(&d.ID, &d.PeriodLabel, &d.AmountDue, &d.AmountPaid,
			&d.DueDate, &d.PaidOn, &d.ReceiptURL, &d.RatePerSqft, &d.AreaSqft); err != nil {
			return nil, err
		}
		dues = append(dues, d)
	}
	return dues, rows.Err()
}

func (s *Store) DocumentsForPlot(ctx context.Context, plotID, societyID uuid.UUID, staff bool) ([]Doc, error) {
	visible := "('society','plot')"
	if staff {
		visible = "('society','plot','staff')"
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, doc_type, title, url
		  FROM documents
		 WHERE (plot_id = $1 OR (plot_id IS NULL AND society_id = $2 AND visibility = 'society'))
		   AND visibility IN `+visible+`
		 ORDER BY created_at DESC`, plotID, societyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []Doc{}
	for rows.Next() {
		var d Doc
		if err := rows.Scan(&d.ID, &d.DocType, &d.Title, &d.URL); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

type UpsertInput struct {
	PlotNo   string   `json:"plotNo"`
	Phase    string   `json:"phase"`
	AreaSqft *float64 `json:"areaSqft"`
	Facing   string   `json:"facing"`
	IsCorner bool     `json:"isCorner"`
	Status   string   `json:"status"`
	Price    *float64 `json:"price"`
	MapShape json.RawMessage `json:"mapShape"`
	Notes    string   `json:"notes"`
}

func (s *Store) Create(ctx context.Context, societyID uuid.UUID, in UpsertInput) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `
		INSERT INTO plots (society_id, plot_no, phase, area_sqft, facing, is_corner, status, price, map_shape, notes)
		VALUES ($1,$2,NULLIF($3,''),$4,NULLIF($5,''),$6,$7,$8,$9,NULLIF($10,''))
		RETURNING id`,
		societyID, in.PlotNo, in.Phase, in.AreaSqft, in.Facing, in.IsCorner,
		in.Status, in.Price, in.MapShape, in.Notes).Scan(&id)
	return id, err
}

func (s *Store) Update(ctx context.Context, plotID uuid.UUID, in UpsertInput) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE plots
		   SET plot_no = $2, phase = NULLIF($3,''), area_sqft = $4, facing = NULLIF($5,''),
		       is_corner = $6, status = $7, price = $8, map_shape = $9,
		       notes = NULLIF($10,''), updated_at = now()
		 WHERE id = $1`,
		plotID, in.PlotNo, in.Phase, in.AreaSqft, in.Facing, in.IsCorner,
		in.Status, in.Price, in.MapShape, in.Notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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
	mux.Handle("GET /api/societies/{societyId}/plots", a.Optional(httpx.Handler(h.list)))
	mux.Handle("GET /api/societies/{societyId}/summary", a.Optional(httpx.Handler(h.summary)))
	mux.Handle("GET /api/plots/{plotId}", a.RequireAuth(httpx.Handler(h.get)))

	staff := func(fn httpx.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(fn, auth.RequireStaff()))
	}
	mux.Handle("POST /api/societies/{societyId}/plots", staff(h.create))
	mux.Handle("PATCH /api/plots/{plotId}", staff(h.update))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}

	viewer := uuid.Nil
	if identity, ok := auth.FromContext(r.Context()); ok {
		viewer = identity.UserID
	}

	plots, err := h.store.ListForMap(r.Context(), societyID, viewer)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"plots": plots})
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	sum, err := h.store.Summary(r.Context(), societyID)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, sum)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	plotID, err := uuid.Parse(r.PathValue("plotId"))
	if err != nil {
		return httpx.BadRequest("Not a valid plot id.")
	}
	identity := auth.MustFromContext(r.Context())

	detail, ownerID, societyID, err := h.store.Get(r.Context(), plotID)
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That plot does not exist.")
	}
	if err != nil {
		return httpx.Internal(err)
	}

	staff := identity.Role.IsStaff()
	mine := ownerID != uuid.Nil && ownerID == identity.UserID
	detail.IsMine = mine

	// A non-owner sees the plot as it appears on the map and nothing more.
	if !staff && !mine {
		detail.Notes = ""
		detail.OwnerName = ""
		return httpx.JSON(w, http.StatusOK, detail)
	}

	if detail.Dues, err = h.store.DuesForPlot(r.Context(), plotID); err != nil {
		return httpx.Internal(err)
	}
	if detail.Documents, err = h.store.DocumentsForPlot(r.Context(), plotID, societyID, staff); err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	// RequireStaff proves the caller is a builder, not that they are this one.
	caller := auth.MustFromContext(r.Context())
	if err := h.guard.Society(r.Context(), caller.Role, caller.BuilderID, societyID); err != nil {
		return err
	}
	in, err := decodeUpsert(r)
	if err != nil {
		return err
	}
	id, err := h.store.Create(r.Context(), societyID, in)
	if err != nil {
		if strings.Contains(err.Error(), "plots_society_id_plot_no_key") {
			return httpx.Conflict("A plot with that number already exists in this society.")
		}
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	plotID, err := uuid.Parse(r.PathValue("plotId"))
	if err != nil {
		return httpx.BadRequest("Not a valid plot id.")
	}
	caller := auth.MustFromContext(r.Context())
	if err := h.guard.Plot(r.Context(), caller.Role, caller.BuilderID, plotID); err != nil {
		return err
	}
	in, err := decodeUpsert(r)
	if err != nil {
		return err
	}
	if err := h.store.Update(r.Context(), plotID, in); errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That plot does not exist.")
	} else if err != nil {
		// Renaming a plot onto a number that already exists is the caller's
		// mistake, the same as it is on create.
		if strings.Contains(err.Error(), "plots_society_id_plot_no_key") {
			return httpx.Conflict("A plot with that number already exists in this society.")
		}
		return httpx.Internal(err)
	}
	return httpx.NoContent(w)
}

func decodeUpsert(r *http.Request) (UpsertInput, error) {
	var in UpsertInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return in, err
	}

	fields := map[string]string{}
	in.PlotNo = strings.TrimSpace(in.PlotNo)
	if in.PlotNo == "" {
		fields["plotNo"] = "Plot number is required."
	}
	if in.Status == "" {
		in.Status = domain.PlotAvailable
	}
	if !domain.ValidPlotStatus(in.Status) {
		fields["status"] = "Unknown plot status."
	}
	if in.AreaSqft != nil && *in.AreaSqft <= 0 {
		fields["areaSqft"] = "Area must be greater than zero."
	}
	if in.Price != nil && *in.Price < 0 {
		fields["price"] = "Price cannot be negative."
	}
	if len(fields) > 0 {
		return in, httpx.Invalid(fields)
	}
	return in, nil
}
