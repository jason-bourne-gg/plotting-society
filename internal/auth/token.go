package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

// Claims is the access-token payload. Role and builder id ride along so the
// common authorisation checks need no database round trip.
type Claims struct {
	jwt.RegisteredClaims
	Role      domain.Role `json:"role"`
	BuilderID string      `json:"bid,omitempty"`
	Name      string      `json:"name"`
}

var ErrInvalidToken = errors.New("invalid or expired token")

type TokenIssuer struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenIssuer(secret []byte, accessTTL, refreshTTL time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (t *TokenIssuer) AccessTTL() time.Duration  { return t.accessTTL }
func (t *TokenIssuer) RefreshTTL() time.Duration { return t.refreshTTL }

func (t *TokenIssuer) IssueAccess(u domain.User) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.accessTTL)),
			NotBefore: jwt.NewNumericDate(now),
		},
		Role: u.Role,
		Name: u.Name,
	}
	if u.BuilderID != nil {
		claims.BuilderID = u.BuilderID.String()
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

func (t *TokenIssuer) ParseAccess(raw string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(tok *jwt.Token) (any, error) {
		// Pin the algorithm: without this, a token signed with "none" parses.
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", tok.Header["alg"])
		}
		return t.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// NewOpaqueToken returns a random token for refresh and invite flows, plus the
// SHA-256 of it. Only the hash is stored, so a database leak yields no
// usable sessions or invite links.
func NewOpaqueToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("read random: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, HashOpaqueToken(token), nil
}

func HashOpaqueToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func parseUUID(s string) (uuid.UUID, error) { return uuid.Parse(s) }
