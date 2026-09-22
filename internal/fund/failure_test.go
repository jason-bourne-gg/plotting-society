package fund

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

// A ledger that silently returns an empty balance when the database is down
// would be worse than one that errors, so every path must surface a 500.
func TestBehaviourWhenTheDatabaseIsDown(t *testing.T) {
	f := newFixture(t)
	entryID := testsupport.NewFundEntry(t, f.db, f.a.SocietyID, f.adminA, "roads", 0, 5000)
	societyID := f.a.SocietyID
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)
	owner := ident(f.owner, nil, domain.RoleOwner)
	f.db.Close()

	ctx := context.Background()

	if _, err := f.store.List(ctx, societyID, 10); err == nil {
		t.Error("List should fail")
	}
	if _, err := f.store.Balance(ctx, societyID); err == nil {
		t.Error("Balance should fail")
	}
	if _, err := f.store.Create(ctx, societyID, f.adminA, NewEntry{
		EntryDate: today(), Head: "h", Description: "d", Debit: 1,
	}); err == nil {
		t.Error("Create should fail")
	}
	if _, err := f.store.Reverse(ctx, entryID, f.adminA, "x"); err == nil {
		t.Error("Reverse should fail")
	}
	if _, err := f.store.OwnerInSociety(ctx, f.owner, societyID); err == nil {
		t.Error("OwnerInSociety should fail")
	}

	// An owner's read fails at the membership check, staff at the guard.
	ownerList := as(httptest.NewRequest(http.MethodGet, "/x", nil), owner)
	ownerList.SetPathValue("societyId", societyID.String())
	if got := apiStatus(t, f.h.list(httptest.NewRecorder(), ownerList)); got != http.StatusInternalServerError {
		t.Errorf("owner list status = %d", got)
	}

	staffList := as(httptest.NewRequest(http.MethodGet, "/x", nil), admin)
	staffList.SetPathValue("societyId", societyID.String())
	if got := apiStatus(t, f.h.list(httptest.NewRecorder(), staffList)); got != http.StatusInternalServerError {
		t.Errorf("staff list status = %d", got)
	}

	create := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(
		`{"entryDate":"`+today()+`","head":"h","description":"d","debit":1}`)), admin)
	create.SetPathValue("societyId", societyID.String())
	if got := apiStatus(t, f.h.create(httptest.NewRecorder(), create)); got != http.StatusInternalServerError {
		t.Errorf("create status = %d", got)
	}

	reverse := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"reason":"x"}`)), admin)
	reverse.SetPathValue("entryId", entryID.String())
	if got := apiStatus(t, f.h.reverse(httptest.NewRecorder(), reverse)); got != http.StatusInternalServerError {
		t.Errorf("reverse status = %d", got)
	}
	_ = domain.RoleOwner
}
