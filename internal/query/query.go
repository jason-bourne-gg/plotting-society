// Package query is the owner's ticket system. Its reason to exist is the SLA
// clock: the owner can see how long the builder has left to answer, which is
// what a WhatsApp group can never give them.
package query

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

var ErrNotFound = errors.New("not found")

type Query struct {
	ID         uuid.UUID  `json:"id"`
	SocietyID  uuid.UUID  `json:"societyId"`
	PlotID     *uuid.UUID `json:"plotId,omitempty"`
	PlotNo     string     `json:"plotNo,omitempty"`
	RaisedBy   uuid.UUID  `json:"raisedBy"`
	RaisedName string     `json:"raisedByName"`
	Category   string     `json:"category"`
	Subject    string     `json:"subject"`
	Status     string     `json:"status"`
	Priority   string     `json:"priority"`
	SLADueAt   *time.Time `json:"slaDueAt,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	// Breached is computed, not stored, so it is always current.
	Breached bool `json:"breached"`
}

type Message struct {
	ID            uuid.UUID `json:"id"`
	AuthorID      uuid.UUID `json:"authorId"`
	AuthorName    string    `json:"authorName"`
	Body          string    `json:"body"`
	AttachmentURL string    `json:"attachmentUrl,omitempty"`
	IsInternal    bool      `json:"isInternal"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

const queryColumns = `
	q.id, q.society_id, q.plot_id, COALESCE(p.plot_no,''), q.raised_by, COALESCE(u.name,''),
	q.category, q.subject, q.status, q.priority, q.sla_due_at, q.resolved_at,
	q.created_at, q.updated_at`

func scanQueries(rows pgx.Rows) ([]Query, error) {
	defer rows.Close()
	out := []Query{}
	for rows.Next() {
		var q Query
		if err := rows.Scan(&q.ID, &q.SocietyID, &q.PlotID, &q.PlotNo, &q.RaisedBy,
			&q.RaisedName, &q.Category, &q.Subject, &q.Status, &q.Priority,
			&q.SLADueAt, &q.ResolvedAt, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		q.Breached = isBreached(q)
		out = append(out, q)
	}
	return out, rows.Err()
}

// isBreached is true only while the query is still open past its SLA. A query
// resolved late is not "breached" forever; it is simply resolved.
func isBreached(q Query) bool {
	if q.SLADueAt == nil || q.ResolvedAt != nil {
		return false
	}
	if q.Status == "resolved" || q.Status == "closed" {
		return false
	}
	return time.Now().After(*q.SLADueAt)
}

func (s *Store) Create(ctx context.Context, societyID, raisedBy uuid.UUID, plotID *uuid.UUID, category, subject, body string) (uuid.UUID, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	slaDue := time.Now().Add(domain.QuerySLA[category])

	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO queries (society_id, plot_id, raised_by, category, subject, sla_due_at)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		societyID, plotID, raisedBy, category, subject, slaDue).Scan(&id); err != nil {
		return uuid.Nil, err
	}

	// The opening message is the body, so the thread reads in one place.
	if _, err := tx.Exec(ctx,
		`INSERT INTO query_messages (query_id, author_id, body) VALUES ($1,$2,$3)`,
		id, raisedBy, body); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

// ListForOwner returns only the caller's own queries.
func (s *Store) ListForOwner(ctx context.Context, ownerID uuid.UUID, limit int) ([]Query, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+queryColumns+`
		  FROM queries q
		  LEFT JOIN plots p ON p.id = q.plot_id
		  LEFT JOIN users u ON u.id = q.raised_by
		 WHERE q.raised_by = $1
		 ORDER BY q.created_at DESC LIMIT $2`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	return scanQueries(rows)
}

// ListForSociety is the builder's inbox. status may be empty for all.
func (s *Store) ListForSociety(ctx context.Context, societyID uuid.UUID, status string, limit int) ([]Query, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+queryColumns+`
		  FROM queries q
		  LEFT JOIN plots p ON p.id = q.plot_id
		  LEFT JOIN users u ON u.id = q.raised_by
		 WHERE q.society_id = $1 AND ($2 = '' OR q.status = $2)
		 ORDER BY
		   -- breached and open first: the inbox should surface what is late
		   (q.resolved_at IS NULL AND q.sla_due_at < now()) DESC,
		   q.created_at DESC
		 LIMIT $3`, societyID, status, limit)
	if err != nil {
		return nil, err
	}
	return scanQueries(rows)
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (Query, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+queryColumns+`
		  FROM queries q
		  LEFT JOIN plots p ON p.id = q.plot_id
		  LEFT JOIN users u ON u.id = q.raised_by
		 WHERE q.id = $1`, id)
	if err != nil {
		return Query{}, err
	}
	list, err := scanQueries(rows)
	if err != nil {
		return Query{}, err
	}
	if len(list) == 0 {
		return Query{}, ErrNotFound
	}
	return list[0], nil
}

func (s *Store) Messages(ctx context.Context, queryID uuid.UUID, includeInternal bool) ([]Message, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id, m.author_id, COALESCE(u.name,''), m.body,
		       COALESCE(m.attachment_url,''), m.is_internal, m.created_at
		  FROM query_messages m
		  LEFT JOIN users u ON u.id = m.author_id
		 WHERE m.query_id = $1 AND ($2 OR m.is_internal = false)
		 ORDER BY m.created_at`, queryID, includeInternal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.AuthorID, &m.AuthorName, &m.Body,
			&m.AttachmentURL, &m.IsInternal, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddMessage(ctx context.Context, queryID, authorID uuid.UUID, body, attachment string, internal bool) (uuid.UUID, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO query_messages (query_id, author_id, body, attachment_url, is_internal)
		VALUES ($1,$2,$3,NULLIF($4,''),$5) RETURNING id`,
		queryID, authorID, body, attachment, internal).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE queries SET updated_at = now() WHERE id = $1`, queryID); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) SetStatus(ctx context.Context, queryID uuid.UUID, status string, assignee *uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE queries
		   SET status = $2,
		       assigned_to = COALESCE($3, assigned_to),
		       resolved_at = CASE WHEN $2 IN ('resolved','closed') THEN COALESCE(resolved_at, now()) ELSE NULL END,
		       updated_at = now()
		 WHERE id = $1`, queryID, status, assignee)
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
	mux.Handle("POST /api/societies/{societyId}/queries", a.RequireAuth(httpx.Handler(h.create)))
	mux.Handle("GET /api/queries/mine", a.RequireAuth(httpx.Handler(h.listMine)))
	mux.Handle("GET /api/queries/{queryId}", a.RequireAuth(httpx.Handler(h.get)))
	mux.Handle("POST /api/queries/{queryId}/messages", a.RequireAuth(httpx.Handler(h.addMessage)))

	staff := func(fn httpx.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(fn, auth.RequireStaff()))
	}
	mux.Handle("GET /api/societies/{societyId}/queries", staff(h.listForSociety))
	mux.Handle("PATCH /api/queries/{queryId}", staff(h.setStatus))
	mux.Handle("GET /api/categories", httpx.Handler(h.categories))
}

// categories is public so the "raise a query" form can render before sign-in.
func (h *Handler) categories(w http.ResponseWriter, r *http.Request) error {
	type cat struct {
		Key     string `json:"key"`
		Label   string `json:"label"`
		SLADays int    `json:"slaDays"`
	}
	labels := map[string]string{
		"documents_legal":  "Documents & legal",
		"payments_dues":    "Payments & dues",
		"plot_condition":   "Plot condition",
		"infrastructure":   "Roads, drainage & lighting",
		"construction_noc": "Construction NOC",
		"resale_transfer":  "Resale or transfer",
		"site_visit":       "Site visit or video walkthrough",
		"other":            "Something else",
	}
	// Fixed order: the list should not reshuffle between page loads.
	order := []string{"documents_legal", "payments_dues", "plot_condition", "infrastructure",
		"construction_noc", "resale_transfer", "site_visit", "other"}

	out := make([]cat, 0, len(order))
	for _, key := range order {
		out = append(out, cat{Key: key, Label: labels[key],
			SLADays: int(domain.QuerySLA[key].Hours() / 24)})
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"categories": out})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	identity := auth.MustFromContext(r.Context())

	var req struct {
		PlotID   string `json:"plotId,omitempty"`
		Category string `json:"category"`
		Subject  string `json:"subject"`
		Body     string `json:"body"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}

	fields := map[string]string{}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Body = strings.TrimSpace(req.Body)
	if req.Subject == "" {
		fields["subject"] = "Add a short subject."
	}
	if len(req.Subject) > 200 {
		fields["subject"] = "Keep the subject under 200 characters."
	}
	if req.Body == "" {
		fields["body"] = "Describe what you need."
	}
	if !domain.ValidQueryCategory(req.Category) {
		fields["category"] = "Pick a category."
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	var plotID *uuid.UUID
	if req.PlotID != "" {
		id, err := uuid.Parse(req.PlotID)
		if err != nil {
			return httpx.Invalid(map[string]string{"plotId": "Not a valid plot id."})
		}
		plotID = &id
	}

	id, err := h.store.Create(r.Context(), societyID, identity.UserID, plotID,
		req.Category, req.Subject, req.Body)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) listMine(w http.ResponseWriter, r *http.Request) error {
	identity := auth.MustFromContext(r.Context())
	limit := httpx.QueryInt(r, "limit", 50, 1, 200)

	list, err := h.store.ListForOwner(r.Context(), identity.UserID, limit)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"queries": list})
}

func (h *Handler) listForSociety(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	// RequireStaff proves the caller is a builder, not that they are this one.
	caller := auth.MustFromContext(r.Context())
	if err := h.guard.Society(r.Context(), caller.Role, caller.BuilderID, societyID); err != nil {
		return err
	}

	status := r.URL.Query().Get("status")
	if status != "" && !domain.ValidQueryStatus(status) {
		return httpx.BadRequest("Unknown status filter.")
	}
	limit := httpx.QueryInt(r, "limit", 100, 1, 500)

	list, err := h.store.ListForSociety(r.Context(), societyID, status, limit)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"queries": list})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	queryID, err := uuid.Parse(r.PathValue("queryId"))
	if err != nil {
		return httpx.BadRequest("Not a valid query id.")
	}
	identity := auth.MustFromContext(r.Context())

	q, err := h.store.Get(r.Context(), queryID)
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That query does not exist.")
	}
	if err != nil {
		return httpx.Internal(err)
	}

	staff := identity.Role.IsStaff()
	if staff {
		if err := h.guard.Query(r.Context(), identity.Role, identity.BuilderID, queryID); err != nil {
			return err
		}
	} else if q.RaisedBy != identity.UserID {
		// Same body as a genuine miss, so ids cannot be probed.
		return httpx.NotFound("That query does not exist.")
	}

	messages, err := h.store.Messages(r.Context(), queryID, staff)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"query": q, "messages": messages})
}

func (h *Handler) addMessage(w http.ResponseWriter, r *http.Request) error {
	queryID, err := uuid.Parse(r.PathValue("queryId"))
	if err != nil {
		return httpx.BadRequest("Not a valid query id.")
	}
	identity := auth.MustFromContext(r.Context())

	var req struct {
		Body          string `json:"body"`
		AttachmentURL string `json:"attachmentUrl,omitempty"`
		IsInternal    bool   `json:"isInternal,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Body) == "" {
		return httpx.Invalid(map[string]string{"body": "Write a message."})
	}

	q, err := h.store.Get(r.Context(), queryID)
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That query does not exist.")
	}
	if err != nil {
		return httpx.Internal(err)
	}

	staff := identity.Role.IsStaff()
	if staff {
		if err := h.guard.Query(r.Context(), identity.Role, identity.BuilderID, queryID); err != nil {
			return err
		}
	} else if q.RaisedBy != identity.UserID {
		return httpx.NotFound("That query does not exist.")
	}
	if !domain.SafeExternalURL(req.AttachmentURL) {
		return httpx.Invalid(map[string]string{"attachmentUrl": "Attach an http or https link."})
	}
	// Only staff can leave an internal note; an owner marking one would
	// silently hide their own message from themselves.
	internal := req.IsInternal && staff

	id, err := h.store.AddMessage(r.Context(), queryID, identity.UserID,
		strings.TrimSpace(req.Body), req.AttachmentURL, internal)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) setStatus(w http.ResponseWriter, r *http.Request) error {
	queryID, err := uuid.Parse(r.PathValue("queryId"))
	if err != nil {
		return httpx.BadRequest("Not a valid query id.")
	}
	caller := auth.MustFromContext(r.Context())
	if err := h.guard.Query(r.Context(), caller.Role, caller.BuilderID, queryID); err != nil {
		return err
	}

	var req struct {
		Status     string `json:"status"`
		AssignedTo string `json:"assignedTo,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if !domain.ValidQueryStatus(req.Status) {
		return httpx.Invalid(map[string]string{"status": "Unknown status."})
	}

	var assignee *uuid.UUID
	if req.AssignedTo != "" {
		id, err := uuid.Parse(req.AssignedTo)
		if err != nil {
			return httpx.Invalid(map[string]string{"assignedTo": "Not a valid user id."})
		}
		assignee = &id
	}

	if err := h.store.SetStatus(r.Context(), queryID, req.Status, assignee); errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That query does not exist.")
	} else if err != nil {
		return httpx.Internal(err)
	}
	return httpx.NoContent(w)
}
