package plot

import (
	"strings"
	"time"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
)

func testAuthenticator() *auth.Authenticator {
	return auth.NewAuthenticator(
		auth.NewTokenIssuer([]byte(strings.Repeat("k", 48)), time.Minute, time.Hour))
}
