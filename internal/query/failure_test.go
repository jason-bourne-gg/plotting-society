package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

// See plot/failure_test.go: these branches are unreachable without either a
// fault-injecting driver or a closed pool, and a silent success here would mean
// an owner's query vanishing without anyone noticing.
func TestBehaviourWhenTheDatabaseIsDown(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id, err := f.store.Create(ctx, f.a.SocietyID, f.owner, &f.plot, "other", "s", "b")
	if err != nil {
		t.Fatal(err)
	}
	societyID := f.a.SocietyID
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)
	owner := ident(f.owner, nil, domain.RoleOwner)
	f.db.Close()

	if _, err := f.store.Create(ctx, societyID, f.owner, &f.plot, "other", "s", "b"); err == nil {
		t.Error("Create should fail")
	}
	if _, err := f.store.ListForOwner(ctx, f.owner, 10); err == nil {
		t.Error("ListForOwner should fail")
	}
	if _, err := f.store.ListForSociety(ctx, societyID, "", 10); err == nil {
		t.Error("ListForSociety should fail")
	}
	if _, err := f.store.Get(ctx, id); err == nil {
		t.Error("Get should fail")
	}
	if _, err := f.store.Messages(ctx, id, true); err == nil {
		t.Error("Messages should fail")
	}
	if _, err := f.store.AddMessage(ctx, id, f.owner, "x", "", false); err == nil {
		t.Error("AddMessage should fail")
	}
	if err := f.store.SetStatus(ctx, id, "open", nil); err == nil {
		t.Error("SetStatus should fail")
	}

	create := as(httptest.NewRequest(http.MethodPost, "/x",
		strings.NewReader(`{"category":"other","subject":"s","body":"b"}`)), owner)
	create.SetPathValue("societyId", societyID.String())
	if got := apiStatus(t, f.h.create(httptest.NewRecorder(), create)); got != http.StatusInternalServerError {
		t.Errorf("create status = %d", got)
	}

	mine := as(httptest.NewRequest(http.MethodGet, "/x", nil), owner)
	if got := apiStatus(t, f.h.listMine(httptest.NewRecorder(), mine)); got != http.StatusInternalServerError {
		t.Errorf("listMine status = %d", got)
	}

	inbox := as(httptest.NewRequest(http.MethodGet, "/x", nil), admin)
	inbox.SetPathValue("societyId", societyID.String())
	if got := apiStatus(t, f.h.listForSociety(httptest.NewRecorder(), inbox)); got != http.StatusInternalServerError {
		t.Errorf("listForSociety status = %d", got)
	}

	get := as(httptest.NewRequest(http.MethodGet, "/x", nil), owner)
	get.SetPathValue("queryId", id.String())
	if got := apiStatus(t, f.h.get(httptest.NewRecorder(), get)); got != http.StatusInternalServerError {
		t.Errorf("get status = %d", got)
	}

	msg := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"body":"x"}`)), owner)
	msg.SetPathValue("queryId", id.String())
	if got := apiStatus(t, f.h.addMessage(httptest.NewRecorder(), msg)); got != http.StatusInternalServerError {
		t.Errorf("addMessage status = %d", got)
	}

	patch := as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"status":"open"}`)), admin)
	patch.SetPathValue("queryId", id.String())
	if got := apiStatus(t, f.h.setStatus(httptest.NewRecorder(), patch)); got != http.StatusInternalServerError {
		t.Errorf("setStatus status = %d", got)
	}
}
