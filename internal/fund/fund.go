// Package fund is the society's money ledger.
//
// It is append-only on purpose. When an owner disputes a number, the builder
// must be able to show the whole history rather than a figure that has quietly
// changed; a mistake is corrected with a reversal row, never an UPDATE.
package fund

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

var ErrNotFound = errors.New("not found")

type Entry struct {
	ID          uuid.UUID  `json:"id"`
	EntryDate   string     `json:"entryDate"`
	Head        string     `json:"head"`
	Description string     `json:"description"`
	Credit      float64    `json:"credit"`
	Debit       float64    `json:"debit"`
	DocumentURL string     `json:"documentUrl,omitempty"`
	ReversesID  *uuid.UUID `json:"reversesId,omitempty"`
	CreatedBy   string     `json:"createdBy,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// Balance is what the society page shows at the top.
type Balance struct {
	TotalCredit float64            `json:"totalCredit"`
	TotalDebit  float64            `json:"totalDebit"`
	Closing     float64            `json:"closing"`
	ByHead      map[string]float64 `json:"byHead"`
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

func (s *Store) List(ctx context.Context, societyID uuid.UUID, limit int) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT f.id, to_char(f.entry_date,'YYYY-MM-DD'), f.head, f.description,
		       f.credit, f.debit, COALESCE(f.document_url,''), f.reverses_id,
		       COALESCE(u.name,''), f.created_at
		  FROM fund_entries f
		  LEFT JOIN users u ON u.id = f.created_by
		 WHERE f.society_id = $1
		 ORDER BY f.entry_date DESC, f.created_at DESC
		 LIMIT $2`, societyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.EntryDate, &e.Head, &e.Description, &e.Credit,
			&e.Debit, &e.DocumentURL, &e.ReversesID, &e.CreatedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Balance(ctx context.Context, societyID uuid.UUID) (Balance, error) {
	b := Balance{ByHead: map[string]float64{}}

	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(sum(credit),0), COALESCE(sum(debit),0)
		  FROM fund_entries WHERE society_id = $1`, societyID,
	).Scan(&b.TotalCredit, &b.TotalDebit)
	if err != nil {
		return b, err
	}
	b.Closing = b.TotalCredit - b.TotalDebit

	rows, err := s.db.Query(ctx, `
		SELECT head, COALESCE(sum(debit),0)
		  FROM fund_entries
		 WHERE society_id = $1 AND debit > 0
		 GROUP BY head ORDER BY 2 DESC`, societyID)
	if err != nil {
		return b, err
	}
	defer rows.Close()

	for rows.Next() {
		var head string
		var spent float64
		if err := rows.Scan(&head, &spent); err != nil {
			return b, err
		}
		b.ByHead[head] = spent
	}
	return b, rows.Err()
}

type NewEntry struct {
	EntryDate   string  `json:"entryDate"`
	Head        string  `json:"head"`
	Description string  `json:"description"`
	Credit      float64 `json:"credit"`
	Debit       float64 `json:"debit"`
	DocumentURL string  `json:"documentUrl,omitempty"`
}

func (s *Store) Create(ctx context.Context, societyID, createdBy uuid.UUID, in NewEntry) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `
		INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, document_url, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)
		RETURNING id`,
		societyID, in.EntryDate, in.Head, in.Description, in.Credit, in.Debit,
		in.DocumentURL, createdBy).Scan(&id)
	return id, err
}

// Reverse writes the mirror image of an existing entry and links the two.
func (s *Store) Reverse(ctx context.Context, entryID, createdBy uuid.UUID, reason string) (uuid.UUID, error) {
	var newID uuid.UUID
	err := s.db.QueryRow(ctx, `
		INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, reverses_id, created_by)
		SELECT society_id, CURRENT_DATE, head, $3, debit, credit, id, $2
		  FROM fund_entries
		 WHERE id = $1 AND reverses_id IS NULL
		   AND NOT EXISTS (SELECT 1 FROM fund_entries r WHERE r.reverses_id = fund_entries.id)
		RETURNING id`, entryID, createdBy, reason).Scan(&newID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the entry is gone, is itself a reversal, or is already reversed.
		return uuid.Nil, ErrNotFound
	}
	return newID, err
}

// ------------------------------------------------------------------ handler

type Handler struct{ store *Store }

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

func (h *Handler) Routes(mux *http.ServeMux, a *auth.Authenticator) {
	// Every signed-in owner reads the ledger. That transparency is the point.
	mux.Handle("GET /api/societies/{societyId}/fund", a.RequireAuth(httpx.Handler(h.list)))

	staff := func(fn httpx.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(fn, auth.RequireStaff()))
	}
	mux.Handle("POST /api/societies/{societyId}/fund", staff(h.create))
	mux.Handle("POST /api/fund/{entryId}/reverse", staff(h.reverse))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	limit := httpx.QueryInt(r, "limit", 200, 1, 1000)

	entries, err := h.store.List(r.Context(), societyID, limit)
	if err != nil {
		return httpx.Internal(err)
	}
	balance, err := h.store.Balance(r.Context(), societyID)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"balance": balance, "entries": entries})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	identity := auth.MustFromContext(r.Context())

	var in NewEntry
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}

	fields := map[string]string{}
	in.Head = strings.TrimSpace(in.Head)
	in.Description = strings.TrimSpace(in.Description)
	if in.Head == "" {
		fields["head"] = "Pick a head, e.g. security or roads."
	}
	if in.Description == "" {
		fields["description"] = "Describe what this is for."
	}
	if _, err := time.Parse("2006-01-02", in.EntryDate); err != nil {
		fields["entryDate"] = "Use a YYYY-MM-DD date."
	}
	if in.Credit < 0 || in.Debit < 0 {
		fields["amount"] = "Amounts cannot be negative."
	}
	// Exactly one side, matching the database CHECK.
	if (in.Credit > 0) == (in.Debit > 0) {
		fields["amount"] = "Enter either money in or money out, not both."
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	id, err := h.store.Create(r.Context(), societyID, identity.UserID, in)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) reverse(w http.ResponseWriter, r *http.Request) error {
	entryID, err := uuid.Parse(r.PathValue("entryId"))
	if err != nil {
		return httpx.BadRequest("Not a valid entry id.")
	}
	identity := auth.MustFromContext(r.Context())

	var req struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Reason) == "" {
		return httpx.Invalid(map[string]string{"reason": "Say why this entry is being reversed."})
	}

	id, err := h.store.Reverse(r.Context(), entryID, identity.UserID,
		"Reversal: "+strings.TrimSpace(req.Reason))
	if errors.Is(err, ErrNotFound) {
		return httpx.Conflict("That entry does not exist, or has already been reversed.")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}
