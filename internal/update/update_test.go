package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/access"
	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

type fixture struct {
	db     *database.DB
	store  *Store
	h      *Handler
	a, b   testsupport.Builder
	adminA uuid.UUID
	adminB uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testsupport.DB(t, "update")
	store := NewStore(db)

	f := fixture{db: db, store: store, h: NewHandler(store, access.NewGuard(db))}
	f.a = testsupport.NewBuilder(t, db, "Alpha")
	f.b = testsupport.NewBuilder(t, db, "Beta")
	f.adminA = testsupport.NewUser(t, db, &f.a.ID, domain.RoleBuilderAdmin, "a@alpha.in", "")
	f.adminB = testsupport.NewUser(t, db, &f.b.ID, domain.RoleBuilderAdmin, "b@beta.in", "")
	return f
}

func ident(userID uuid.UUID, builderID *uuid.UUID, role domain.Role) auth.Identity {
	return auth.Identity{UserID: userID, BuilderID: builderID, Role: role}
}

func as(r *http.Request, id auth.Identity) *http.Request {
	return r.WithContext(auth.ContextWithIdentity(r.Context(), id))
}

func apiStatus(t *testing.T, err error) int {
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

func jsonBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// ------------------------------------------------------------------- store

func TestCreateWithMediaAndList(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewPost{
		Title: "Roads cast", Body: "All Sector 01 roads are cast.", Phase: "Sector 01",
		Publish: true,
		Media: []Media{
			{URL: "https://pub.r2.dev/a.jpg", Caption: "north"},
			{URL: "https://pub.r2.dev/b.jpg"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	posts, err := f.store.List(ctx, f.a.SocietyID, false, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != id {
		t.Fatalf("got %d posts", len(posts))
	}
	if len(posts[0].Media) != 2 {
		t.Errorf("got %d media, want 2", len(posts[0].Media))
	}
	// Media come back in the order they were attached.
	if posts[0].Media[0].Caption != "north" {
		t.Errorf("media order is wrong: %+v", posts[0].Media)
	}
	if posts[0].AuthorName == "" {
		t.Error("the author name was not joined in")
	}
}

// A draft is for the site office only; an owner must not see unpublished work.
func TestDraftsAreStaffOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewPost{Title: "Draft", Publish: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewPost{Title: "Live", Publish: true}); err != nil {
		t.Fatal(err)
	}

	ownerView, err := f.store.List(ctx, f.a.SocietyID, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerView) != 1 || ownerView[0].Title != "Live" {
		t.Errorf("an owner saw %d posts: %+v", len(ownerView), ownerView)
	}

	staffView, err := f.store.List(ctx, f.a.SocietyID, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(staffView) != 2 {
		t.Errorf("staff saw %d posts, want 2", len(staffView))
	}
}

func TestListEmpty(t *testing.T) {
	f := newFixture(t)
	posts, err := f.store.List(context.Background(), f.a.SocietyID, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Errorf("got %d posts from an empty society", len(posts))
	}
}

func TestPublish(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewPost{Title: "Draft", Publish: false})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Publish(ctx, id); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	posts, _ := f.store.List(ctx, f.a.SocietyID, false, 10)
	if len(posts) != 1 {
		t.Fatal("the post was not published")
	}
	first := posts[0].PublishedAt

	// Publishing again must not move the original publication time.
	if err := f.store.Publish(ctx, id); err != nil {
		t.Fatal(err)
	}
	posts, _ = f.store.List(ctx, f.a.SocietyID, false, 10)
	if first == nil || posts[0].PublishedAt == nil || !posts[0].PublishedAt.Equal(*first) {
		t.Error("republishing changed the publication time")
	}

	if err := f.store.Publish(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ----------------------------------------------------------------- handler

func TestListHandlerShowsDraftsOnlyToStaff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewPost{Title: "Draft", Publish: false}); err != nil {
		t.Fatal(err)
	}

	count := func(r *http.Request) int {
		r.SetPathValue("societyId", f.a.SocietyID.String())
		rec := httptest.NewRecorder()
		if err := f.h.list(rec, r); err != nil {
			t.Fatalf("list: %v", err)
		}
		list, _ := jsonBody(t, rec)["updates"].([]any)
		return len(list)
	}

	if n := count(httptest.NewRequest(http.MethodGet, "/x", nil)); n != 0 {
		t.Errorf("a guest saw %d drafts", n)
	}
	if n := count(as(httptest.NewRequest(http.MethodGet, "/x", nil),
		ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))); n != 1 {
		t.Errorf("staff saw %d posts, want the draft", n)
	}

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("societyId", "nope")
	if got := apiStatus(t, f.h.list(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
		t.Errorf("status = %d", got)
	}
}

func TestCreateHandlerScopingAndValidation(t *testing.T) {
	f := newFixture(t)
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	create := func(who auth.Identity, societyID, body string) (*httptest.ResponseRecorder, error) {
		r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), who)
		r.SetPathValue("societyId", societyID)
		rec := httptest.NewRecorder()
		return rec, f.h.create(rec, r)
	}

	good := `{"title":"Roads cast","body":"done","phase":"Sector 01","publish":true,"media":[{"url":"https://pub.r2.dev/a.jpg","caption":"x"}]}`
	rec, err := create(admin, f.a.SocietyID.String(), good)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}

	if _, err := create(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), f.a.SocietyID.String(), good); apiStatus(t, err) != http.StatusNotFound {
		t.Fatal("another builder posted into this society's feed")
	}

	cases := map[string]struct {
		body   string
		status int
	}{
		"no title":      {`{"title":"   "}`, http.StatusUnprocessableEntity},
		"too much media": {`{"title":"t","media":[` + strings.TrimSuffix(strings.Repeat(`{"url":"https://a/b.jpg"},`, 21), ",") + `]}`, http.StatusUnprocessableEntity},
		"unsafe media":  {`{"title":"t","media":[{"url":"javascript:alert(1)"}]}`, http.StatusUnprocessableEntity},
		"empty media":   {`{"title":"t","media":[{"url":""}]}`, http.StatusUnprocessableEntity},
		"malformed":     {`{`, http.StatusBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := create(admin, f.a.SocietyID.String(), c.body); apiStatus(t, err) != c.status {
				t.Errorf("status = %d, want %d", apiStatus(t, err), c.status)
			}
		})
	}

	t.Run("bad society id", func(t *testing.T) {
		if _, err := create(admin, "nope", good); apiStatus(t, err) != http.StatusBadRequest {
			t.Error("expected 400")
		}
	})
}

func TestPublishHandler(t *testing.T) {
	f := newFixture(t)
	postID := testsupport.NewUpdate(t, f.db, f.a.SocietyID, f.adminA, "Draft", false)

	publish := func(who auth.Identity, id string) (*httptest.ResponseRecorder, error) {
		r := as(httptest.NewRequest(http.MethodPost, "/x", nil), who)
		r.SetPathValue("postId", id)
		rec := httptest.NewRecorder()
		return rec, f.h.publish(rec, r)
	}
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	if _, err := publish(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), postID.String()); apiStatus(t, err) != http.StatusNotFound {
		t.Fatal("another builder published this post")
	}
	if _, err := publish(admin, "nope"); apiStatus(t, err) != http.StatusBadRequest {
		t.Error("a bad id should be 400")
	}
	if _, err := publish(admin, uuid.NewString()); apiStatus(t, err) != http.StatusNotFound {
		t.Error("a missing post should be 404")
	}

	rec, err := publish(admin, postID.String())
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestUpdateRoutesAreRegistered(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	f.h.Routes(mux, auth.NewAuthenticator(
		auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Minute, time.Hour)))

	// The feed is open to guests.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/societies/"+f.a.SocietyID.String()+"/updates", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("guest feed status = %d, want 200", rec.Code)
	}

	// Posting is not.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/societies/"+f.a.SocietyID.String()+"/updates", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("posting status = %d, want 401", rec.Code)
	}
}
