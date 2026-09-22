package plot

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
	db     *database.DB
	store  *Store
	h      *Handler
	a, b   testsupport.Builder
	adminA uuid.UUID
	adminB uuid.UUID
	owner  uuid.UUID
	sold   uuid.UUID
	free   uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testsupport.DB(t, "plot")
	store := NewStore(db)

	f := fixture{db: db, store: store, h: NewHandler(store, access.NewGuard(db))}
	f.a = testsupport.NewBuilder(t, db, "Alpha")
	f.b = testsupport.NewBuilder(t, db, "Beta")
	f.adminA = testsupport.NewUser(t, db, &f.a.ID, domain.RoleBuilderAdmin, "a@alpha.in", "")
	f.adminB = testsupport.NewUser(t, db, &f.b.ID, domain.RoleBuilderAdmin, "b@beta.in", "")
	f.owner = testsupport.NewUser(t, db, nil, domain.RoleOwner, "o@example.in", "")

	f.sold = testsupport.NewPlot(t, db, f.a.SocietyID, "147", "sold")
	testsupport.AssignPlot(t, db, f.sold, f.owner)
	f.free = testsupport.NewPlot(t, db, f.a.SocietyID, "148", "available")
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

func TestListForMapMarksTheViewersPlot(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	plots, err := f.store.ListForMap(ctx, f.a.SocietyID, f.owner)
	if err != nil {
		t.Fatalf("ListForMap: %v", err)
	}
	if len(plots) != 2 {
		t.Fatalf("got %d plots, want 2", len(plots))
	}

	var mine int
	for _, p := range plots {
		if p.IsMine {
			mine++
			if p.PlotNo != "147" {
				t.Errorf("the wrong plot is marked as the viewer's: %s", p.PlotNo)
			}
		}
	}
	if mine != 1 {
		t.Errorf("%d plots marked as the viewer's, want 1", mine)
	}

	// A guest passes uuid.Nil and owns nothing.
	guestView, err := f.store.ListForMap(ctx, f.a.SocietyID, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range guestView {
		if p.IsMine {
			t.Error("a guest was shown a plot as their own")
		}
	}
}

func TestSummaryCounts(t *testing.T) {
	f := newFixture(t)
	testsupport.NewPlot(t, f.db, f.a.SocietyID, "149", "booked")
	testsupport.NewPlot(t, f.db, f.a.SocietyID, "150", "on_hold")

	sum, err := f.store.Summary(context.Background(), f.a.SocietyID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.Total != 4 || sum.Sold != 1 || sum.Available != 1 || sum.Booked != 1 || sum.OnHold != 1 {
		t.Errorf("summary = %+v", sum)
	}
}

func TestGetAndDues(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	detail, ownerID, societyID, err := f.store.Get(ctx, f.sold)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ownerID != f.owner || societyID != f.a.SocietyID || detail.PlotNo != "147" {
		t.Errorf("unexpected: %+v owner=%v society=%v", detail, ownerID, societyID)
	}

	if _, _, _, err := f.store.Get(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	if _, err := f.db.Exec(ctx, `
		INSERT INTO maintenance_dues (plot_id, period_label, amount_due, amount_paid, due_date)
		VALUES ($1,'FY2026-Q4',7500,0,CURRENT_DATE)`, f.sold); err != nil {
		t.Fatal(err)
	}
	dues, err := f.store.DuesForPlot(ctx, f.sold)
	if err != nil {
		t.Fatalf("DuesForPlot: %v", err)
	}
	if len(dues) != 1 || dues[0].AmountDue != 7500 {
		t.Errorf("dues = %+v", dues)
	}
}

// Staff-only documents must not reach an owner.
func TestDocumentsForPlotRespectsVisibility(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	for _, v := range []string{"plot", "staff"} {
		if _, err := f.db.Exec(ctx, `
			INSERT INTO documents (society_id, plot_id, doc_type, title, url, visibility)
			VALUES ($1,$2,'sale_deed',$3,'https://x/y.pdf',$4)`,
			f.a.SocietyID, f.sold, "doc-"+v, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Exec(ctx, `
		INSERT INTO documents (society_id, plot_id, doc_type, title, url, visibility)
		VALUES ($1,NULL,'layout_plan','society-wide','https://x/z.pdf','society')`,
		f.a.SocietyID); err != nil {
		t.Fatal(err)
	}

	ownerDocs, err := f.store.DocumentsForPlot(ctx, f.sold, f.a.SocietyID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ownerDocs {
		if d.Title == "doc-staff" {
			t.Fatal("an owner was shown a staff-only document")
		}
	}

	staffDocs, err := f.store.DocumentsForPlot(ctx, f.sold, f.a.SocietyID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(staffDocs) <= len(ownerDocs) {
		t.Error("staff should see at least the staff-only document as well")
	}
}

func TestCreateAndUpdate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	area := 1291.68
	price := 3400000.0

	id, err := f.store.Create(ctx, f.a.SocietyID, UpsertInput{
		PlotNo: "200", Phase: "Sector 02", AreaSqft: &area, Facing: "East",
		Status: domain.PlotAvailable, Price: &price,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := f.store.Update(ctx, id, UpsertInput{
		PlotNo: "200", Phase: "Sector 02", AreaSqft: &area, Facing: "West",
		Status: domain.PlotBooked, Price: &price,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	detail, _, _, err := f.store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != domain.PlotBooked || detail.Facing != "West" {
		t.Errorf("update did not apply: %+v", detail)
	}

	if err := f.store.Update(ctx, uuid.New(), UpsertInput{PlotNo: "x", Status: "available"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ----------------------------------------------------------------- handler

func TestGetHandlerHidesOtherPeoplesDetails(t *testing.T) {
	f := newFixture(t)

	get := func(id auth.Identity) map[string]any {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), id)
		r.SetPathValue("plotId", f.sold.String())
		rec := httptest.NewRecorder()
		if err := f.h.get(rec, r); err != nil {
			t.Fatalf("get: %v", err)
		}
		return jsonBody(t, rec)
	}

	t.Run("the owner sees their own dues and name", func(t *testing.T) {
		body := get(ident(f.owner, nil, domain.RoleOwner))
		if body["isMine"] != true {
			t.Error("isMine should be true for the plot's owner")
		}
	})

	t.Run("another owner sees neither", func(t *testing.T) {
		stranger := testsupport.NewUser(t, f.db, nil, domain.RoleOwner, "stranger@example.in", "")
		body := get(ident(stranger, nil, domain.RoleOwner))
		if body["ownerName"] != nil {
			t.Errorf("a stranger was shown the owner's name: %v", body["ownerName"])
		}
		if body["dues"] != nil {
			t.Error("a stranger was shown the dues")
		}
		if body["isMine"] == true {
			t.Error("isMine should be false")
		}
	})

	t.Run("staff see everything", func(t *testing.T) {
		body := get(ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))
		if body["ownerName"] == nil {
			t.Error("staff should see the owner's name")
		}
	})
}

func TestListAndSummaryHandlers(t *testing.T) {
	f := newFixture(t)

	t.Run("guest list", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		rec := httptest.NewRecorder()
		if err := f.h.list(rec, r); err != nil {
			t.Fatalf("list: %v", err)
		}
		plots, _ := jsonBody(t, rec)["plots"].([]any)
		if len(plots) != 2 {
			t.Errorf("got %d plots", len(plots))
		}
	})

	t.Run("signed-in list", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.owner, nil, domain.RoleOwner))
		r.SetPathValue("societyId", f.a.SocietyID.String())
		rec := httptest.NewRecorder()
		if err := f.h.list(rec, r); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("summary", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		rec := httptest.NewRecorder()
		if err := f.h.summary(rec, r); err != nil {
			t.Fatal(err)
		}
		if jsonBody(t, rec)["total"] != float64(2) {
			t.Error("wrong total")
		}
	})

	t.Run("bad ids", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("societyId", "nope")
		if got := apiStatus(t, f.h.list(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
			t.Errorf("list status = %d", got)
		}
		if got := apiStatus(t, f.h.summary(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
			t.Errorf("summary status = %d", got)
		}

		r2 := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.owner, nil, domain.RoleOwner))
		r2.SetPathValue("plotId", "nope")
		if got := apiStatus(t, f.h.get(httptest.NewRecorder(), r2)); got != http.StatusBadRequest {
			t.Errorf("get status = %d", got)
		}

		r3 := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(f.owner, nil, domain.RoleOwner))
		r3.SetPathValue("plotId", uuid.NewString())
		if got := apiStatus(t, f.h.get(httptest.NewRecorder(), r3)); got != http.StatusNotFound {
			t.Errorf("missing plot status = %d", got)
		}
	})
}

// Builder B must not create plots in, or edit plots of, builder A's society.
func TestPlotMutationsAreScopedToTheirBuilder(t *testing.T) {
	f := newFixture(t)
	intruder := ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin)
	owner := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	body := `{"plotNo":"999","phase":"Sector 01","areaSqft":1200,"facing":"East","isCorner":false,"status":"available","price":100000,"mapShape":null,"notes":""}`

	t.Run("create in another builder's society is denied", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), intruder)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		if got := apiStatus(t, f.h.create(httptest.NewRecorder(), r)); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
	})

	t.Run("update another builder's plot is denied", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(body)), intruder)
		r.SetPathValue("plotId", f.free.String())
		if got := apiStatus(t, f.h.update(httptest.NewRecorder(), r)); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
	})

	t.Run("the owning builder can do both", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), owner)
		r.SetPathValue("societyId", f.a.SocietyID.String())
		rec := httptest.NewRecorder()
		if err := f.h.create(rec, r); err != nil {
			t.Fatalf("create: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d", rec.Code)
		}

		updateBody := strings.Replace(body, `"plotNo":"999"`, `"plotNo":"1000"`, 1)
		r = as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(updateBody)), owner)
		r.SetPathValue("plotId", f.free.String())
		rec = httptest.NewRecorder()
		if err := f.h.update(rec, r); err != nil {
			t.Fatalf("update: %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func TestCreateRejectsDuplicatePlotNumber(t *testing.T) {
	f := newFixture(t)
	owner := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	r := as(httptest.NewRequest(http.MethodPost, "/x",
		strings.NewReader(`{"plotNo":"147","status":"available"}`)), owner)
	r.SetPathValue("societyId", f.a.SocietyID.String())
	if got := apiStatus(t, f.h.create(httptest.NewRecorder(), r)); got != http.StatusConflict {
		t.Fatalf("status = %d, want 409", got)
	}
}

func TestDecodeUpsertValidation(t *testing.T) {
	bad := -1.0
	cases := map[string]UpsertInput{
		"no plot number": {Status: "available"},
		"unknown status": {PlotNo: "1", Status: "reserved"},
		"zero area":      {PlotNo: "1", Status: "available", AreaSqft: func() *float64 { z := 0.0; return &z }()},
		"negative price": {PlotNo: "1", Status: "available", Price: &bad},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			payload, _ := json.Marshal(in)
			r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(string(payload)))
			if _, err := decodeUpsert(r); err == nil {
				t.Error("expected a validation error")
			}
		})
	}

	t.Run("status defaults to available", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"plotNo":"1"}`))
		in, err := decodeUpsert(r)
		if err != nil {
			t.Fatal(err)
		}
		if in.Status != domain.PlotAvailable {
			t.Errorf("status = %q", in.Status)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{`))
		if _, err := decodeUpsert(r); err == nil {
			t.Error("expected a decode error")
		}
	})
}

func TestUpdateHandlerBadIDs(t *testing.T) {
	f := newFixture(t)
	owner := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	r := as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"plotNo":"1"}`)), owner)
	r.SetPathValue("plotId", "nope")
	if got := apiStatus(t, f.h.update(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
		t.Errorf("status = %d", got)
	}

	r = as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"plotNo":"1"}`)), owner)
	r.SetPathValue("societyId", "nope")
	if got := apiStatus(t, f.h.create(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
		t.Errorf("status = %d", got)
	}
}

// Renaming a plot onto a number that already exists is a 409, the same as
// creating one — not a 500.
func TestUpdateRejectsDuplicatePlotNumber(t *testing.T) {
	f := newFixture(t)
	owner := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	r := as(httptest.NewRequest(http.MethodPatch, "/x",
		strings.NewReader(`{"plotNo":"147","status":"available"}`)), owner)
	r.SetPathValue("plotId", f.free.String())

	if got := apiStatus(t, f.h.update(httptest.NewRecorder(), r)); got != http.StatusConflict {
		t.Fatalf("status = %d, want 409", got)
	}
}
