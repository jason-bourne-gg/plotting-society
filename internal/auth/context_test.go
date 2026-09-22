package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

func authedRequest(t *testing.T, issuer *TokenIssuer, user domain.User) *http.Request {
	t.Helper()
	token, err := issuer.IssueAccess(user)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func okHandler(seen *Identity) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, found := FromContext(r.Context()); found && seen != nil {
			*seen = id
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuthAcceptsValidToken(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	user := testUser()

	var seen Identity
	rec := httptest.NewRecorder()
	NewAuthenticator(issuer).RequireAuth(okHandler(&seen)).
		ServeHTTP(rec, authedRequest(t, issuer, user))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if seen.UserID != user.ID {
		t.Errorf("user id = %v, want %v", seen.UserID, user.ID)
	}
	if seen.BuilderID == nil || *seen.BuilderID != *user.BuilderID {
		t.Errorf("builder id = %v", seen.BuilderID)
	}
	if seen.Role != domain.RoleBuilderAdmin {
		t.Errorf("role = %q", seen.Role)
	}
}

func TestRequireAuthRejectsMissingAndBadTokens(t *testing.T) {
	a := NewAuthenticator(NewTokenIssuer(secret, time.Minute, time.Hour))

	cases := map[string]string{
		"no header":       "",
		"wrong scheme":    "Basic abc",
		"empty bearer":    "Bearer ",
		"garbage token":   "Bearer not-a-jwt",
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			if header != "" {
				r.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			a.RequireAuth(okHandler(nil)).ServeHTTP(rec, r)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// A token whose subject is not a UUID, or whose role is not one we issue, is a
// forgery attempt or a version skew. Either way it must not become a session.
func TestRequireAuthRejectsMalformedClaims(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	a := NewAuthenticator(issuer)

	mint := func(subject string, role domain.Role) string {
		claims := Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   subject,
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Role: role,
		}
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}

	cases := map[string]string{
		"subject is not a uuid": mint("not-a-uuid", domain.RoleOwner),
		"unknown role":          mint(uuid.NewString(), domain.Role("root")),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			a.RequireAuth(okHandler(nil)).ServeHTTP(rec, r)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// Optional is what lets the layout map show more to a signed-in owner while
// staying open to a guest.
func TestOptional(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	a := NewAuthenticator(issuer)
	user := testUser()

	t.Run("with a token", func(t *testing.T) {
		var seen Identity
		rec := httptest.NewRecorder()
		a.Optional(okHandler(&seen)).ServeHTTP(rec, authedRequest(t, issuer, user))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if seen.UserID != user.ID {
			t.Error("the identity was not attached")
		}
	})

	t.Run("without a token", func(t *testing.T) {
		called := false
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if _, found := FromContext(r.Context()); found {
				t.Error("a guest request should carry no identity")
			}
			w.WriteHeader(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		a.Optional(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

		if !called || rec.Code != http.StatusOK {
			t.Fatalf("a guest request should pass through: called=%v status=%d", called, rec.Code)
		}
	})
}

func TestRequireRole(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	a := NewAuthenticator(issuer)

	staffOnly := func(next http.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(next, RequireStaff()))
	}

	t.Run("staff allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		staffOnly(okHandler(nil)).ServeHTTP(rec, authedRequest(t, issuer, testUser()))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("owner forbidden", func(t *testing.T) {
		owner := domain.User{ID: uuid.New(), Name: "O", Role: domain.RoleOwner}
		rec := httptest.NewRecorder()
		staffOnly(okHandler(nil)).ServeHTTP(rec, authedRequest(t, issuer, owner))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		// RequireRole reached without RequireAuth in front of it.
		rec := httptest.NewRecorder()
		httpx.Chain(okHandler(nil), RequireStaff()).
			ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}

// Only a builder admin may mint another admin or a staff account.
func TestRequireBuilderAdmin(t *testing.T) {
	issuer := NewTokenIssuer(secret, time.Minute, time.Hour)
	a := NewAuthenticator(issuer)
	guard := func(next http.Handler) http.Handler {
		return a.RequireAuth(httpx.Chain(next, RequireBuilderAdmin()))
	}

	builder := uuid.New()
	staff := domain.User{ID: uuid.New(), BuilderID: &builder, Name: "S", Role: domain.RoleBuilderStaff}

	rec := httptest.NewRecorder()
	guard(okHandler(nil)).ServeHTTP(rec, authedRequest(t, issuer, staff))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("site staff should not manage the builder: status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	guard(okHandler(nil)).ServeHTTP(rec, authedRequest(t, issuer, testUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("a builder admin should be allowed: status = %d", rec.Code)
	}
}

func TestMustFromContextPanicsOnPublicRoute(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustFromContext should panic when no identity is present")
		}
	}()
	MustFromContext(context.Background())
}

func TestMustFromContextReturnsIdentity(t *testing.T) {
	want := Identity{UserID: uuid.New(), Role: domain.RoleOwner, Name: "O"}
	ctx := context.WithValue(context.Background(), ctxKey{}, want)
	if got := MustFromContext(ctx); got.UserID != want.UserID {
		t.Errorf("got %v, want %v", got, want)
	}
}
