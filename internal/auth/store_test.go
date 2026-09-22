package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

type storeFixture struct {
	db      *database.DB
	store   *Store
	builder testsupport.Builder
	admin   uuid.UUID
	plot    uuid.UUID
}

const testPassword = "a-long-enough-password"

func newStoreFixture(t *testing.T) storeFixture {
	t.Helper()
	db := testsupport.DB(t, "auth")

	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}

	f := storeFixture{db: db, store: NewStore(db)}
	f.builder = testsupport.NewBuilder(t, db, "Alpha")
	f.admin = testsupport.NewUser(t, db, &f.builder.ID, domain.RoleBuilderAdmin, "admin@alpha.in", hash)
	f.plot = testsupport.NewPlot(t, db, f.builder.SocietyID, "1", "available")
	return f
}

func TestUserByEmail(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	user, hash, err := f.store.UserByEmail(ctx, "admin@alpha.in")
	if err != nil {
		t.Fatalf("UserByEmail: %v", err)
	}
	if user.ID != f.admin || user.Role != domain.RoleBuilderAdmin {
		t.Errorf("unexpected user: %+v", user)
	}
	if err := VerifyPassword(testPassword, hash); err != nil {
		t.Errorf("the stored hash does not verify: %v", err)
	}

	// Sign-in must not depend on how the address was typed.
	if _, _, err := f.store.UserByEmail(ctx, "ADMIN@Alpha.IN"); err != nil {
		t.Errorf("lookup should be case-insensitive: %v", err)
	}

	if _, _, err := f.store.UserByEmail(ctx, "nobody@alpha.in"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUserByID(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	user, err := f.store.UserByID(ctx, f.admin)
	if err != nil || user.Email != "admin@alpha.in" {
		t.Fatalf("UserByID: %+v %v", user, err)
	}
	if _, err := f.store.UserByID(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTouchLastLoginAndUpdateProfile(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	if err := f.store.TouchLastLogin(ctx, f.admin); err != nil {
		t.Fatalf("TouchLastLogin: %v", err)
	}
	var last *time.Time
	if err := f.db.QueryRow(ctx, `SELECT last_login_at FROM users WHERE id = $1`, f.admin).Scan(&last); err != nil {
		t.Fatal(err)
	}
	if last == nil {
		t.Error("last_login_at was not set")
	}

	updated, err := f.store.UpdateProfile(ctx, f.admin, "New Name", "+91 90000 11111",
		"Flat 1, Somewhere", "Pune", true)
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Name != "New Name" || updated.Phone != "+91 90000 11111" ||
		updated.City != "Pune" || !updated.DirectoryOptIn {
		t.Errorf("profile not updated: %+v", updated)
	}

	// Blank optional fields become NULL and read back as empty, not "".
	cleared, err := f.store.UpdateProfile(ctx, f.admin, "New Name", "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Phone != "" || cleared.City != "" || cleared.DirectoryOptIn {
		t.Errorf("fields were not cleared: %+v", cleared)
	}
}

func TestSetPassword(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	newHash, err := HashPassword("a-different-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetPassword(ctx, f.admin, newHash); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	_, stored, err := f.store.UserByEmail(ctx, "admin@alpha.in")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword("a-different-password", stored); err != nil {
		t.Errorf("the new password does not verify: %v", err)
	}
	if err := VerifyPassword(testPassword, stored); err == nil {
		t.Error("the old password still works after a change")
	}
}

func TestInviteLifecycle(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	token, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	invite := Invite{
		Email: "owner@example.in", Name: "Rohit", Role: domain.RoleOwner,
		PlotID: &f.plot, ExpiresAt: time.Now().Add(14 * 24 * time.Hour),
	}
	if _, err := f.store.CreateInvite(ctx, invite, hash, f.admin); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	found, err := f.store.InviteByTokenHash(ctx, HashOpaqueToken(token))
	if err != nil {
		t.Fatalf("InviteByTokenHash: %v", err)
	}
	if found.Email != "owner@example.in" || found.PlotID == nil || *found.PlotID != f.plot {
		t.Errorf("unexpected invite: %+v", found)
	}

	// Only the hash is stored, so the raw token must not find anything.
	if _, err := f.store.InviteByTokenHash(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Error("the raw token should not match the stored hash")
	}

	passwordHash, err := HashPassword("owner-password-here")
	if err != nil {
		t.Fatal(err)
	}
	user, err := f.store.AcceptInvite(ctx, found, passwordHash)
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if user.Role != domain.RoleOwner || user.Email != "owner@example.in" {
		t.Errorf("unexpected user: %+v", user)
	}

	// Accepting links the plot and marks it sold, in the same transaction.
	var ownerID *uuid.UUID
	var status string
	if err := f.db.QueryRow(ctx,
		`SELECT owner_id, status FROM plots WHERE id = $1`, f.plot).Scan(&ownerID, &status); err != nil {
		t.Fatal(err)
	}
	if ownerID == nil || *ownerID != user.ID {
		t.Error("the plot was not linked to the new owner")
	}
	if status != "sold" {
		t.Errorf("plot status = %q, want sold", status)
	}

	// An invite is single use.
	if _, err := f.store.InviteByTokenHash(ctx, HashOpaqueToken(token)); !errors.Is(err, ErrNotFound) {
		t.Error("an accepted invite should no longer be findable")
	}
}

func TestInviteByTokenHashRejectsExpired(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	token, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateInvite(ctx, Invite{
		Email: "late@example.in", Name: "Late", Role: domain.RoleOwner,
		ExpiresAt: time.Now().Add(-time.Hour),
	}, hash, f.admin); err != nil {
		t.Fatal(err)
	}

	if _, err := f.store.InviteByTokenHash(ctx, HashOpaqueToken(token)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an expired invite should not be usable, got %v", err)
	}
}

// An invite without a plot is how staff accounts are created.
func TestAcceptInviteWithoutPlot(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	_, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	invite := Invite{
		Email: "staff@alpha.in", Name: "Prashant", Role: domain.RoleBuilderStaff,
		BuilderID: &f.builder.ID, ExpiresAt: time.Now().Add(time.Hour),
	}
	id, err := f.store.CreateInvite(ctx, invite, hash, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	invite.ID = id

	pw, _ := HashPassword("staff-password-x")
	user, err := f.store.AcceptInvite(ctx, invite, pw)
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if user.BuilderID == nil || *user.BuilderID != f.builder.ID {
		t.Error("staff should be attached to their builder")
	}
}

// A duplicate email must fail the whole transaction, not leave a half-accepted
// invite that strands the owner.
func TestAcceptInviteRollsBackOnDuplicateEmail(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	_, hash, _ := NewOpaqueToken()
	invite := Invite{
		Email: "admin@alpha.in", // already exists
		Name:  "Clash", Role: domain.RoleOwner, PlotID: &f.plot,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	id, err := f.store.CreateInvite(ctx, invite, hash, f.admin)
	if err != nil {
		t.Fatal(err)
	}
	invite.ID = id

	pw, _ := HashPassword("whatever-password")
	if _, err := f.store.AcceptInvite(ctx, invite, pw); err == nil {
		t.Fatal("expected a duplicate-email failure")
	}

	var ownerID *uuid.UUID
	if err := f.db.QueryRow(ctx, `SELECT owner_id FROM plots WHERE id = $1`, f.plot).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if ownerID != nil {
		t.Error("the plot was linked despite the failure — the transaction did not roll back")
	}
}

func TestRefreshTokenLifecycle(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	token, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.StoreRefreshToken(ctx, f.admin, hash, time.Now().Add(time.Hour), "curl/8"); err != nil {
		t.Fatalf("StoreRefreshToken: %v", err)
	}

	userID, err := f.store.ConsumeRefreshToken(ctx, HashOpaqueToken(token))
	if err != nil {
		t.Fatalf("ConsumeRefreshToken: %v", err)
	}
	if userID != f.admin {
		t.Errorf("user id = %v", userID)
	}

	// Single use: replaying a stolen refresh token must fail.
	if _, err := f.store.ConsumeRefreshToken(ctx, HashOpaqueToken(token)); !errors.Is(err, ErrNotFound) {
		t.Fatal("a refresh token was accepted twice")
	}
}

func TestConsumeRefreshTokenRejectsExpired(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	token, hash, _ := NewOpaqueToken()
	if err := f.store.StoreRefreshToken(ctx, f.admin, hash, time.Now().Add(-time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ConsumeRefreshToken(ctx, HashOpaqueToken(token)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an expired refresh token should be rejected, got %v", err)
	}
}

// Changing a password must end every other session.
func TestRevokeAllRefreshTokens(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()

	var tokens []string
	for i := 0; i < 3; i++ {
		token, hash, _ := NewOpaqueToken()
		if err := f.store.StoreRefreshToken(ctx, f.admin, hash, time.Now().Add(time.Hour), ""); err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}

	if err := f.store.RevokeAllRefreshTokens(ctx, f.admin); err != nil {
		t.Fatalf("RevokeAllRefreshTokens: %v", err)
	}
	for i, token := range tokens {
		if _, err := f.store.ConsumeRefreshToken(ctx, HashOpaqueToken(token)); !errors.Is(err, ErrNotFound) {
			t.Errorf("session %d survived the revoke", i)
		}
	}
}
