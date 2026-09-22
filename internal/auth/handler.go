package auth

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

type Handler struct {
	store  *Store
	tokens *TokenIssuer
	// inviteBaseURL is where the emailed link points, e.g. https://app/invite
	inviteBaseURL string
}

func NewHandler(store *Store, tokens *TokenIssuer, inviteBaseURL string) *Handler {
	return &Handler{store: store, tokens: tokens, inviteBaseURL: inviteBaseURL}
}

// Routes registers everything under /api/auth.
func (h *Handler) Routes(mux *http.ServeMux, a *Authenticator) {
	mux.Handle("POST /api/auth/login", httpx.Handler(h.login))
	mux.Handle("POST /api/auth/refresh", httpx.Handler(h.refresh))
	mux.Handle("GET /api/auth/invite/{token}", httpx.Handler(h.inspectInvite))
	mux.Handle("POST /api/auth/invite/{token}/accept", httpx.Handler(h.acceptInvite))

	authed := func(fn httpx.Handler) http.Handler { return a.RequireAuth(fn) }
	mux.Handle("POST /api/auth/logout", authed(h.logout))
	mux.Handle("GET /api/auth/me", authed(h.me))
	mux.Handle("PATCH /api/auth/me", authed(h.updateMe))
	mux.Handle("POST /api/auth/change-password", authed(h.changePassword))

	staffOnly := func(fn httpx.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(fn, RequireStaff()))
	}
	mux.Handle("POST /api/auth/invites", staffOnly(h.createInvite))
}

type sessionResponse struct {
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	ExpiresIn    int         `json:"expiresIn"`
	User         domain.User `json:"user"`
}

func (h *Handler) issueSession(w http.ResponseWriter, r *http.Request, user domain.User, status int) error {
	access, err := h.tokens.IssueAccess(user)
	if err != nil {
		return httpx.Internal(err)
	}
	refresh, refreshHash, err := NewOpaqueToken()
	if err != nil {
		return httpx.Internal(err)
	}
	expires := time.Now().Add(h.tokens.RefreshTTL())
	if err := h.store.StoreRefreshToken(r.Context(), user.ID, refreshHash, expires, r.UserAgent()); err != nil {
		return httpx.Internal(err)
	}

	return httpx.JSON(w, status, sessionResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int(h.tokens.AccessTTL().Seconds()),
		User:         user,
	})
}

// ------------------------------------------------------------------- login

func (h *Handler) login(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		return httpx.BadRequest("Email and password are required.")
	}

	user, hash, err := h.store.UserByEmail(r.Context(), req.Email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return httpx.Internal(err)
	}

	// Always run a verification, even for an unknown email, so response timing
	// does not reveal which addresses have accounts.
	if errors.Is(err, ErrNotFound) || hash == "" {
		_ = VerifyPassword(req.Password, dummyHash)
		return httpx.Unauthorized("Email or password is incorrect.")
	}
	if err := VerifyPassword(req.Password, hash); err != nil {
		return httpx.Unauthorized("Email or password is incorrect.")
	}
	if !user.IsActive {
		return httpx.Forbidden("This account has been deactivated. Contact the builder's office.")
	}

	_ = h.store.TouchLastLogin(r.Context(), user.ID)
	return h.issueSession(w, r, user, http.StatusOK)
}

// dummyHash is a valid Argon2id hash of a random value, used only to burn the
// same CPU time on a miss as on a hit.
const dummyHash = "$argon2id$v=19$m=65536,t=2,p=2$YWJjZGVmZ2hpamtsbW5vcA$" +
	"c2FtcGxlZHVtbXloYXNodmFsdWVmb3J0aW1pbmdlcQ"

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if req.RefreshToken == "" {
		return httpx.BadRequest("refreshToken is required.")
	}

	// Single-use: consuming revokes the old token and we hand back a new one.
	userID, err := h.store.ConsumeRefreshToken(r.Context(), HashOpaqueToken(req.RefreshToken))
	if errors.Is(err, ErrNotFound) {
		return httpx.Unauthorized("Your session has expired. Sign in again.")
	}
	if err != nil {
		return httpx.Internal(err)
	}

	user, err := h.store.UserByID(r.Context(), userID)
	if err != nil {
		return httpx.Internal(err)
	}
	if !user.IsActive {
		return httpx.Forbidden("This account has been deactivated.")
	}
	return h.issueSession(w, r, user, http.StatusOK)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	id := MustFromContext(r.Context())
	if err := h.store.RevokeAllRefreshTokens(r.Context(), id.UserID); err != nil {
		return httpx.Internal(err)
	}
	return httpx.NoContent(w)
}

// ------------------------------------------------------------------ profile

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	id := MustFromContext(r.Context())
	user, err := h.store.UserByID(r.Context(), id.UserID)
	if errors.Is(err, ErrNotFound) {
		return httpx.Unauthorized("Account no longer exists.")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, user)
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) error {
	id := MustFromContext(r.Context())

	var req struct {
		Name           string `json:"name"`
		Phone          string `json:"phone"`
		CurrentAddress string `json:"currentAddress"`
		City           string `json:"city"`
		DirectoryOptIn bool   `json:"directoryOptIn"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Name) == "" {
		return httpx.Invalid(map[string]string{"name": "Name cannot be empty."})
	}

	user, err := h.store.UpdateProfile(r.Context(), id.UserID,
		strings.TrimSpace(req.Name), strings.TrimSpace(req.Phone),
		strings.TrimSpace(req.CurrentAddress), strings.TrimSpace(req.City), req.DirectoryOptIn)
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, user)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) error {
	id := MustFromContext(r.Context())

	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}

	user, err := h.store.UserByID(r.Context(), id.UserID)
	if err != nil {
		return httpx.Internal(err)
	}
	_, hash, err := h.store.UserByEmail(r.Context(), user.Email)
	if err != nil {
		return httpx.Internal(err)
	}
	if err := VerifyPassword(req.CurrentPassword, hash); err != nil {
		return httpx.Unauthorized("Current password is incorrect.")
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		return httpx.Internal(err)
	}
	if err := h.store.SetPassword(r.Context(), id.UserID, newHash); err != nil {
		return httpx.Internal(err)
	}
	// Changing a password ends every other session.
	if err := h.store.RevokeAllRefreshTokens(r.Context(), id.UserID); err != nil {
		return httpx.Internal(err)
	}
	return httpx.NoContent(w)
}

// ------------------------------------------------------------------ invites

func (h *Handler) createInvite(w http.ResponseWriter, r *http.Request) error {
	caller := MustFromContext(r.Context())

	var req struct {
		Email  string `json:"email"`
		Name   string `json:"name"`
		Role   string `json:"role"`
		PlotID string `json:"plotId,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}

	fields := map[string]string{}
	req.Email = strings.TrimSpace(req.Email)
	if _, err := mail.ParseAddress(req.Email); err != nil {
		fields["email"] = "Enter a valid email address."
	}
	if strings.TrimSpace(req.Name) == "" {
		fields["name"] = "Name is required."
	}
	role := domain.Role(req.Role)
	if !role.Valid() || role == domain.RoleSuperAdmin {
		fields["role"] = "Role must be owner, builder_staff or builder_admin."
	}
	// Only a builder admin may mint another admin or staff account.
	if role != domain.RoleOwner && !caller.Role.CanManageBuilder() {
		fields["role"] = "Only a builder admin can invite staff."
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	invite := Invite{
		Email:     req.Email,
		Name:      strings.TrimSpace(req.Name),
		Role:      role,
		BuilderID: caller.BuilderID,
		ExpiresAt: time.Now().Add(14 * 24 * time.Hour),
	}
	if role == domain.RoleOwner {
		// An owner invite is meaningless without the plot it binds to.
		if req.PlotID == "" {
			return httpx.Invalid(map[string]string{"plotId": "Select the plot this owner belongs to."})
		}
		plotID, err := uuid.Parse(req.PlotID)
		if err != nil {
			return httpx.Invalid(map[string]string{"plotId": "Not a valid plot id."})
		}
		invite.PlotID = &plotID
		invite.BuilderID = nil
	}

	token, tokenHash, err := NewOpaqueToken()
	if err != nil {
		return httpx.Internal(err)
	}
	if _, err := h.store.CreateInvite(r.Context(), invite, tokenHash, caller.UserID); err != nil {
		return httpx.Internal(err)
	}

	return httpx.JSON(w, http.StatusCreated, map[string]any{
		"email":     invite.Email,
		"role":      invite.Role,
		"expiresAt": invite.ExpiresAt,
		// Returned so the builder can share it directly when email is
		// unreliable, which for this audience it often is.
		"inviteUrl": strings.TrimRight(h.inviteBaseURL, "/") + "/" + token,
	})
}

func (h *Handler) inspectInvite(w http.ResponseWriter, r *http.Request) error {
	invite, err := h.store.InviteByTokenHash(r.Context(), HashOpaqueToken(r.PathValue("token")))
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("This invite link is invalid or has expired.")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{
		"email": invite.Email, "name": invite.Name, "role": invite.Role,
	})
}

func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}
	if err := validatePassword(req.Password); err != nil {
		return err
	}

	invite, err := h.store.InviteByTokenHash(r.Context(), HashOpaqueToken(r.PathValue("token")))
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("This invite link is invalid or has expired.")
	}
	if err != nil {
		return httpx.Internal(err)
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return httpx.Internal(err)
	}
	user, err := h.store.AcceptInvite(r.Context(), invite, hash)
	if err != nil {
		return httpx.Internal(err)
	}
	return h.issueSession(w, r, user, http.StatusCreated)
}

func validatePassword(p string) error {
	if len(p) < 10 {
		return httpx.Invalid(map[string]string{"password": "Use at least 10 characters."})
	}
	if len(p) > 200 {
		return httpx.Invalid(map[string]string{"password": "That password is too long."})
	}
	return nil
}
