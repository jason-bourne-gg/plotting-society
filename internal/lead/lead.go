// Package lead handles guest access: the public, no-login view of a society
// and the enquiries it produces.
//
// Guests are not users. They have no account, no session and no row in `users`;
// "guest" is simply the absence of a token on a small set of read-only routes,
// plus one rate-limited write — the enquiry. That keeps authorisation to three
// real states, anonymous / owner / builder, instead of inventing a fourth
// identity to maintain.
package lead

import (
	"encoding/json"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

var ErrNotFound = errors.New("not found")

type Enquiry struct {
	ID        uuid.UUID  `json:"id"`
	PlotID    *uuid.UUID `json:"plotId,omitempty"`
	PlotNo    string     `json:"plotNo,omitempty"`
	Name      string     `json:"name"`
	Phone     string     `json:"phone"`
	Email     string     `json:"email,omitempty"`
	Message   string     `json:"message,omitempty"`
	Budget    string     `json:"budget,omitempty"`
	Source    string     `json:"source"`
	Status    string     `json:"status"`
	Notes     string     `json:"notes,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// PublicSociety is the guest-facing shape. It deliberately omits everything
// operational: no fund figures, no owner names, no query counts.
type PublicSociety struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	BuilderName string    `json:"builderName"`
	City        string    `json:"city,omitempty"`
	Address     string    `json:"address,omitempty"`
	RERANumber  string    `json:"reraNumber,omitempty"`
	Tagline     string    `json:"tagline,omitempty"`
	// jsonb. Must be json.RawMessage, not []byte: the latter marshals to a
	// base64 string and the client gets "W3si..." instead of an array.
	Highlights  json.RawMessage `json:"highlights,omitempty"`
	Amenities   json.RawMessage `json:"amenities,omitempty"`
	BrochureURL string    `json:"brochureUrl,omitempty"`
	Phone       string    `json:"contactPhone,omitempty"`
	Email       string    `json:"contactEmail,omitempty"`

	// Where the land is. Latitude and longitude are pointers because they are
	// null until the site office confirms them: a pin in roughly the right
	// district is worse than no pin, since it looks authoritative.
	Latitude  *float64        `json:"latitude,omitempty"`
	Longitude *float64        `json:"longitude,omitempty"`
	MapLabel  string          `json:"mapLabel,omitempty"`
	Landmarks json.RawMessage `json:"landmarks,omitempty"`
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

func (s *Store) PublicBySlug(ctx context.Context, slug string) (PublicSociety, error) {
	var p PublicSociety
	err := s.db.QueryRow(ctx, `
		SELECT s.id, s.name, s.slug, COALESCE(b.name,''), COALESCE(s.city,''),
		       COALESCE(s.address,''), COALESCE(s.rera_number,''), COALESCE(s.tagline,''),
		       s.highlights, s.amenities, COALESCE(s.brochure_url,''),
		       COALESCE(s.contact_phone, b.phone, ''), COALESCE(s.contact_email, b.email, ''),
		       s.latitude, s.longitude, COALESCE(s.map_label,''), s.landmarks
		  FROM societies s JOIN builders b ON b.id = s.builder_id
		 WHERE s.slug = $1 AND s.public_listing = true`, slug,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.BuilderName, &p.City, &p.Address, &p.RERANumber,
		&p.Tagline, &p.Highlights, &p.Amenities, &p.BrochureURL, &p.Phone, &p.Email,
		&p.Latitude, &p.Longitude, &p.MapLabel, &p.Landmarks)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicSociety{}, ErrNotFound
	}
	return p, err
}

// FirstPublic backs the guest landing page when no slug is in the URL.
func (s *Store) FirstPublic(ctx context.Context) (PublicSociety, error) {
	var slug string
	err := s.db.QueryRow(ctx,
		`SELECT slug FROM societies WHERE public_listing = true ORDER BY created_at LIMIT 1`).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicSociety{}, ErrNotFound
	}
	if err != nil {
		return PublicSociety{}, err
	}
	return s.PublicBySlug(ctx, slug)
}

type NewEnquiry struct {
	PlotID  *uuid.UUID
	Name    string
	Phone   string
	Email   string
	Message string
	Budget  string
	Source  string
	Client  string
}

func (s *Store) Create(ctx context.Context, societyID uuid.UUID, in NewEnquiry) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `
		INSERT INTO enquiries (society_id, plot_id, name, phone, email, message, budget, source, client_hash)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9)
		RETURNING id`,
		societyID, in.PlotID, in.Name, in.Phone, in.Email, in.Message, in.Budget,
		in.Source, in.Client).Scan(&id)
	return id, err
}

func (s *Store) List(ctx context.Context, societyID uuid.UUID, status string, limit int) ([]Enquiry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id, e.plot_id, COALESCE(p.plot_no,''), e.name, e.phone, COALESCE(e.email,''),
		       COALESCE(e.message,''), COALESCE(e.budget,''), e.source, e.status,
		       COALESCE(e.notes,''), e.created_at
		  FROM enquiries e
		  LEFT JOIN plots p ON p.id = e.plot_id
		 WHERE e.society_id = $1 AND ($2 = '' OR e.status = $2)
		 ORDER BY e.created_at DESC LIMIT $3`, societyID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Enquiry{}
	for rows.Next() {
		var e Enquiry
		if err := rows.Scan(&e.ID, &e.PlotID, &e.PlotNo, &e.Name, &e.Phone, &e.Email,
			&e.Message, &e.Budget, &e.Source, &e.Status, &e.Notes, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Counts backs the admin dashboard's lead tile.
func (s *Store) Counts(ctx context.Context, societyID uuid.UUID) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT status, count(*)::int FROM enquiries WHERE society_id = $1 GROUP BY status`, societyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

func (s *Store) SetStatus(ctx context.Context, id, handler uuid.UUID, status, notes string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE enquiries
		   SET status = $2, notes = NULLIF($3,''), handled_by = $4, updated_at = now()
		 WHERE id = $1`, id, status, notes, handler)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func validEnquiryStatus(s string) bool {
	switch s {
	case "new", "contacted", "visit_scheduled", "converted", "lost":
		return true
	}
	return false
}

// ------------------------------------------------------------- rate limiting
// The enquiry endpoint is the only unauthenticated write in the app, so it gets
// its own bucket: a handful per client per hour is plenty for a real buyer and
// useless to a script.

type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{hits: map[string][]time.Time{}} }

func (l *limiter) allow(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-window)
	kept := make([]time.Time, 0, len(l.hits[key]))
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, time.Now())

	// Opportunistic sweep so the map cannot grow without bound. This is a
	// single-instance limiter; on several instances each holds its own view,
	// which is acceptable for spam control and is not a security boundary.
	if len(l.hits) > 4096 {
		for k, v := range l.hits {
			if len(v) == 0 || v[len(v)-1].Before(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

// ------------------------------------------------------------------ handler

type Handler struct {
	store   *Store
	guard   *access.Guard
	limiter *limiter
}

func NewHandler(store *Store, guard *access.Guard) *Handler {
	return &Handler{store: store, guard: guard, limiter: newLimiter()}
}

func (h *Handler) Routes(mux *http.ServeMux, a *auth.Authenticator) {
	// Guest surface — no token required.
	mux.Handle("GET /api/public/society", httpx.Handler(h.publicSociety))
	mux.Handle("GET /api/public/society/{slug}", httpx.Handler(h.publicSocietyBySlug))
	mux.Handle("POST /api/public/enquiries", httpx.Handler(h.createEnquiry))

	staff := func(fn httpx.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(fn, auth.RequireStaff()))
	}
	mux.Handle("GET /api/societies/{societyId}/enquiries", staff(h.listEnquiries))
	mux.Handle("PATCH /api/enquiries/{enquiryId}", staff(h.updateEnquiry))
}

func (h *Handler) publicSociety(w http.ResponseWriter, r *http.Request) error {
	soc, err := h.store.FirstPublic(r.Context())
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("No project is publicly listed.")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, soc)
}

func (h *Handler) publicSocietyBySlug(w http.ResponseWriter, r *http.Request) error {
	soc, err := h.store.PublicBySlug(r.Context(), r.PathValue("slug"))
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("No project at that address.")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, soc)
}

func (h *Handler) createEnquiry(w http.ResponseWriter, r *http.Request) error {
	client := clientFingerprint(r)
	if !h.limiter.allow(client, 5, time.Hour) {
		return &httpx.Error{
			Status:  http.StatusTooManyRequests,
			Code:    "rate_limited",
			Message: "You have sent a few enquiries already. The site office will call you shortly.",
		}
	}

	var req struct {
		SocietySlug string `json:"societySlug"`
		PlotID      string `json:"plotId,omitempty"`
		Name        string `json:"name"`
		Phone       string `json:"phone"`
		Email       string `json:"email,omitempty"`
		Message     string `json:"message,omitempty"`
		Budget      string `json:"budget,omitempty"`
		Source      string `json:"source,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}

	fields := map[string]string{}
	req.Name = strings.TrimSpace(req.Name)
	req.Phone = strings.TrimSpace(req.Phone)
	if len(req.Name) < 2 {
		fields["name"] = "Tell us your name."
	}
	if !plausiblePhone(req.Phone) {
		fields["phone"] = "Enter a 10-digit mobile number."
	}
	if len(req.Message) > 2000 {
		fields["message"] = "Please keep the message shorter."
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	soc, err := h.store.PublicBySlug(r.Context(), req.SocietySlug)
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("No project at that address.")
	}
	if err != nil {
		return httpx.Internal(err)
	}

	in := NewEnquiry{
		Name: req.Name, Phone: req.Phone, Email: strings.TrimSpace(req.Email),
		Message: strings.TrimSpace(req.Message), Budget: req.Budget,
		Source: req.Source, Client: client,
	}
	if in.Source == "" {
		in.Source = "layout_map"
	}
	if req.PlotID != "" {
		if id, err := uuid.Parse(req.PlotID); err == nil {
			in.PlotID = &id
		}
	}

	if _, err := h.store.Create(r.Context(), soc.ID, in); err != nil {
		return httpx.Internal(err)
	}

	// No id is returned: a guest has no way to read an enquiry back, and handing
	// out an identifier would only invite probing.
	return httpx.JSON(w, http.StatusCreated, map[string]string{
		"message": "Thanks — the site office has your number and will call you.",
	})
}

func (h *Handler) listEnquiries(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	caller := auth.MustFromContext(r.Context())
	if err := h.guard.Society(r.Context(), caller.Role, caller.BuilderID, societyID); err != nil {
		return err
	}

	status := r.URL.Query().Get("status")
	if status != "" && !validEnquiryStatus(status) {
		return httpx.BadRequest("Unknown status filter.")
	}

	list, err := h.store.List(r.Context(), societyID, status, httpx.QueryInt(r, "limit", 100, 1, 500))
	if err != nil {
		return httpx.Internal(err)
	}
	counts, err := h.store.Counts(r.Context(), societyID)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"enquiries": list, "counts": counts})
}

func (h *Handler) updateEnquiry(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(r.PathValue("enquiryId"))
	if err != nil {
		return httpx.BadRequest("Not a valid enquiry id.")
	}
	identity := auth.MustFromContext(r.Context())
	if err := h.guard.Enquiry(r.Context(), identity.Role, identity.BuilderID, id); err != nil {
		return err
	}

	var req struct {
		Status string `json:"status"`
		Notes  string `json:"notes,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if !validEnquiryStatus(req.Status) {
		return httpx.Invalid(map[string]string{"status": "Unknown status."})
	}

	if err := h.store.SetStatus(r.Context(), id, identity.UserID, req.Status, req.Notes); errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That enquiry does not exist.")
	} else if err != nil {
		return httpx.Internal(err)
	}
	return httpx.NoContent(w)
}

// plausiblePhone accepts Indian mobile numbers with or without +91 and the
// usual spacing, without pretending to verify the line exists.
func plausiblePhone(s string) bool {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	digits = strings.TrimPrefix(digits, "91")
	return len(digits) == 10 && digits[0] >= '6'
}

// clientFingerprint hashes IP plus user agent. It is a spam signal only, and
// hashing means no raw address is stored against a lead.
func clientFingerprint(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if comma := strings.IndexByte(ip, ','); comma > 0 {
		ip = ip[:comma]
	}
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		} else {
			ip = r.RemoteAddr
		}
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip) + "|" + r.UserAgent()))
	return hex.EncodeToString(sum[:16])
}
