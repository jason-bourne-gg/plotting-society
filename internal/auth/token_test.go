package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

var secret = []byte(strings.Repeat("s", 48))

func testUser() domain.User {
	builder := uuid.New()
	return domain.User{
		ID:        uuid.New(),
		BuilderID: &builder,
		Name:      "Site Office",
		Role:      domain.RoleBuilderAdmin,
	}
}

func TestIssueAndParseAccess(t *testing.T) {
	issuer := NewTokenIssuer(secret, 15*time.Minute, 720*time.Hour)
	user := testUser()

	token, err := issuer.IssueAccess(user)
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}

	claims, err := issuer.ParseAccess(token)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if claims.Subject != user.ID.String() {
		t.Errorf("subject = %q", claims.Subject)
	}
	if claims.Role != domain.RoleBuilderAdmin {
		t.Errorf("role = %q", claims.Role)
	}
	if claims.BuilderID != user.BuilderID.String() {
		t.Errorf("builder id = %q", claims.BuilderID)
	}
	if claims.Name != "Site Office" {
		t.Errorf("name = %q", claims.Name)
	}
}

func TestIssueAccessForOwnerHasNoBuilder(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	owner := domain.User{ID: uuid.New(), Name: "Owner", Role: domain.RoleOwner}

	token, err := issuer.IssueAccess(owner)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := issuer.ParseAccess(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.BuilderID != "" {
		t.Errorf("an owner token carried a builder id: %q", claims.BuilderID)
	}
}

func TestTTLAccessors(t *testing.T) {
	issuer := NewTokenIssuer(secret, 7*time.Minute, 30*time.Hour)
	if issuer.AccessTTL() != 7*time.Minute {
		t.Errorf("AccessTTL = %v", issuer.AccessTTL())
	}
	if issuer.RefreshTTL() != 30*time.Hour {
		t.Errorf("RefreshTTL = %v", issuer.RefreshTTL())
	}
}

func TestParseAccessRejectsExpired(t *testing.T) {
	issuer := NewTokenIssuer(secret, -time.Minute, time.Hour)
	token, err := issuer.IssueAccess(testUser())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.ParseAccess(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("an expired token should be rejected, got %v", err)
	}
}

func TestParseAccessRejectsOtherSecret(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	token, err := issuer.IssueAccess(testUser())
	if err != nil {
		t.Fatal(err)
	}

	other := NewTokenIssuer([]byte(strings.Repeat("x", 48)), time.Minute, time.Hour)
	if _, err := other.ParseAccess(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("a token signed with another secret should be rejected, got %v", err)
	}
}

// The classic JWT bug: without pinning the algorithm, a token signed with
// "none" parses as valid and anyone can mint an admin session.
func TestParseAccessRejectsNoneAlgorithm(t *testing.T) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: domain.RoleSuperAdmin,
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building the alg=none token: %v", err)
	}

	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	if _, err := issuer.ParseAccess(unsigned); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("an alg=none token was accepted")
	}
}

func TestParseAccessRejectsGarbage(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	for _, token := range []string{"", "abc", "a.b.c", "....."} {
		if _, err := issuer.ParseAccess(token); err == nil {
			t.Errorf("%q should not parse", token)
		}
	}
}

func TestNewOpaqueToken(t *testing.T) {
	token, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken: %v", err)
	}
	if len(token) < 40 {
		t.Errorf("token looks too short: %q", token)
	}
	if hash != HashOpaqueToken(token) {
		t.Error("the returned hash does not match HashOpaqueToken of the token")
	}
	// Storing only the hash is what makes a database leak useless.
	if strings.Contains(hash, token) {
		t.Error("the hash contains the token")
	}

	second, _, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == second {
		t.Fatal("two calls produced the same token")
	}
}

func TestHashOpaqueTokenIsStable(t *testing.T) {
	if HashOpaqueToken("abc") != HashOpaqueToken("abc") {
		t.Error("hashing is not deterministic")
	}
	if HashOpaqueToken("abc") == HashOpaqueToken("abd") {
		t.Error("different tokens hashed the same")
	}
	if len(HashOpaqueToken("abc")) != 64 {
		t.Error("expected a 64-character hex SHA-256")
	}
}

func TestParseUUID(t *testing.T) {
	id := uuid.New()
	got, err := parseUUID(id.String())
	if err != nil || got != id {
		t.Fatalf("parseUUID round trip failed: %v %v", got, err)
	}
	if _, err := parseUUID("nope"); err == nil {
		t.Error("expected an error for a non-UUID")
	}
}
