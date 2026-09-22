package media

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

const testUserID = "11111111-1111-4111-8111-111111111111"

// authedContext puts an owner identity in the context, standing in for what
// auth.RequireAuth does in the real chain.
func authedContext(ctx context.Context) context.Context {
	issuer := auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Minute, time.Hour)
	_ = issuer
	return auth.ContextWithIdentity(ctx, auth.Identity{
		UserID: uuid.MustParse(testUserID),
		Role:   domain.RoleOwner,
		Name:   "Owner",
	})
}

func testAuthenticator(t *testing.T) *auth.Authenticator {
	t.Helper()
	return auth.NewAuthenticator(auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Minute, time.Hour))
}
