package fund

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
	db := testsupport.DB(t, "fund")
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

func today() string { return time.Now().Format("2006-01-02") }

// ------------------------------------------------------------------- store

func TestCreateListAndBalance(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewEntry{
		EntryDate: today(), Head: "collections", Description: "Q1 collected", Credit: 300000,
	}); err != nil {
		t.Fatalf("Create credit: %v", err)
	}
	if _, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewEntry{
		EntryDate: today(), Head: "security", Description: "Guards", Debit: 66000,
	}); err != nil {
		t.Fatalf("Create debit: %v", err)
	}

	entries, err := f.store.List(ctx, f.a.SocietyID, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries", len(entries))
	}

	balance, err := f.store.Balance(ctx, f.a.SocietyID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance.TotalCredit != 300000 || balance.TotalDebit != 66000 || balance.Closing != 234000 {
		t.Errorf("balance = %+v", balance)
	}
	if balance.ByHead["security"] != 66000 {
		t.Errorf("byHead = %v", balance.ByHead)
	}
	// Collections are money in, so they must not appear as spending.
	if _, present := balance.ByHead["collections"]; present {
		t.Error("a credit head leaked into the spending breakdown")
	}
}

// The ledger is append-only: a correction is a reversal row, and the pair nets
// to zero. This is the property the whole module exists to guarantee.
func TestReverseIsAppendOnlyAndSingleUse(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewEntry{
		EntryDate: today(), Head: "roads", Description: "tar patch", Debit: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}

	reversalID, err := f.store.Reverse(ctx, id, f.adminA, "Reversal: wrong head")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}

	balance, err := f.store.Balance(ctx, f.a.SocietyID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Closing != 0 {
		t.Errorf("closing = %v, want 0 after a reversal", balance.Closing)
	}

	// The original row is still there, untouched.
	entries, _ := f.store.List(ctx, f.a.SocietyID, 50)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want the original plus its reversal", len(entries))
	}
	var linked bool
	for _, e := range entries {
		if e.ID == reversalID {
			if e.ReversesID == nil || *e.ReversesID != id {
				t.Error("the reversal is not linked to the original")
			}
			if e.Credit != 5000 || e.Debit != 0 {
				t.Errorf("the reversal is not the mirror image: %+v", e)
			}
			linked = true
		}
	}
	if !linked {
		t.Error("the reversal row was not found")
	}

	// Reversing twice must not silently double-count.
	if _, err := f.store.Reverse(ctx, id, f.adminA, "again"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a second reversal was allowed: %v", err)
	}
	// A reversal cannot itself be reversed.
	if _, err := f.store.Reverse(ctx, reversalID, f.adminA, "undo the undo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a reversal was itself reversed: %v", err)
	}
	if _, err := f.store.Reverse(ctx, uuid.New(), f.adminA, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestOwnerInSociety(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	ok, err := f.store.OwnerInSociety(ctx, f.owner, f.a.SocietyID)
	if err != nil || !ok {
		t.Fatalf("the plot's owner should be in the society: %v %v", ok, err)
	}
	ok, err = f.store.OwnerInSociety(ctx, f.owner, f.b.SocietyID)
	if err != nil || ok {
		t.Fatalf("the owner should not be in another builder's society: %v %v", ok, err)
	}
}

// ----------------------------------------------------------------- handler

// Any signed-in user reading any builder's accounts would be a serious leak.
func TestListHandlerScoping(t *testing.T) {
	f := newFixture(t)
	testsupport.NewFundEntry(t, f.db, f.a.SocietyID, f.adminA, "roads", 0, 1000)

	list := func(who auth.Identity, societyID string) (*httptest.ResponseRecorder, error) {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), who)
		r.SetPathValue("societyId", societyID)
		rec := httptest.NewRecorder()
		return rec, f.h.list(rec, r)
	}

	t.Run("the society's own owner can read it", func(t *testing.T) {
		rec, err := list(ident(f.owner, nil, domain.RoleOwner), f.a.SocietyID.String())
		if err != nil {
			t.Fatalf("the owner was denied: %v", err)
		}
		if _, present := jsonBody(t, rec)["balance"]; !present {
			t.Error("no balance in the response")
		}
	})

	t.Run("an owner elsewhere cannot", func(t *testing.T) {
		_, err := list(ident(f.owner, nil, domain.RoleOwner), f.b.SocietyID.String())
		if got := apiStatus(t, err); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
	})

	t.Run("a user with no plot at all cannot", func(t *testing.T) {
		stranger := testsupport.NewUser(t, f.db, nil, domain.RoleOwner, "s@example.in", "")
		_, err := list(ident(stranger, nil, domain.RoleOwner), f.a.SocietyID.String())
		if got := apiStatus(t, err); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
	})

	t.Run("the owning builder can", func(t *testing.T) {
		if _, err := list(ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin), f.a.SocietyID.String()); err != nil {
			t.Fatalf("denied: %v", err)
		}
	})

	t.Run("another builder cannot", func(t *testing.T) {
		_, err := list(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), f.a.SocietyID.String())
		if got := apiStatus(t, err); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 — another builder read these accounts", got)
		}
	})

	t.Run("bad society id", func(t *testing.T) {
		_, err := list(ident(f.owner, nil, domain.RoleOwner), "nope")
		if got := apiStatus(t, err); got != http.StatusBadRequest {
			t.Errorf("status = %d", got)
		}
	})
}

func TestCreateHandlerValidation(t *testing.T) {
	f := newFixture(t)
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	create := func(who auth.Identity, societyID, body string) error {
		r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), who)
		r.SetPathValue("societyId", societyID)
		return f.h.create(httptest.NewRecorder(), r)
	}

	good := `{"entryDate":"` + today() + `","head":"security","description":"Guards","credit":0,"debit":66000}`
	if err := create(admin, f.a.SocietyID.String(), good); err != nil {
		t.Fatalf("a valid entry was rejected: %v", err)
	}

	// The guard: builder B writing into builder A's ledger.
	if got := apiStatus(t, create(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), f.a.SocietyID.String(), good)); got != http.StatusNotFound {
		t.Fatalf("another builder wrote to this ledger: status %d", got)
	}

	cases := map[string]struct {
		body   string
		status int
	}{
		"no head":        {`{"entryDate":"` + today() + `","head":" ","description":"d","debit":1}`, http.StatusUnprocessableEntity},
		"no description": {`{"entryDate":"` + today() + `","head":"h","description":" ","debit":1}`, http.StatusUnprocessableEntity},
		"bad date":       {`{"entryDate":"31-12-2026","head":"h","description":"d","debit":1}`, http.StatusUnprocessableEntity},
		"both sides":     {`{"entryDate":"` + today() + `","head":"h","description":"d","credit":1,"debit":1}`, http.StatusUnprocessableEntity},
		"neither side":   {`{"entryDate":"` + today() + `","head":"h","description":"d"}`, http.StatusUnprocessableEntity},
		"negative":       {`{"entryDate":"` + today() + `","head":"h","description":"d","debit":-5}`, http.StatusUnprocessableEntity},
		"unsafe doc url": {`{"entryDate":"` + today() + `","head":"h","description":"d","debit":1,"documentUrl":"javascript:alert(1)"}`, http.StatusUnprocessableEntity},
		"malformed":      {`{`, http.StatusBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := apiStatus(t, create(admin, f.a.SocietyID.String(), c.body)); got != c.status {
				t.Errorf("status = %d, want %d", got, c.status)
			}
		})
	}

	t.Run("bad society id", func(t *testing.T) {
		if got := apiStatus(t, create(admin, "nope", good)); got != http.StatusBadRequest {
			t.Errorf("status = %d", got)
		}
	})
}

func TestReverseHandler(t *testing.T) {
	f := newFixture(t)
	entryID := testsupport.NewFundEntry(t, f.db, f.a.SocietyID, f.adminA, "roads", 0, 5000)

	reverse := func(who auth.Identity, id, body string) (*httptest.ResponseRecorder, error) {
		r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), who)
		r.SetPathValue("entryId", id)
		rec := httptest.NewRecorder()
		return rec, f.h.reverse(rec, r)
	}
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	if _, err := reverse(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin), entryID.String(), `{"reason":"mine now"}`); apiStatus(t, err) != http.StatusNotFound {
		t.Fatal("another builder reversed this entry")
	}
	if _, err := reverse(admin, "nope", `{"reason":"x"}`); apiStatus(t, err) != http.StatusBadRequest {
		t.Error("a bad id should be 400")
	}
	if _, err := reverse(admin, entryID.String(), `{"reason":"  "}`); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("a reversal needs a reason")
	}
	if _, err := reverse(admin, entryID.String(), `{`); apiStatus(t, err) != http.StatusBadRequest {
		t.Error("malformed should be 400")
	}

	rec, err := reverse(admin, entryID.String(), `{"reason":"wrong head"}`)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if _, err := reverse(admin, entryID.String(), `{"reason":"again"}`); apiStatus(t, err) != http.StatusConflict {
		t.Error("a second reversal should be a 409")
	}
}

func TestFundRoutesAreRegistered(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	f.h.Routes(mux, auth.NewAuthenticator(
		auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Minute, time.Hour)))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/societies/"+f.a.SocietyID.String()+"/fund", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("the ledger should need a token: status %d", rec.Code)
	}
}
