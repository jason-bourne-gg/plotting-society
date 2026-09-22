package auth

import (
	"context"
	"strings"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

func TestStoreBehaviourWhenTheDatabaseIsDown(t *testing.T) {
	f := newStoreFixture(t)
	adminID, plotID := f.admin, f.plot
	f.db.Close()

	ctx := context.Background()

	if _, _, err := f.store.UserByEmail(ctx, "admin@alpha.in"); err == nil {
		t.Error("UserByEmail should fail")
	}
	if _, err := f.store.UserByID(ctx, adminID); err == nil {
		t.Error("UserByID should fail")
	}
	if err := f.store.TouchLastLogin(ctx, adminID); err == nil {
		t.Error("TouchLastLogin should fail")
	}
	if _, err := f.store.UpdateProfile(ctx, adminID, "n", "", "", "", false); err == nil {
		t.Error("UpdateProfile should fail")
	}
	if err := f.store.SetPassword(ctx, adminID, "h"); err == nil {
		t.Error("SetPassword should fail")
	}
	if _, err := f.store.CreateInvite(ctx, Invite{
		Email: "a@b.in", Name: "A", Role: domain.RoleOwner, PlotID: &plotID,
		ExpiresAt: time.Now().Add(time.Hour),
	}, "hash", adminID); err == nil {
		t.Error("CreateInvite should fail")
	}
	if _, err := f.store.InviteByTokenHash(ctx, "hash"); err == nil {
		t.Error("InviteByTokenHash should fail")
	}
	if _, err := f.store.AcceptInvite(ctx, Invite{
		Email: "a@b.in", Name: "A", Role: domain.RoleOwner, ExpiresAt: time.Now(),
	}, "h"); err == nil {
		t.Error("AcceptInvite should fail")
	}
	if err := f.store.StoreRefreshToken(ctx, adminID, "h", time.Now().Add(time.Hour), ""); err == nil {
		t.Error("StoreRefreshToken should fail")
	}
	if _, err := f.store.ConsumeRefreshToken(ctx, "h"); err == nil {
		t.Error("ConsumeRefreshToken should fail")
	}
	if err := f.store.RevokeAllRefreshTokens(ctx, adminID); err == nil {
		t.Error("RevokeAllRefreshTokens should fail")
	}
}

// Sign-in must fail closed: a database outage must never hand out a session.
func TestHandlersFailClosedWhenTheDatabaseIsDown(t *testing.T) {
	f := newHandlerFixture(t)
	ownerID := f.owner
	adminID, builderID, plotID := f.admin, f.builder.ID, f.plot
	f.db.Close()

	if got := apiStatus(t, f.h.login(httptest.NewRecorder(),
		post(`{"email":"admin@alpha.in","password":"`+testPassword+`"}`))); got != http.StatusInternalServerError {
		t.Errorf("login status = %d, want 500 — never a session", got)
	}
	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(),
		post(`{"refreshToken":"anything"}`))); got != http.StatusInternalServerError {
		t.Errorf("refresh status = %d", got)
	}

	ownerIdent := Identity{UserID: ownerID, Role: domain.RoleOwner}
	if got := apiStatus(t, f.h.me(httptest.NewRecorder(),
		as(httptest.NewRequest(http.MethodGet, "/x", nil), ownerIdent))); got != http.StatusInternalServerError {
		t.Errorf("me status = %d", got)
	}
	updateMe := as(httptest.NewRequest(http.MethodPatch, "/x",
		strings.NewReader(`{"name":"New Name"}`)), ownerIdent)
	if got := apiStatus(t, f.h.updateMe(httptest.NewRecorder(), updateMe)); got != http.StatusInternalServerError {
		t.Errorf("updateMe status = %d", got)
	}

	changePw := as(post(`{"currentPassword":"`+testPassword+`","newPassword":"another-good-password"}`), ownerIdent)
	if got := apiStatus(t, f.h.changePassword(httptest.NewRecorder(), changePw)); got != http.StatusInternalServerError {
		t.Errorf("changePassword status = %d", got)
	}

	if got := apiStatus(t, f.h.logout(httptest.NewRecorder(),
		as(httptest.NewRequest(http.MethodPost, "/x", nil), ownerIdent))); got != http.StatusInternalServerError {
		t.Errorf("logout status = %d", got)
	}

	admin := staffIdentity(adminID, builderID, domain.RoleBuilderAdmin)
	invite := as(post(`{"email":"a@b.in","name":"A","role":"owner","plotId":"`+plotID.String()+`"}`), admin)
	if got := apiStatus(t, f.h.createInvite(httptest.NewRecorder(), invite)); got != http.StatusInternalServerError {
		t.Errorf("createInvite status = %d", got)
	}

	inspect := httptest.NewRequest(http.MethodGet, "/x", nil)
	inspect.SetPathValue("token", "anything")
	if got := apiStatus(t, f.h.inspectInvite(httptest.NewRecorder(), inspect)); got != http.StatusInternalServerError {
		t.Errorf("inspectInvite status = %d", got)
	}

	accept := post(`{"password":"a-decent-password"}`)
	accept.SetPathValue("token", "anything")
	if got := apiStatus(t, f.h.acceptInvite(httptest.NewRecorder(), accept)); got != http.StatusInternalServerError {
		t.Errorf("acceptInvite status = %d", got)
	}
}

func TestIssueSessionFailsWhenTheTokenCannotBeStored(t *testing.T) {
	f := newHandlerFixture(t)
	user := domain.User{ID: uuid.New(), Name: "X", Role: domain.RoleOwner}
	f.db.Close()

	if err := f.h.issueSession(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/x", nil), user, http.StatusOK); err == nil {
		t.Fatal("issueSession should fail when the refresh token cannot be stored")
	}
}
