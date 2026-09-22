package query

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
	owner  uuid.UUID
	plot   uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testsupport.DB(t, "query")
	store := NewStore(db)

	f := fixture{db: db, store: store, h: NewHandler(store, access.NewGuard(db))}
	f.a = testsupport.NewBuilder(t, db, "Alpha")
	f.b = testsupport.NewBuilder(t, db, "Beta")
	f.adminA = testsupport.NewUser(t, db, &f.a.ID, domain.RoleBuilderAdmin, "a@alpha.in", "")
	f.adminB = testsupport.NewUser(t, db, &f.b.ID, domain.RoleBuilderAdmin, "b@beta.in", "")
	f.owner = testsupport.NewUser(t, db, nil, domain.RoleOwner, "o@example.in", "")
	f.plot = testsupport.NewPlot(t, db, f.a.SocietyID, "147", "sold")
	testsupport.AssignPlot(t, db, f.plot, f.owner)
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

// The SLA clock is the reason this module exists, so it must be set from the
// category rather than left at zero.
func TestCreateSetsTheSLAFromTheCategory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot,
		"documents_legal", "Sale deed missing", "Registered in March.")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	q, err := f.store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if q.SLADueAt == nil {
		t.Fatal("no SLA deadline was set")
	}
	want := time.Now().Add(domain.QuerySLA["documents_legal"])
	if diff := q.SLADueAt.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("SLA due at %v, expected about %v", q.SLADueAt, want)
	}
	if q.Breached {
		t.Error("a query should not be breached the moment it is raised")
	}

	// The body becomes the opening message, so the thread reads in one place.
	msgs, err := f.store.Messages(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Body != "Registered in March." {
		t.Errorf("opening message = %+v", msgs)
	}
}

func TestIsBreached(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	resolved := time.Now()

	cases := map[string]struct {
		q    Query
		want bool
	}{
		"open and overdue":      {Query{SLADueAt: &past, Status: "open"}, true},
		"open and in time":      {Query{SLADueAt: &future, Status: "open"}, false},
		"no deadline":           {Query{Status: "open"}, false},
		"resolved late":         {Query{SLADueAt: &past, Status: "resolved", ResolvedAt: &resolved}, false},
		"closed late":           {Query{SLADueAt: &past, Status: "closed"}, false},
		"marked resolved only":  {Query{SLADueAt: &past, Status: "resolved"}, false},
	}
	for name, c := range cases {
		if got := isBreached(c.q); got != c.want {
			t.Errorf("%s: isBreached = %v, want %v", name, got, c.want)
		}
	}
}

// The builder's inbox is useless if a breached query is not at the top.
func TestListForSocietySortsBreachedFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	fresh, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "site_visit", "Recent", "body")
	if err != nil {
		t.Fatal(err)
	}
	late, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "documents_legal", "Late", "body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx,
		`UPDATE queries SET sla_due_at = now() - interval '2 days', created_at = now() - interval '9 days' WHERE id = $1`,
		late); err != nil {
		t.Fatal(err)
	}

	list, err := f.store.ListForSociety(ctx, f.a.SocietyID, "", 50)
	if err != nil {
		t.Fatalf("ListForSociety: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d queries", len(list))
	}
	if list[0].ID != late {
		t.Error("the breached query is not first in the inbox")
	}
	if !list[0].Breached {
		t.Error("the overdue query is not flagged as breached")
	}
	_ = fresh

	// The status filter must actually filter.
	if l, _ := f.store.ListForSociety(ctx, f.a.SocietyID, "resolved", 50); len(l) != 0 {
		t.Errorf("expected no resolved queries, got %d", len(l))
	}
}

func TestListForOwnerAndGetMissing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "other", "Mine", "b"); err != nil {
		t.Fatal(err)
	}
	stranger := testsupport.NewUser(t, f.db, nil, domain.RoleOwner, "s@example.in", "")

	mine, err := f.store.ListForOwner(ctx, f.owner, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 {
		t.Errorf("the owner sees %d queries, want 1", len(mine))
	}
	theirs, err := f.store.ListForOwner(ctx, stranger, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 0 {
		t.Errorf("a stranger sees %d of someone else's queries", len(theirs))
	}

	if _, err := f.store.Get(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAddMessageAndInternalNotes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "infrastructure", "Light out", "body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AddMessage(ctx, id, f.adminA, "Contractor visiting Thursday.", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AddMessage(ctx, id, f.adminA, "Chase the electrician.", "", true); err != nil {
		t.Fatal(err)
	}

	ownerView, err := f.store.Messages(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ownerView {
		if m.IsInternal {
			t.Fatal("an internal note reached the owner's thread")
		}
	}
	if len(ownerView) != 2 {
		t.Errorf("the owner sees %d messages, want 2", len(ownerView))
	}

	staffView, err := f.store.Messages(ctx, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(staffView) != 3 {
		t.Errorf("staff see %d messages, want 3", len(staffView))
	}
}

func TestSetStatusStampsResolvedAt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "other", "s", "b")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetStatus(ctx, id, "resolved", &f.adminA); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	q, _ := f.store.Get(ctx, id)
	if q.ResolvedAt == nil {
		t.Fatal("resolved_at was not stamped")
	}
	first := *q.ResolvedAt

	// Moving to closed must not restamp the original resolution time.
	if err := f.store.SetStatus(ctx, id, "closed", nil); err != nil {
		t.Fatal(err)
	}
	q, _ = f.store.Get(ctx, id)
	if q.ResolvedAt == nil || !q.ResolvedAt.Equal(first) {
		t.Error("resolved_at was overwritten")
	}

	// Reopening clears it.
	if err := f.store.SetStatus(ctx, id, "open", nil); err != nil {
		t.Fatal(err)
	}
	q, _ = f.store.Get(ctx, id)
	if q.ResolvedAt != nil {
		t.Error("reopening should clear resolved_at")
	}

	if err := f.store.SetStatus(ctx, uuid.New(), "open", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ----------------------------------------------------------------- handler

func TestCategoriesEndpointIsStable(t *testing.T) {
	f := newFixture(t)
	rec := httptest.NewRecorder()
	if err := f.h.categories(rec, httptest.NewRequest(http.MethodGet, "/x", nil)); err != nil {
		t.Fatal(err)
	}
	list, _ := jsonBody(t, rec)["categories"].([]any)
	if len(list) != len(domain.QuerySLA) {
		t.Fatalf("got %d categories, want %d", len(list), len(domain.QuerySLA))
	}
	// The order is fixed so the form does not reshuffle between page loads.
	first, _ := list[0].(map[string]any)
	if first["key"] != "documents_legal" {
		t.Errorf("first category = %v", first["key"])
	}
	if first["slaDays"] != float64(7) {
		t.Errorf("slaDays = %v", first["slaDays"])
	}
}

func TestCreateHandler(t *testing.T) {
	f := newFixture(t)
	owner := ident(f.owner, nil, domain.RoleOwner)

	r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(
		`{"plotId":"`+f.plot.String()+`","category":"documents_legal","subject":"Deed","body":"Please send it."}`)), owner)
	r.SetPathValue("societyId", f.a.SocietyID.String())
	rec := httptest.NewRecorder()
	if err := f.h.create(rec, r); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateHandlerValidation(t *testing.T) {
	f := newFixture(t)
	owner := ident(f.owner, nil, domain.RoleOwner)

	cases := map[string]struct {
		body   string
		status int
	}{
		"no subject":     {`{"category":"other","subject":"  ","body":"x"}`, http.StatusUnprocessableEntity},
		"long subject":   {`{"category":"other","subject":"` + strings.Repeat("s", 201) + `","body":"x"}`, http.StatusUnprocessableEntity},
		"no body":        {`{"category":"other","subject":"s","body":" "}`, http.StatusUnprocessableEntity},
		"bad category":   {`{"category":"nonsense","subject":"s","body":"b"}`, http.StatusUnprocessableEntity},
		"bad plot id":    {`{"plotId":"nope","category":"other","subject":"s","body":"b"}`, http.StatusUnprocessableEntity},
		"malformed":      {`{`, http.StatusBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(c.body)), owner)
			r.SetPathValue("societyId", f.a.SocietyID.String())
			if got := apiStatus(t, f.h.create(httptest.NewRecorder(), r)); got != c.status {
				t.Errorf("status = %d, want %d", got, c.status)
			}
		})
	}

	t.Run("bad society id", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"category":"other","subject":"s","body":"b"}`)), owner)
		r.SetPathValue("societyId", "nope")
		if got := apiStatus(t, f.h.create(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
			t.Errorf("status = %d", got)
		}
	})
}

// An owner must not be able to read another owner's thread, and the refusal
// must look identical to a genuine miss so ids cannot be probed.
func TestGetHandlerScoping(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "other", "s", "b")
	if err != nil {
		t.Fatal(err)
	}
	stranger := testsupport.NewUser(t, f.db, nil, domain.RoleOwner, "s@example.in", "")

	get := func(who auth.Identity, queryID string) error {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), who)
		r.SetPathValue("queryId", queryID)
		return f.h.get(httptest.NewRecorder(), r)
	}

	if err := get(ident(f.owner, nil, domain.RoleOwner), id.String()); err != nil {
		t.Errorf("the author was denied their own query: %v", err)
	}
	if got := apiStatus(t, get(ident(stranger, nil, domain.RoleOwner), id.String())); got != http.StatusNotFound {
		t.Errorf("a stranger got status %d, want 404", got)
	}
	if got := apiStatus(t, get(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), id.String())); got != http.StatusNotFound {
		t.Errorf("another builder's staff got status %d, want 404", got)
	}
	if err := get(ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin), id.String()); err != nil {
		t.Errorf("the owning builder was denied: %v", err)
	}
	if got := apiStatus(t, get(ident(f.owner, nil, domain.RoleOwner), "nope")); got != http.StatusBadRequest {
		t.Errorf("bad id status = %d", got)
	}
	if got := apiStatus(t, get(ident(f.owner, nil, domain.RoleOwner), uuid.NewString())); got != http.StatusNotFound {
		t.Errorf("missing id status = %d", got)
	}
}

func TestAddMessageHandler(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "other", "s", "b")
	if err != nil {
		t.Fatal(err)
	}
	stranger := testsupport.NewUser(t, f.db, nil, domain.RoleOwner, "s@example.in", "")

	send := func(who auth.Identity, body string) error {
		r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), who)
		r.SetPathValue("queryId", id.String())
		return f.h.addMessage(httptest.NewRecorder(), r)
	}

	if err := send(ident(f.owner, nil, domain.RoleOwner), `{"body":"any update?"}`); err != nil {
		t.Fatalf("the author should be able to post: %v", err)
	}
	if got := apiStatus(t, send(ident(stranger, nil, domain.RoleOwner), `{"body":"hi"}`)); got != http.StatusNotFound {
		t.Errorf("a stranger posted to someone else's thread: status %d", got)
	}
	if got := apiStatus(t, send(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), `{"body":"hi"}`)); got != http.StatusNotFound {
		t.Errorf("another builder posted: status %d", got)
	}
	if got := apiStatus(t, send(ident(f.owner, nil, domain.RoleOwner), `{"body":"  "}`)); got != http.StatusUnprocessableEntity {
		t.Errorf("an empty message status = %d", got)
	}
	if got := apiStatus(t, send(ident(f.owner, nil, domain.RoleOwner), `{`)); got != http.StatusBadRequest {
		t.Errorf("malformed status = %d", got)
	}

	// The stored-XSS guard.
	if got := apiStatus(t, send(ident(f.owner, nil, domain.RoleOwner),
		`{"body":"see this","attachmentUrl":"javascript:alert(1)"}`)); got != http.StatusUnprocessableEntity {
		t.Fatalf("a javascript: attachment URL was accepted: status %d", got)
	}
	if err := send(ident(f.owner, nil, domain.RoleOwner),
		`{"body":"see this","attachmentUrl":"https://pub.r2.dev/a.pdf"}`); err != nil {
		t.Errorf("an https attachment should be accepted: %v", err)
	}
}

// An owner must not be able to hide their own message from themselves.
func TestOnlyStaffCanLeaveInternalNotes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "other", "s", "b")
	if err != nil {
		t.Fatal(err)
	}

	r := as(httptest.NewRequest(http.MethodPost, "/x",
		strings.NewReader(`{"body":"sneaky","isInternal":true}`)), ident(f.owner, nil, domain.RoleOwner))
	r.SetPathValue("queryId", id.String())
	if err := f.h.addMessage(httptest.NewRecorder(), r); err != nil {
		t.Fatal(err)
	}

	msgs, err := f.store.Messages(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.Body == "sneaky" && m.IsInternal {
			t.Fatal("an owner managed to mark their own message internal")
		}
	}
}

func TestListHandlersAndSetStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "other", "s", "b")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("mine", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.owner, nil, domain.RoleOwner))
		rec := httptest.NewRecorder()
		if err := f.h.listMine(rec, r); err != nil {
			t.Fatal(err)
		}
		list, _ := jsonBody(t, rec)["queries"].([]any)
		if len(list) != 1 {
			t.Errorf("got %d", len(list))
		}
	})

	t.Run("society inbox is scoped", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin))
		r.SetPathValue("societyId", f.a.SocietyID.String())
		if got := apiStatus(t, f.h.listForSociety(httptest.NewRecorder(), r)); got != http.StatusNotFound {
			t.Fatalf("another builder read the inbox: status %d", got)
		}

		r = as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))
		r.SetPathValue("societyId", f.a.SocietyID.String())
		if err := f.h.listForSociety(httptest.NewRecorder(), r); err != nil {
			t.Fatalf("the owning builder was denied: %v", err)
		}
	})

	t.Run("inbox bad input", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))
		r.SetPathValue("societyId", "nope")
		if got := apiStatus(t, f.h.listForSociety(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
			t.Errorf("status = %d", got)
		}

		r = as(httptest.NewRequest(http.MethodGet, "/x?status=whatever", nil), ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))
		r.SetPathValue("societyId", f.a.SocietyID.String())
		if got := apiStatus(t, f.h.listForSociety(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
			t.Errorf("bad filter status = %d", got)
		}
	})

	t.Run("set status", func(t *testing.T) {
		patch := func(who auth.Identity, queryID, body string) error {
			r := as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(body)), who)
			r.SetPathValue("queryId", queryID)
			return f.h.setStatus(httptest.NewRecorder(), r)
		}
		admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

		if got := apiStatus(t, patch(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), id.String(), `{"status":"closed"}`)); got != http.StatusNotFound {
			t.Errorf("another builder closed the query: status %d", got)
		}
		if err := patch(admin, id.String(), `{"status":"in_progress","assignedTo":"`+f.adminA.String()+`"}`); err != nil {
			t.Fatalf("setStatus: %v", err)
		}
		if got := apiStatus(t, patch(admin, id.String(), `{"status":"finished"}`)); got != http.StatusUnprocessableEntity {
			t.Errorf("unknown status = %d", got)
		}
		if got := apiStatus(t, patch(admin, id.String(), `{"status":"open","assignedTo":"nope"}`)); got != http.StatusUnprocessableEntity {
			t.Errorf("bad assignee = %d", got)
		}
		if got := apiStatus(t, patch(admin, "nope", `{"status":"open"}`)); got != http.StatusBadRequest {
			t.Errorf("bad id = %d", got)
		}
		if got := apiStatus(t, patch(admin, id.String(), `{`)); got != http.StatusBadRequest {
			t.Errorf("malformed = %d", got)
		}
	})
}

func TestQueryRoutesAreRegistered(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	f.h.Routes(mux, auth.NewAuthenticator(
		auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Minute, time.Hour)))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/categories", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("categories should be public: status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/queries/mine", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("mine should need a token: status %d", rec.Code)
	}
}
