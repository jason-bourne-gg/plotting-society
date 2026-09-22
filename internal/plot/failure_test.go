package plot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

// When the database is gone, every path must surface a 500 rather than an empty
// success. Closing the pool is the cheapest honest way to reach those branches:
// they are otherwise unreachable without a fault-injecting driver.
func TestBehaviourWhenTheDatabaseIsDown(t *testing.T) {
	f := newFixture(t)
	societyID, plotID, ownerID := f.a.SocietyID, f.sold, f.owner
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)
	f.db.Close()

	ctx := context.Background()

	t.Run("store", func(t *testing.T) {
		if _, err := f.store.ListForMap(ctx, societyID, ownerID); err == nil {
			t.Error("ListForMap should fail")
		}
		if _, err := f.store.Summary(ctx, societyID); err == nil {
			t.Error("Summary should fail")
		}
		if _, _, _, err := f.store.Get(ctx, plotID); err == nil {
			t.Error("Get should fail")
		}
		if _, err := f.store.DuesForPlot(ctx, plotID); err == nil {
			t.Error("DuesForPlot should fail")
		}
		if _, err := f.store.DocumentsForPlot(ctx, plotID, societyID, true); err == nil {
			t.Error("DocumentsForPlot should fail")
		}
		if _, err := f.store.Create(ctx, societyID, UpsertInput{PlotNo: "1", Status: "available"}); err == nil {
			t.Error("Create should fail")
		}
		if err := f.store.Update(ctx, plotID, UpsertInput{PlotNo: "1", Status: "available"}); err == nil {
			t.Error("Update should fail")
		}
		if _, err := f.store.Societies(ctx, domain.RoleSuperAdmin, nil, ownerID); err == nil {
			t.Error("Societies should fail")
		}
	})

	t.Run("handlers return 500", func(t *testing.T) {
		list := httptest.NewRequest(http.MethodGet, "/x", nil)
		list.SetPathValue("societyId", societyID.String())
		if got := apiStatus(t, f.h.list(httptest.NewRecorder(), list)); got != http.StatusInternalServerError {
			t.Errorf("list status = %d", got)
		}

		sum := httptest.NewRequest(http.MethodGet, "/x", nil)
		sum.SetPathValue("societyId", societyID.String())
		if got := apiStatus(t, f.h.summary(httptest.NewRecorder(), sum)); got != http.StatusInternalServerError {
			t.Errorf("summary status = %d", got)
		}

		get := as(httptest.NewRequest(http.MethodGet, "/x", nil), admin)
		get.SetPathValue("plotId", plotID.String())
		if got := apiStatus(t, f.h.get(httptest.NewRecorder(), get)); got != http.StatusInternalServerError {
			t.Errorf("get status = %d", got)
		}

		socs := as(httptest.NewRequest(http.MethodGet, "/x", nil), admin)
		if got := apiStatus(t, f.h.listSocieties(httptest.NewRecorder(), socs)); got != http.StatusInternalServerError {
			t.Errorf("listSocieties status = %d", got)
		}

		body := `{"plotNo":"1","status":"available"}`
		create := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), admin)
		create.SetPathValue("societyId", societyID.String())
		if got := apiStatus(t, f.h.create(httptest.NewRecorder(), create)); got != http.StatusInternalServerError {
			t.Errorf("create status = %d", got)
		}

		update := as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(body)), admin)
		update.SetPathValue("plotId", plotID.String())
		if got := apiStatus(t, f.h.update(httptest.NewRecorder(), update)); got != http.StatusInternalServerError {
			t.Errorf("update status = %d", got)
		}
	})

	t.Run("owner detail with dues failing", func(t *testing.T) {
		// The owner path loads dues and documents after the plot itself, which
		// is a separate failure branch from the plot lookup.
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), ident(ownerID, nil, domain.RoleOwner))
		r.SetPathValue("plotId", uuid.New().String())
		if got := apiStatus(t, f.h.get(httptest.NewRecorder(), r)); got != http.StatusInternalServerError {
			t.Errorf("status = %d", got)
		}
	})
}
