package auth

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
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

type handlerFixture struct {
	storeFixture
	h      *Handler
	issuer *TokenIssuer
	other  testsupport.Builder
	otherA uuid.UUID
	owner  uuid.UUID
}

func newHandlerFixture(t *testing.T) handlerFixture {
	t.Helper()
	sf := newStoreFixture(t)
	issuer := NewTokenIssuer(secret, 15*time.Minute, 720*time.Hour)

	f := handlerFixture{
		storeFixture: sf,
		issuer:       issuer,
		h:            NewHandler(sf.store, issuer, access.NewGuard(sf.db), "https://app.example.in/invite"),
	}
	f.other = testsupport.NewBuilder(t, sf.db, "Beta")
	f.otherA = testsupport.NewUser(t, sf.db, &f.other.ID, domain.RoleBuilderAdmin, "admin@beta.in", "")

	hash, _ := HashPassword(testPassword)
	f.owner = testsupport.NewUser(t, sf.db, nil, domain.RoleOwner, "owner@example.in", hash)
	return f
}

func post(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
}

func as(r *http.Request, id Identity) *http.Request {
	return r.WithContext(ContextWithIdentity(r.Context(), id))
}

func staffIdentity(userID, builderID uuid.UUID, role domain.Role) Identity {
	return Identity{UserID: userID, BuilderID: &builderID, Role: role}
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

// ------------------------------------------------------------------- login

func TestLogin(t *testing.T) {
	f := newHandlerFixture(t)

	rec := httptest.NewRecorder()
	err := f.h.login(rec, post(`{"email":"admin@alpha.in","password":"`+testPassword+`"}`))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	body := jsonBody(t, rec)
	access, _ := body["accessToken"].(string)
	refresh, _ := body["refreshToken"].(string)
	if access == "" || refresh == "" {
		t.Fatal("login did not return both tokens")
	}
	claims, err := f.issuer.ParseAccess(access)
	if err != nil {
		t.Fatalf("the issued access token does not parse: %v", err)
	}
	if claims.Subject != f.admin.String() {
		t.Errorf("subject = %q", claims.Subject)
	}
	// The password hash must never travel back to the client.
	if strings.Contains(rec.Body.String(), "argon2id") {
		t.Fatal("the response leaked a password hash")
	}
}

func TestLoginWrongPasswordAndUnknownEmail(t *testing.T) {
	f := newHandlerFixture(t)

	// Both must give the same answer, or the response tells an attacker which
	// addresses have accounts.
	wrongPw := apiStatus(t, f.h.login(httptest.NewRecorder(),
		post(`{"email":"admin@alpha.in","password":"not-the-password"}`)))
	unknown := apiStatus(t, f.h.login(httptest.NewRecorder(),
		post(`{"email":"nobody@alpha.in","password":"`+testPassword+`"}`)))

	if wrongPw != http.StatusUnauthorized || unknown != http.StatusUnauthorized {
		t.Fatalf("statuses = %d and %d, want 401 for both", wrongPw, unknown)
	}
}

// A user created by an invite that was never accepted has no password hash.
func TestLoginUserWithoutPassword(t *testing.T) {
	f := newHandlerFixture(t)
	if got := apiStatus(t, f.h.login(httptest.NewRecorder(),
		post(`{"email":"admin@beta.in","password":"anything-at-all"}`))); got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got)
	}
}

func TestLoginDeactivatedAccount(t *testing.T) {
	f := newHandlerFixture(t)
	if _, err := f.db.Exec(context.Background(),
		`UPDATE users SET is_active = false WHERE id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}
	if got := apiStatus(t, f.h.login(httptest.NewRecorder(),
		post(`{"email":"admin@alpha.in","password":"`+testPassword+`"}`))); got != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", got)
	}
}

func TestLoginValidation(t *testing.T) {
	f := newHandlerFixture(t)
	for name, payload := range map[string]string{
		"no email":    `{"email":"","password":"x"}`,
		"no password": `{"email":"a@b.in","password":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := apiStatus(t, f.h.login(httptest.NewRecorder(), post(payload))); got != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", got)
			}
		})
	}
	t.Run("malformed", func(t *testing.T) {
		if got := apiStatus(t, f.h.login(httptest.NewRecorder(), post(`{`))); got != http.StatusBadRequest {
			t.Error("expected 400")
		}
	})
}

// ----------------------------------------------------------------- refresh

func TestRefreshRotatesTheToken(t *testing.T) {
	f := newHandlerFixture(t)

	rec := httptest.NewRecorder()
	if err := f.h.login(rec, post(`{"email":"admin@alpha.in","password":"`+testPassword+`"}`)); err != nil {
		t.Fatal(err)
	}
	first, _ := jsonBody(t, rec)["refreshToken"].(string)

	rec = httptest.NewRecorder()
	if err := f.h.refresh(rec, post(`{"refreshToken":"`+first+`"}`)); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	second, _ := jsonBody(t, rec)["refreshToken"].(string)
	if second == "" || second == first {
		t.Fatal("refresh should hand back a new token")
	}

	// The consumed token must not work again.
	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(),
		post(`{"refreshToken":"`+first+`"}`))); got != http.StatusUnauthorized {
		t.Fatalf("a replayed refresh token status = %d, want 401", got)
	}
}

func TestRefreshRejectsUnknownAndMissing(t *testing.T) {
	f := newHandlerFixture(t)

	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(), post(`{"refreshToken":""}`))); got != http.StatusBadRequest {
		t.Errorf("empty token status = %d, want 400", got)
	}
	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(), post(`{"refreshToken":"nope"}`))); got != http.StatusUnauthorized {
		t.Errorf("unknown token status = %d, want 401", got)
	}
	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(), post(`{`))); got != http.StatusBadRequest {
		t.Error("malformed body should be 400")
	}
}

func TestRefreshRejectsDeactivatedAccount(t *testing.T) {
	f := newHandlerFixture(t)

	rec := httptest.NewRecorder()
	if err := f.h.login(rec, post(`{"email":"admin@alpha.in","password":"`+testPassword+`"}`)); err != nil {
		t.Fatal(err)
	}
	token, _ := jsonBody(t, rec)["refreshToken"].(string)

	if _, err := f.db.Exec(context.Background(),
		`UPDATE users SET is_active = false WHERE id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}
	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(),
		post(`{"refreshToken":"`+token+`"}`))); got != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", got)
	}
}

// ----------------------------------------------------------------- profile

func TestMeAndUpdateMe(t *testing.T) {
	f := newHandlerFixture(t)
	id := Identity{UserID: f.owner, Role: domain.RoleOwner}

	rec := httptest.NewRecorder()
	if err := f.h.me(rec, as(httptest.NewRequest(http.MethodGet, "/x", nil), id)); err != nil {
		t.Fatalf("me: %v", err)
	}
	if jsonBody(t, rec)["email"] != "owner@example.in" {
		t.Error("me returned the wrong user")
	}

	rec = httptest.NewRecorder()
	r := as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(
		`{"name":"Rohit Deshmukh","phone":"+91 99876 54321","currentAddress":"Flat 1204","city":"Pune","directoryOptIn":true}`)), id)
	if err := f.h.updateMe(rec, r); err != nil {
		t.Fatalf("updateMe: %v", err)
	}
	body := jsonBody(t, rec)
	if body["name"] != "Rohit Deshmukh" || body["city"] != "Pune" || body["directoryOptIn"] != true {
		t.Errorf("profile not updated: %v", body)
	}
}

func TestMeForDeletedUser(t *testing.T) {
	f := newHandlerFixture(t)
	id := Identity{UserID: uuid.New(), Role: domain.RoleOwner}
	if got := apiStatus(t, f.h.me(httptest.NewRecorder(),
		as(httptest.NewRequest(http.MethodGet, "/x", nil), id))); got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got)
	}
}

func TestUpdateMeValidation(t *testing.T) {
	f := newHandlerFixture(t)
	id := Identity{UserID: f.owner, Role: domain.RoleOwner}

	r := as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"name":"   "}`)), id)
	if got := apiStatus(t, f.h.updateMe(httptest.NewRecorder(), r)); got != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", got)
	}

	r = as(httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{`)), id)
	if got := apiStatus(t, f.h.updateMe(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

func TestChangePassword(t *testing.T) {
	f := newHandlerFixture(t)
	id := Identity{UserID: f.owner, Role: domain.RoleOwner}

	// Give the owner a live session first, so the revoke can be observed.
	rec := httptest.NewRecorder()
	if err := f.h.login(rec, post(`{"email":"owner@example.in","password":"`+testPassword+`"}`)); err != nil {
		t.Fatal(err)
	}
	oldRefresh, _ := jsonBody(t, rec)["refreshToken"].(string)

	rec = httptest.NewRecorder()
	r := as(post(`{"currentPassword":"`+testPassword+`","newPassword":"brand-new-password"}`), id)
	if err := f.h.changePassword(rec, r); err != nil {
		t.Fatalf("changePassword: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}

	// Every other session must be gone.
	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(),
		post(`{"refreshToken":"`+oldRefresh+`"}`))); got != http.StatusUnauthorized {
		t.Error("an existing session survived a password change")
	}

	// And the new password must work.
	if err := f.h.login(httptest.NewRecorder(),
		post(`{"email":"owner@example.in","password":"brand-new-password"}`)); err != nil {
		t.Errorf("the new password does not sign in: %v", err)
	}
}

func TestChangePasswordValidation(t *testing.T) {
	f := newHandlerFixture(t)
	id := Identity{UserID: f.owner, Role: domain.RoleOwner}

	cases := map[string]struct {
		body string
		want int
	}{
		"too short":       {`{"currentPassword":"` + testPassword + `","newPassword":"short"}`, http.StatusUnprocessableEntity},
		"too long":        {`{"currentPassword":"` + testPassword + `","newPassword":"` + strings.Repeat("x", 201) + `"}`, http.StatusUnprocessableEntity},
		"wrong current":   {`{"currentPassword":"wrong-one-here","newPassword":"a-good-new-password"}`, http.StatusUnauthorized},
		"malformed":       {`{`, http.StatusBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := apiStatus(t, f.h.changePassword(httptest.NewRecorder(), as(post(c.body), id))); got != c.want {
				t.Errorf("status = %d, want %d", got, c.want)
			}
		})
	}
}

func TestLogoutRevokesEverySession(t *testing.T) {
	f := newHandlerFixture(t)

	rec := httptest.NewRecorder()
	if err := f.h.login(rec, post(`{"email":"admin@alpha.in","password":"`+testPassword+`"}`)); err != nil {
		t.Fatal(err)
	}
	refresh, _ := jsonBody(t, rec)["refreshToken"].(string)

	rec = httptest.NewRecorder()
	r := as(httptest.NewRequest(http.MethodPost, "/x", nil),
		staffIdentity(f.admin, f.builder.ID, domain.RoleBuilderAdmin))
	if err := f.h.logout(rec, r); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := apiStatus(t, f.h.refresh(httptest.NewRecorder(),
		post(`{"refreshToken":"`+refresh+`"}`))); got != http.StatusUnauthorized {
		t.Error("the session survived logout")
	}
}

// ------------------------------------------------------------------ invites

func TestCreateOwnerInvite(t *testing.T) {
	f := newHandlerFixture(t)
	caller := staffIdentity(f.admin, f.builder.ID, domain.RoleBuilderAdmin)

	rec := httptest.NewRecorder()
	r := as(post(`{"email":"new@example.in","name":"New Owner","role":"owner","plotId":"`+f.plot.String()+`"}`), caller)
	if err := f.h.createInvite(rec, r); err != nil {
		t.Fatalf("createInvite: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}

	url, _ := jsonBody(t, rec)["inviteUrl"].(string)
	if !strings.HasPrefix(url, "https://app.example.in/invite/") {
		t.Errorf("inviteUrl = %q", url)
	}

	// The link must work end to end.
	token := strings.TrimPrefix(url, "https://app.example.in/invite/")
	rec = httptest.NewRecorder()
	inspect := httptest.NewRequest(http.MethodGet, "/x", nil)
	inspect.SetPathValue("token", token)
	if err := f.h.inspectInvite(rec, inspect); err != nil {
		t.Fatalf("inspectInvite: %v", err)
	}
	if jsonBody(t, rec)["email"] != "new@example.in" {
		t.Error("inspectInvite returned the wrong invite")
	}

	rec = httptest.NewRecorder()
	accept := post(`{"password":"a-decent-password"}`)
	accept.SetPathValue("token", token)
	if err := f.h.acceptInvite(rec, accept); err != nil {
		t.Fatalf("acceptInvite: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("accept status = %d", rec.Code)
	}
}

// The IDOR this guard exists to stop: staff at Beta inviting an "owner" onto
// one of Alpha's plots, which on acceptance would mark it sold and hand it over.
func TestCreateInviteCannotTargetAnotherBuildersPlot(t *testing.T) {
	f := newHandlerFixture(t)
	intruder := staffIdentity(f.otherA, f.other.ID, domain.RoleBuilderAdmin)

	r := as(post(`{"email":"attacker@example.in","name":"Attacker","role":"owner","plotId":"`+f.plot.String()+`"}`), intruder)
	if got := apiStatus(t, f.h.createInvite(httptest.NewRecorder(), r)); got != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 — another builder invited an owner onto this plot", got)
	}

	var ownerID *uuid.UUID
	if err := f.db.QueryRow(context.Background(),
		`SELECT owner_id FROM plots WHERE id = $1`, f.plot).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if ownerID != nil {
		t.Fatal("the plot was reassigned")
	}
}

func TestCreateInviteValidation(t *testing.T) {
	f := newHandlerFixture(t)
	admin := staffIdentity(f.admin, f.builder.ID, domain.RoleBuilderAdmin)
	staff := staffIdentity(f.admin, f.builder.ID, domain.RoleBuilderStaff)

	cases := []struct {
		name   string
		id     Identity
		body   string
		status int
	}{
		{"bad email", admin, `{"email":"not-an-email","name":"X","role":"owner","plotId":"` + f.plot.String() + `"}`, http.StatusUnprocessableEntity},
		{"no name", admin, `{"email":"a@b.in","name":"  ","role":"owner","plotId":"` + f.plot.String() + `"}`, http.StatusUnprocessableEntity},
		{"unknown role", admin, `{"email":"a@b.in","name":"X","role":"wizard"}`, http.StatusUnprocessableEntity},
		{"super admin refused", admin, `{"email":"a@b.in","name":"X","role":"super_admin"}`, http.StatusUnprocessableEntity},
		{"staff cannot invite staff", staff, `{"email":"a@b.in","name":"X","role":"builder_staff"}`, http.StatusUnprocessableEntity},
		{"owner invite needs a plot", admin, `{"email":"a@b.in","name":"X","role":"owner"}`, http.StatusUnprocessableEntity},
		{"bad plot id", admin, `{"email":"a@b.in","name":"X","role":"owner","plotId":"nope"}`, http.StatusUnprocessableEntity},
		{"malformed", admin, `{`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := apiStatus(t, f.h.createInvite(httptest.NewRecorder(), as(post(c.body), c.id))); got != c.status {
				t.Errorf("status = %d, want %d", got, c.status)
			}
		})
	}
}

func TestInviteStaffAccount(t *testing.T) {
	f := newHandlerFixture(t)
	admin := staffIdentity(f.admin, f.builder.ID, domain.RoleBuilderAdmin)

	rec := httptest.NewRecorder()
	if err := f.h.createInvite(rec, as(post(`{"email":"desk@alpha.in","name":"Desk","role":"builder_staff"}`), admin)); err != nil {
		t.Fatalf("createInvite: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestInviteTokenHandlersRejectBadTokens(t *testing.T) {
	f := newHandlerFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("token", "nonsense")
	if got := apiStatus(t, f.h.inspectInvite(httptest.NewRecorder(), r)); got != http.StatusNotFound {
		t.Errorf("inspect status = %d, want 404", got)
	}

	accept := post(`{"password":"a-decent-password"}`)
	accept.SetPathValue("token", "nonsense")
	if got := apiStatus(t, f.h.acceptInvite(httptest.NewRecorder(), accept)); got != http.StatusNotFound {
		t.Errorf("accept status = %d, want 404", got)
	}

	short := post(`{"password":"short"}`)
	short.SetPathValue("token", "nonsense")
	if got := apiStatus(t, f.h.acceptInvite(httptest.NewRecorder(), short)); got != http.StatusUnprocessableEntity {
		t.Errorf("a short password should be rejected before the token lookup, got %d", got)
	}

	bad := post(`{`)
	bad.SetPathValue("token", "nonsense")
	if got := apiStatus(t, f.h.acceptInvite(httptest.NewRecorder(), bad)); got != http.StatusBadRequest {
		t.Errorf("malformed body status = %d, want 400", got)
	}
}

func TestAuthRoutesAreRegistered(t *testing.T) {
	f := newHandlerFixture(t)
	mux := http.NewServeMux()
	f.h.Routes(mux, NewAuthenticator(f.issuer))

	// Public.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, post(`{"email":"admin@alpha.in","password":"`+testPassword+`"}`))
	_ = rec

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/auth/me without a token = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/invites", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/auth/invites without a token = %d, want 401", rec.Code)
	}
}

// Argon2id makes each guess cost ~50ms, which raises the price of a brute
// force but does not bound it. This does.
func TestLoginIsRateLimited(t *testing.T) {
	f := newHandlerFixture(t)

	attempt := func(email, password, client string) int {
		r := post(`{"email":"` + email + `","password":"` + password + `"}`)
		r.RemoteAddr = client + ":40000"
		r.Header.Set("User-Agent", "attacker/1")
		return apiStatus(t, f.h.login(httptest.NewRecorder(), r))
	}

	for i := 0; i < loginAttempts; i++ {
		if got := attempt("admin@alpha.in", "wrong-password", "203.0.113.1"); got != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, got)
		}
	}
	if got := attempt("admin@alpha.in", "wrong-password", "203.0.113.1"); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once the budget is spent", got)
	}

	// The email is now blocked from anywhere, so a botnet cannot grind it.
	if got := attempt("admin@alpha.in", "wrong-password", "198.51.100.9"); got != http.StatusTooManyRequests {
		t.Errorf("status = %d from a fresh client; the email budget should still apply", got)
	}

	// A different account from a fresh client is unaffected.
	if got := attempt("owner@example.in", "wrong-password", "198.51.100.9"); got != http.StatusUnauthorized {
		t.Errorf("status = %d; an unrelated account should not be locked out", got)
	}
}

// Someone who has just proved who they are should not be one typo from a lockout.
func TestASuccessfulLoginClearsTheBudget(t *testing.T) {
	f := newHandlerFixture(t)

	fail := func() int {
		r := post(`{"email":"admin@alpha.in","password":"wrong-password"}`)
		r.RemoteAddr = "203.0.113.2:40000"
		return apiStatus(t, f.h.login(httptest.NewRecorder(), r))
	}
	for i := 0; i < loginAttempts-1; i++ {
		fail()
	}

	ok := post(`{"email":"admin@alpha.in","password":"` + testPassword + `"}`)
	ok.RemoteAddr = "203.0.113.2:40000"
	if err := f.h.login(httptest.NewRecorder(), ok); err != nil {
		t.Fatalf("the correct password should still work: %v", err)
	}

	// Budget reset, so the next wrong guess is a 401 and not a 429.
	if got := fail(); got != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 — the budget should have been cleared", got)
	}
}

func TestRefreshIsRateLimited(t *testing.T) {
	f := newHandlerFixture(t)

	attempt := func() int {
		r := post(`{"refreshToken":"guess"}`)
		r.RemoteAddr = "203.0.113.3:40000"
		return apiStatus(t, f.h.refresh(httptest.NewRecorder(), r))
	}
	for i := 0; i < refreshAttempts; i++ {
		if got := attempt(); got != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d", i+1, got)
		}
	}
	if got := attempt(); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", got)
	}
}
