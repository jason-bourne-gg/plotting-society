package maintenance

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
	db      *database.DB
	store   *Store
	h       *Handler
	a, b    testsupport.Builder
	adminA  uuid.UUID
	adminB  uuid.UUID
	owner   uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testsupport.DB(t, "maintenance")
	store := NewStore(db)

	f := fixture{db: db, store: store, h: NewHandler(store, access.NewGuard(db))}
	f.a = testsupport.NewBuilder(t, db, "Alpha")
	f.b = testsupport.NewBuilder(t, db, "Beta")
	f.adminA = testsupport.NewUser(t, db, &f.a.ID, domain.RoleBuilderAdmin, "a@alpha.in", "")
	f.adminB = testsupport.NewUser(t, db, &f.b.ID, domain.RoleBuilderAdmin, "b@beta.in", "")
	f.owner = testsupport.NewUser(t, db, nil, domain.RoleOwner, "o@example.in", "")

	for _, s := range []testsupport.Builder{f.a, f.b} {
		if _, err := f.db.Exec(context.Background(),
			`INSERT INTO maintenance_rates (society_id, sector, rate_per_sqft) VALUES ($1, NULL, 11.04)`,
			s.SocietyID); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f fixture) soldPlot(t *testing.T, no string) uuid.UUID {
	t.Helper()
	id := testsupport.NewPlot(t, f.db, f.a.SocietyID, no, "sold")
	testsupport.AssignPlot(t, f.db, id, f.owner)
	return id
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

func body(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// ------------------------------------------------------------------- store

func TestListFallsBackToTheDefaultRate(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")
	ctx := context.Background()

	rates, fallback, err := f.store.List(ctx, f.a.SocietyID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if fallback != 11.04 {
		t.Errorf("fallback = %v", fallback)
	}
	if len(rates) != 1 {
		t.Fatalf("got %d sectors", len(rates))
	}
	// A sector with no rate of its own must report the default, not zero —
	// otherwise the site office cannot see what an unset sector charges.
	if rates[0].RatePerSqft != 11.04 {
		t.Errorf("unset sector charges %v, want the 11.04 default", rates[0].RatePerSqft)
	}
	if rates[0].ReferenceAmount != amountFor(ReferenceAreaSqft, 11.04) {
		t.Errorf("reference = %v", rates[0].ReferenceAmount)
	}
}

func TestListWithoutAnyRate(t *testing.T) {
	db := testsupport.DB(t, "maintenance")
	b := testsupport.NewBuilder(t, db, "NoRate")
	if _, _, err := NewStore(db).List(context.Background(), b.SocietyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound with no default rate, got %v", err)
	}
}

func TestSetRateUpsertsSectorAndDefault(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")
	ctx := context.Background()

	if err := f.store.SetRate(ctx, f.a.SocietyID, "Sector 01", 12.50, f.adminA); err != nil {
		t.Fatalf("SetRate sector: %v", err)
	}
	// Upsert, not insert: setting the same sector twice must not conflict.
	if err := f.store.SetRate(ctx, f.a.SocietyID, "Sector 01", 13.00, f.adminA); err != nil {
		t.Fatalf("SetRate sector again: %v", err)
	}
	if err := f.store.SetRate(ctx, f.a.SocietyID, "", 9.00, f.adminA); err != nil {
		t.Fatalf("SetRate default: %v", err)
	}
	if err := f.store.SetRate(ctx, f.a.SocietyID, "", 9.50, f.adminA); err != nil {
		t.Fatalf("SetRate default again: %v", err)
	}

	rates, fallback, err := f.store.List(ctx, f.a.SocietyID)
	if err != nil {
		t.Fatal(err)
	}
	if fallback != 9.50 {
		t.Errorf("fallback = %v, want the updated 9.50", fallback)
	}
	if rates[0].RatePerSqft != 13.00 {
		t.Errorf("sector rate = %v, want the updated 13.00", rates[0].RatePerSqft)
	}

	// Exactly one default per society, enforced by a partial unique index —
	// a plain UNIQUE would let NULLs pile up because NULL != NULL.
	var defaults int
	if err := f.db.QueryRow(ctx,
		`SELECT count(*) FROM maintenance_rates WHERE society_id = $1 AND sector IS NULL`,
		f.a.SocietyID).Scan(&defaults); err != nil {
		t.Fatal(err)
	}
	if defaults != 1 {
		t.Errorf("%d default rates, want exactly 1", defaults)
	}
}

// A bill already issued must keep the rate it was raised at.
func TestGenerateBillsDoesNotRepriceExistingBills(t *testing.T) {
	f := newFixture(t)
	plot := f.soldPlot(t, "1")
	ctx := context.Background()

	if _, _, err := f.store.GenerateBills(ctx, f.a.SocietyID); err != nil {
		t.Fatal(err)
	}
	var first float64
	if err := f.db.QueryRow(ctx,
		`SELECT rate_per_sqft FROM maintenance_dues WHERE plot_id = $1`, plot).Scan(&first); err != nil {
		t.Fatal(err)
	}

	if err := f.store.SetRate(ctx, f.a.SocietyID, "Sector 01", 99.00, f.adminA); err != nil {
		t.Fatal(err)
	}
	raised, _, err := f.store.GenerateBills(ctx, f.a.SocietyID)
	if err != nil {
		t.Fatal(err)
	}
	if raised != 0 {
		t.Errorf("raised %d bills for an already-billed plot", raised)
	}

	var after float64
	if err := f.db.QueryRow(ctx,
		`SELECT rate_per_sqft FROM maintenance_dues WHERE plot_id = $1`, plot).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != first {
		t.Errorf("an issued bill was re-priced from %v to %v", first, after)
	}
}

func TestGenerateBillsUsesTheSectorRate(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")
	ctx := context.Background()

	if err := f.store.SetRate(ctx, f.a.SocietyID, "Sector 01", 20.00, f.adminA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.store.GenerateBills(ctx, f.a.SocietyID); err != nil {
		t.Fatal(err)
	}

	var rate, area, amount float64
	if err := f.db.QueryRow(ctx,
		`SELECT rate_per_sqft, area_sqft, amount_due FROM maintenance_dues`).Scan(&rate, &area, &amount); err != nil {
		t.Fatal(err)
	}
	if rate != 20.00 {
		t.Errorf("rate = %v, want the sector's 20.00 not the 11.04 default", rate)
	}
	if amount != amountFor(area, 20.00) {
		t.Errorf("amount = %v, want %v", amount, amountFor(area, 20.00))
	}
}

func TestAmountForRoundsToPaise(t *testing.T) {
	if got := amountFor(1130.22, 11.04); got != 12477.63 {
		t.Errorf("amountFor = %v, want 12477.63", got)
	}
	if got := amountFor(0, 11.04); got != 0 {
		t.Errorf("amountFor(0) = %v", got)
	}
}

// ----------------------------------------------------------------- handler

func TestListHandler(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")

	r := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))
	r.SetPathValue("societyId", f.a.SocietyID.String())
	rec := httptest.NewRecorder()
	if err := f.h.list(rec, r); err != nil {
		t.Fatalf("list: %v", err)
	}

	b := body(t, rec)
	if b["defaultRate"] != 11.04 {
		t.Errorf("defaultRate = %v", b["defaultRate"])
	}
	if b["referenceAreaSqft"] != ReferenceAreaSqft {
		t.Errorf("referenceAreaSqft = %v", b["referenceAreaSqft"])
	}
	if b["defaultReference"] != amountFor(ReferenceAreaSqft, 11.04) {
		t.Errorf("defaultReference = %v", b["defaultReference"])
	}
}

func TestHandlersAreScopedToTheirBuilder(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")
	intruder := ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin)

	listReq := as(httptest.NewRequest(http.MethodGet, "/x", nil), intruder)
	listReq.SetPathValue("societyId", f.a.SocietyID.String())
	if got := apiStatus(t, f.h.list(httptest.NewRecorder(), listReq)); got != http.StatusNotFound {
		t.Errorf("another builder listed these rates: status %d", got)
	}

	setReq := as(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{"sector":"Sector 01","ratePerSqft":1}`)), intruder)
	setReq.SetPathValue("societyId", f.a.SocietyID.String())
	if got := apiStatus(t, f.h.setRate(httptest.NewRecorder(), setReq)); got != http.StatusNotFound {
		t.Errorf("another builder set this rate: status %d", got)
	}

	genReq := as(httptest.NewRequest(http.MethodPost, "/x", nil), intruder)
	genReq.SetPathValue("societyId", f.a.SocietyID.String())
	if got := apiStatus(t, f.h.generate(httptest.NewRecorder(), genReq)); got != http.StatusNotFound {
		t.Errorf("another builder raised bills here: status %d", got)
	}
}

func TestSetRateHandler(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	call := func(body string) (*httptest.ResponseRecorder, error) {
		r := as(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(body)), admin)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		rec := httptest.NewRecorder()
		return rec, f.h.setRate(rec, r)
	}

	rec, err := call(`{"sector":"Sector 01","ratePerSqft":12.5}`)
	if err != nil {
		t.Fatalf("setRate: %v", err)
	}
	if got := body(t, rec)["referenceAmount"]; got != amountFor(ReferenceAreaSqft, 12.5) {
		t.Errorf("referenceAmount = %v", got)
	}

	// The whole point of the ceiling: a slipped decimal point would bill every
	// owner lakhs.
	if _, err := call(`{"sector":"Sector 01","ratePerSqft":1104}`); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("a rate over Rs 500/sq ft should be rejected")
	}
	if _, err := call(`{"sector":"Sector 01","ratePerSqft":-1}`); apiStatus(t, err) != http.StatusUnprocessableEntity {
		t.Error("a negative rate should be rejected")
	}
	if _, err := call(`{`); apiStatus(t, err) != http.StatusBadRequest {
		t.Error("a malformed body should be 400")
	}

	// Zero is legitimate — a builder may waive maintenance for a sector.
	if _, err := call(`{"sector":"Sector 04","ratePerSqft":0}`); err != nil {
		t.Errorf("a zero rate should be allowed: %v", err)
	}
}

func TestGenerateHandler(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")
	f.soldPlot(t, "2")
	testsupport.NewPlot(t, f.db, f.a.SocietyID, "3", "available")
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	call := func() (*httptest.ResponseRecorder, error) {
		r := as(httptest.NewRequest(http.MethodPost, "/x", nil), admin)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		rec := httptest.NewRecorder()
		return rec, f.h.generate(rec, r)
	}

	rec, err := call()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := body(t, rec)["raised"]; got != float64(2) {
		t.Errorf("raised = %v, want 2 — the available plot is not billable", got)
	}

	// Idempotent: the button can be pressed twice without double-billing.
	rec, err = call()
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, rec)["raised"]; got != float64(0) {
		t.Errorf("a second run raised %v bills", got)
	}
}

func TestBadSocietyID(t *testing.T) {
	f := newFixture(t)
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	for name, call := range map[string]func(*http.Request) error{
		"list":     func(r *http.Request) error { return f.h.list(httptest.NewRecorder(), r) },
		"setRate":  func(r *http.Request) error { return f.h.setRate(httptest.NewRecorder(), r) },
		"generate": func(r *http.Request) error { return f.h.generate(httptest.NewRecorder(), r) },
	} {
		r := as(httptest.NewRequest(http.MethodGet, "/x", strings.NewReader(`{}`)), admin)
		r.SetPathValue("societyId", "not-a-uuid")
		if got := apiStatus(t, call(r)); got != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, got)
		}
	}
}

func TestRoutesAreRegistered(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	f.h.Routes(mux, auth.NewAuthenticator(
		auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Minute, time.Hour)))

	for _, c := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/societies/" + f.a.SocietyID.String() + "/maintenance-rates"},
		{http.MethodPut, "/api/societies/" + f.a.SocietyID.String() + "/maintenance-rates"},
		{http.MethodPost, "/api/societies/" + f.a.SocietyID.String() + "/maintenance-bills"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, strings.NewReader(`{}`)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401 without a token", c.method, c.path, rec.Code)
		}
	}
}

// Every path must surface a 500 rather than a silent empty result.
func TestBehaviourWhenTheDatabaseIsDown(t *testing.T) {
	f := newFixture(t)
	f.soldPlot(t, "1")
	societyID := f.a.SocietyID
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)
	f.db.Close()

	ctx := context.Background()
	if _, _, err := f.store.List(ctx, societyID); err == nil {
		t.Error("List should fail")
	}
	if err := f.store.SetRate(ctx, societyID, "Sector 01", 1, f.adminA); err == nil {
		t.Error("SetRate sector should fail")
	}
	if err := f.store.SetRate(ctx, societyID, "", 1, f.adminA); err == nil {
		t.Error("SetRate default should fail")
	}
	if _, _, err := f.store.GenerateBills(ctx, societyID); err == nil {
		t.Error("GenerateBills should fail")
	}

	for name, call := range map[string]func(*http.Request) error{
		"list":     func(r *http.Request) error { return f.h.list(httptest.NewRecorder(), r) },
		"setRate":  func(r *http.Request) error { return f.h.setRate(httptest.NewRecorder(), r) },
		"generate": func(r *http.Request) error { return f.h.generate(httptest.NewRecorder(), r) },
	} {
		r := as(httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{"ratePerSqft":1}`)), admin)
		r.SetPathValue("societyId", societyID.String())
		if got := apiStatus(t, call(r)); got != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", name, got)
		}
	}
}
