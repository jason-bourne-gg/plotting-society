// Package update is the builder's construction progress feed: dated posts with
// photos. It is the feature that removes the "any update?" phone call.
package update

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

var ErrNotFound = errors.New("not found")

type Post struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	Body        string     `json:"body,omitempty"`
	Phase       string     `json:"phase,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	AuthorName  string     `json:"authorName,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	Media       []Media    `json:"media"`
}

type Media struct {
	URL     string `json:"url"`
	Caption string `json:"caption,omitempty"`
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

// List returns the feed. Unpublished drafts are visible to staff only.
func (s *Store) List(ctx context.Context, societyID uuid.UUID, includeDrafts bool, limit int) ([]Post, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.title, COALESCE(u.body,''), COALESCE(u.phase,''),
		       u.published_at, COALESCE(a.name,''), u.created_at
		  FROM site_updates u
		  LEFT JOIN users a ON a.id = u.created_by
		 WHERE u.society_id = $1 AND ($2 OR u.published_at IS NOT NULL)
		 ORDER BY COALESCE(u.published_at, u.created_at) DESC
		 LIMIT $3`, societyID, includeDrafts, limit)
	if err != nil {
		return nil, err
	}

	posts := []Post{}
	byID := map[uuid.UUID]int{}
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Body, &p.Phase,
			&p.PublishedAt, &p.AuthorName, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.Media = []Media{}
		byID[p.ID] = len(posts)
		posts = append(posts, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return posts, nil
	}

	// One follow-up query for all media, rather than one per post.
	ids := make([]uuid.UUID, 0, len(posts))
	for id := range byID {
		ids = append(ids, id)
	}
	mediaRows, err := s.db.Query(ctx, `
		SELECT update_id, url, COALESCE(caption,'')
		  FROM site_update_media
		 WHERE update_id = ANY($1)
		 ORDER BY sort_order, created_at`, ids)
	if err != nil {
		return nil, err
	}
	defer mediaRows.Close()

	for mediaRows.Next() {
		var updateID uuid.UUID
		var m Media
		if err := mediaRows.Scan(&updateID, &m.URL, &m.Caption); err != nil {
			return nil, err
		}
		if idx, ok := byID[updateID]; ok {
			posts[idx].Media = append(posts[idx].Media, m)
		}
	}
	return posts, mediaRows.Err()
}

type NewPost struct {
	Title   string  `json:"title"`
	Body    string  `json:"body"`
	Phase   string  `json:"phase"`
	Publish bool    `json:"publish"`
	Media   []Media `json:"media"`
}

func (s *Store) Create(ctx context.Context, societyID, author uuid.UUID, in NewPost) (uuid.UUID, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var publishedAt *time.Time
	if in.Publish {
		now := time.Now()
		publishedAt = &now
	}

	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO site_updates (society_id, title, body, phase, published_at, created_by)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6) RETURNING id`,
		societyID, in.Title, in.Body, in.Phase, publishedAt, author).Scan(&id); err != nil {
		return uuid.Nil, err
	}

	for i, m := range in.Media {
		if _, err := tx.Exec(ctx, `
			INSERT INTO site_update_media (update_id, url, caption, sort_order)
			VALUES ($1,$2,NULLIF($3,''),$4)`, id, m.URL, m.Caption, i); err != nil {
			return uuid.Nil, err
		}
	}
	return id, tx.Commit(ctx)
}

func (s *Store) Publish(ctx context.Context, postID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE site_updates
		   SET published_at = COALESCE(published_at, now()), updated_at = now()
		 WHERE id = $1`, postID)
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
	mux.Handle("GET /api/societies/{societyId}/updates", a.Optional(httpx.Handler(h.list)))

	staff := func(fn httpx.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(fn, auth.RequireStaff()))
	}
	mux.Handle("POST /api/societies/{societyId}/updates", staff(h.create))
	mux.Handle("POST /api/updates/{postId}/publish", staff(h.publish))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	limit := httpx.QueryInt(r, "limit", 30, 1, 100)

	includeDrafts := false
	if identity, ok := auth.FromContext(r.Context()); ok {
		includeDrafts = identity.Role.IsStaff()
	}

	posts, err := h.store.List(r.Context(), societyID, includeDrafts, limit)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"updates": posts})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	societyID, err := uuid.Parse(r.PathValue("societyId"))
	if err != nil {
		return httpx.BadRequest("Not a valid society id.")
	}
	identity := auth.MustFromContext(r.Context())
	if err := h.guard.Society(r.Context(), identity.Role, identity.BuilderID, societyID); err != nil {
		return err
	}

	var in NewPost
	if err := httpx.DecodeJSON(r, &in); err != nil {
		return err
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return httpx.Invalid(map[string]string{"title": "Give the update a title."})
	}
	if len(in.Media) > 20 {
		return httpx.Invalid(map[string]string{"media": "Attach at most 20 photos per update."})
	}
	for _, m := range in.Media {
		if m.URL == "" || !domain.SafeExternalURL(m.URL) {
			return httpx.Invalid(map[string]string{"media": "Photos must be http or https links."})
		}
	}

	id, err := h.store.Create(r.Context(), societyID, identity.UserID, in)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) error {
	postID, err := uuid.Parse(r.PathValue("postId"))
	if err != nil {
		return httpx.BadRequest("Not a valid update id.")
	}
	caller := auth.MustFromContext(r.Context())
	if err := h.guard.Update(r.Context(), caller.Role, caller.BuilderID, postID); err != nil {
		return err
	}
	if err := h.store.Publish(r.Context(), postID); errors.Is(err, ErrNotFound) {
		return httpx.NotFound("That update does not exist.")
	} else if err != nil {
		return httpx.Internal(err)
	}
	return httpx.NoContent(w)
}
