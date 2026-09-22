package lead

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

type fixture struct {
	db      *database.DB
	store   *Store
	handler *Handler
	a, b    testsupport.Builder
	adminA  uuid.UUID
	adminB  uuid.UUID
	plotA   uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testsupport.DB(t, "lead")
	store := NewStore(db)

	f := fixture{
		db:      db,
		store:   store,
		handler: NewHandler(store, access.NewGuard(db)),
	}
	f.a = testsupport.NewBuilder(t, db, "Alpha")
	f.b = testsupport.NewBuilder(t, db, "Beta")
	f.adminA = testsupport.NewUser(t, db, &f.a.ID, domain.RoleBuilderAdmin, "a@alpha.in", "")
	f.adminB = testsupport.NewUser(t, db, &f.b.ID, domain.RoleBuilderAdmin, "b@beta.in", "")
	f.plotA = testsupport.NewPlot(t, db, f.a.SocietyID, "1", "available")
	return f
}

func asStaff(r *http.Request, builderID uuid.UUID, userID uuid.UUID) *http.Request {
	return r.WithContext(auth.ContextWithIdentity(r.Context(), auth.Identity{
		UserID:    userID,
		BuilderID: &builderID,
		Role:      domain.RoleBuilderAdmin,
	}))
}

func status(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var apiErr *httpx.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an httpx.Error, got %T: %v", err, err)
	}
	return apiErr.Status
}

func body(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// ------------------------------------------------------------------ store

func TestPublicBySlug(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	soc, err := f.store.PublicBySlug(ctx, f.a.Slug)
	if err != nil {
		t.Fatalf("PublicBySlug: %v", err)
	}
	if soc.ID != f.a.SocietyID {
		t.Errorf("id = %v", soc.ID)
	}
	if soc.BuilderName != "Alpha" {
		t.Errorf("builder name = %q", soc.BuilderName)
	}

	if _, err := f.store.PublicBySlug(ctx, "no-such-slug"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// A builder who turns off the public listing must disappear from guest view.
func TestPublicBySlugRespectsListingFlag(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.db.Exec(ctx,
		`UPDATE societies SET public_listing = false WHERE id = $1`, f.a.SocietyID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.PublicBySlug(ctx, f.a.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unlisted society should be invisible to guests, got %v", err)
	}
}

func TestFirstPublic(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.store.FirstPublic(ctx); err != nil {
		t.Fatalf("FirstPublic: %v", err)
	}

	if _, err := f.db.Exec(ctx, `UPDATE societies SET public_listing = false`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.FirstPublic(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound with nothing listed, got %v", err)
	}
}

func TestCreateListCountsAndSetStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id, err := f.store.Create(ctx, f.a.SocietyID, NewEnquiry{
		PlotID: &f.plotA, Name: "Sagar", Phone: "9822041190",
		Email: "s@example.in", Message: "interested", Budget: "40-50 L",
		Source: "plot_detail", Client: "hash",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := f.store.List(ctx, f.a.SocietyID, "", 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d enquiries, want 1", len(list))
	}
	got := list[0]
	if got.Name != "Sagar" || got.Status != "new" || got.PlotNo != "1" || got.Budget != "40-50 L" {
		t.Errorf("unexpected row: %+v", got)
	}

	counts, err := f.store.Counts(ctx, f.a.SocietyID)
	if err != nil {
		t.Fatal(err)
	}
	if counts["new"] != 1 {
		t.Errorf("counts = %v", counts)
	}

	if err := f.store.SetStatus(ctx, id, f.adminA, "contacted", "called, will visit"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	list, _ = f.store.List(ctx, f.a.SocietyID, "contacted", 50)
	if len(list) != 1 || list[0].Notes != "called, will visit" {
		t.Errorf("after SetStatus: %+v", list)
	}

	// Filtering must actually filter.
	if l, _ := f.store.List(ctx, f.a.SocietyID, "new", 50); len(l) != 0 {
		t.Errorf("expected no rows still marked new, got %d", len(l))
	}

	if err := f.store.SetStatus(ctx, uuid.New(), f.adminA, "lost", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetStatus on a missing row should be ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------- handler

func TestCreateEnquiryHappyPath(t *testing.T) {
	f := newFixture(t)

	payload := `{"societySlug":"` + f.a.Slug + `","plotId":"` + f.plotA.String() +
		`","name":"Sagar Waghmare","phone":"+91 98220 41190","email":"s@example.in",` +
		`"message":"Is plot 1 open?","budget":"40-50 L","source":"plot_detail"}`

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/public/enquiries", strings.NewReader(payload))
	if err := f.handler.createEnquiry(rec, r); err != nil {
		t.Fatalf("createEnquiry: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	// A guest must not be handed an id they could probe with.
	if _, present := body(t, rec)["id"]; present {
		t.Error("the response returned an enquiry id to an anonymous caller")
	}

	list, _ := f.store.List(context.Background(), f.a.SocietyID, "", 10)
	if len(list) != 1 || list[0].Source != "plot_detail" {
		t.Fatalf("the enquiry was not stored as expected: %+v", list)
	}
}

func TestCreateEnquiryDefaultsSourceAndIgnoresBadPlot(t *testing.T) {
	f := newFixture(t)

	payload := `{"societySlug":"` + f.a.Slug + `","plotId":"not-a-uuid",` +
		`"name":"Meera","phone":"9960423117"}`
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/public/enquiries", strings.NewReader(payload))
	if err := f.handler.createEnquiry(rec, r); err != nil {
		t.Fatalf("createEnquiry: %v", err)
	}

	list, _ := f.store.List(context.Background(), f.a.SocietyID, "", 10)
	if len(list) != 1 {
		t.Fatalf("got %d rows", len(list))
	}
	if list[0].Source != "layout_map" {
		t.Errorf("source = %q, want the layout_map default", list[0].Source)
	}
	if list[0].PlotID != nil {
		t.Error("an unparseable plot id should be dropped, not stored")
	}
}

func TestCreateEnquiryValidation(t *testing.T) {
	f := newFixture(t)

	cases := map[string]string{
		"short name":   `{"societySlug":"` + f.a.Slug + `","name":"S","phone":"9822041190"}`,
		"bad phone":    `{"societySlug":"` + f.a.Slug + `","name":"Sagar","phone":"12345"}`,
		"long message": `{"societySlug":"` + f.a.Slug + `","name":"Sagar","phone":"9822041190","message":"` + strings.Repeat("x", 2001) + `"}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(payload))
			if got := status(t, f.handler.createEnquiry(rec, r)); got != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422", got)
			}
		})
	}

	t.Run("malformed json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{`))
		if got := status(t, f.handler.createEnquiry(rec, r)); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("unknown society", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/x",
			strings.NewReader(`{"societySlug":"nope","name":"Sagar","phone":"9822041190"}`))
		if got := status(t, f.handler.createEnquiry(rec, r)); got != http.StatusNotFound {
			t.Errorf("status = %d, want 404", got)
		}
	})
}

// The enquiry endpoint is the only unauthenticated write in the app.
func TestCreateEnquiryIsRateLimited(t *testing.T) {
	f := newFixture(t)
	payload := `{"societySlug":"` + f.a.Slug + `","name":"Spammer","phone":"9822041190"}`

	newReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(payload))
		r.RemoteAddr = "203.0.113.50:40000"
		r.Header.Set("User-Agent", "bot/1")
		return r
	}

	for i := 0; i < 5; i++ {
		if err := f.handler.createEnquiry(httptest.NewRecorder(), newReq()); err != nil {
			t.Fatalf("request %d should be allowed: %v", i+1, err)
		}
	}
	if got := status(t, f.handler.createEnquiry(httptest.NewRecorder(), newReq())); got != http.StatusTooManyRequests {
		t.Fatalf("the sixth request status = %d, want 429", got)
	}
}

func TestPublicSocietyHandlers(t *testing.T) {
	f := newFixture(t)

	t.Run("first public", func(t *testing.T) {
		rec := httptest.NewRecorder()
		if err := f.handler.publicSociety(rec, httptest.NewRequest(http.MethodGet, "/x", nil)); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("by slug", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("slug", f.a.Slug)
		if err := f.handler.publicSocietyBySlug(rec, r); err != nil {
			t.Fatal(err)
		}
		if body(t, rec)["slug"] != f.a.Slug {
			t.Error("wrong society returned")
		}
	})

	t.Run("unknown slug", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("slug", "nope")
		if got := status(t, f.handler.publicSocietyBySlug(rec, r)); got != http.StatusNotFound {
			t.Errorf("status = %d, want 404", got)
		}
	})

	t.Run("nothing listed", func(t *testing.T) {
		if _, err := f.db.Exec(context.Background(), `UPDATE societies SET public_listing = false`); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		if got := status(t, f.handler.publicSociety(rec, httptest.NewRequest(http.MethodGet, "/x", nil))); got != http.StatusNotFound {
			t.Errorf("status = %d, want 404", got)
		}
	})
}

// The whole point of the guard: builder B must not read or work builder A's leads.
func TestLeadsAreScopedToTheirBuilder(t *testing.T) {
	f := newFixture(t)
	enquiryID := testsupport.NewEnquiry(t, f.db, f.a.SocietyID, &f.plotA, "Guest")

	t.Run("own builder can list", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		if err := f.handler.listEnquiries(rec, asStaff(r, f.a.ID, f.adminA)); err != nil {
			t.Fatalf("the owning builder was denied: %v", err)
		}
		list, _ := body(t, rec)["enquiries"].([]any)
		if len(list) != 1 {
			t.Errorf("got %d leads, want 1", len(list))
		}
	})

	t.Run("other builder cannot list", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		if got := status(t, f.handler.listEnquiries(rec, asStaff(r, f.b.ID, f.adminB))); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 — another builder read these leads", got)
		}
	})

	t.Run("other builder cannot update", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"status":"lost"}`))
		r.SetPathValue("enquiryId", enquiryID.String())
		if got := status(t, f.handler.updateEnquiry(rec, asStaff(r, f.b.ID, f.adminB))); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 — another builder moved this lead", got)
		}
	})

	t.Run("own builder can update", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"status":"contacted","notes":"called"}`))
		r.SetPathValue("enquiryId", enquiryID.String())
		if err := f.handler.updateEnquiry(rec, asStaff(r, f.a.ID, f.adminA)); err != nil {
			t.Fatalf("the owning builder was denied: %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", rec.Code)
		}
	})
}

func TestEnquiryHandlerBadInput(t *testing.T) {
	f := newFixture(t)
	enquiryID := testsupport.NewEnquiry(t, f.db, f.a.SocietyID, nil, "Guest")

	t.Run("bad society id", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("societyId", "nope")
		if got := status(t, f.handler.listEnquiries(httptest.NewRecorder(), asStaff(r, f.a.ID, f.adminA))); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("bad status filter", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x?status=whatever", nil)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		if got := status(t, f.handler.listEnquiries(httptest.NewRecorder(), asStaff(r, f.a.ID, f.adminA))); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("bad enquiry id", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"status":"lost"}`))
		r.SetPathValue("enquiryId", "nope")
		if got := status(t, f.handler.updateEnquiry(httptest.NewRecorder(), asStaff(r, f.a.ID, f.adminA))); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})

	t.Run("unknown status", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"status":"won"}`))
		r.SetPathValue("enquiryId", enquiryID.String())
		if got := status(t, f.handler.updateEnquiry(httptest.NewRecorder(), asStaff(r, f.a.ID, f.adminA))); got != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422", got)
		}
	})

	t.Run("malformed patch body", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{`))
		r.SetPathValue("enquiryId", enquiryID.String())
		if got := status(t, f.handler.updateEnquiry(httptest.NewRecorder(), asStaff(r, f.a.ID, f.adminA))); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})
}

func TestRoutesAreRegistered(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	f.handler.Routes(mux, auth.NewAuthenticator(
		auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), 0, 0)))

	// The guest routes answer without a token.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/society", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/public/society status = %d, want 200 for a guest", rec.Code)
	}

	// The staff routes do not.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/societies/"+f.a.SocietyID.String()+"/enquiries", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("the staff route status = %d, want 401 without a token", rec.Code)
	}
}


// dbCtx is a plain background context, for tests that set fixtures up directly.
func (f fixture) dbCtx() context.Context { return context.Background() }
