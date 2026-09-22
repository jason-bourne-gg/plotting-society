// Package config loads and validates every environment variable the API needs.
// It fails loudly at boot rather than at the first request that needs a value.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env            string
	HTTPAddr       string
	PublicBaseURL  string
	CORSOrigins    []string
	DatabaseURL    string
	JWTSecret      []byte
	AccessTokenTTL time.Duration
	RefreshTTL     time.Duration
	S3             S3Config
	ResendAPIKey   string
	MailFrom       string
}

type S3Config struct {
	Endpoint      string
	Region        string
	Bucket        string
	AccessKeyID   string
	SecretKey     string
	PublicBaseURL string
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads the process environment. Every error found is reported together so
// a misconfigured deploy takes one round trip to fix, not five.
func Load() (Config, error) {
	var problems []string

	require := func(key string) string {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			problems = append(problems, key+" is required")
		}
		return v
	}
	optional := func(key, fallback string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return fallback
	}
	duration := func(key, fallback string) time.Duration {
		raw := optional(key, fallback)
		d, err := time.ParseDuration(raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s is not a duration: %q", key, raw))
			return 0
		}
		return d
	}

	cfg := Config{
		Env:            optional("APP_ENV", "development"),
		HTTPAddr:       optional("HTTP_ADDR", ":8080"),
		PublicBaseURL:  optional("PUBLIC_BASE_URL", "http://localhost:8080"),
		DatabaseURL:    require("DATABASE_URL"),
		AccessTokenTTL: duration("ACCESS_TOKEN_TTL", "15m"),
		RefreshTTL:     duration("REFRESH_TOKEN_TTL", "720h"),
		ResendAPIKey:   optional("RESEND_API_KEY", ""),
		MailFrom:       optional("MAIL_FROM", "no-reply@example.com"),
		S3: S3Config{
			Endpoint:      optional("S3_ENDPOINT", ""),
			Region:        optional("S3_REGION", "auto"),
			Bucket:        optional("S3_BUCKET", ""),
			AccessKeyID:   optional("S3_ACCESS_KEY_ID", ""),
			SecretKey:     optional("S3_SECRET_ACCESS_KEY", ""),
			PublicBaseURL: optional("S3_PUBLIC_BASE_URL", ""),
		},
	}

	for _, origin := range strings.Split(optional("CORS_ORIGINS", ""), ",") {
		if o := strings.TrimSpace(origin); o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}

	secret := require("JWT_SECRET")
	// 32 bytes is the floor for HS256 to be worth anything.
	if len(secret) < 32 {
		problems = append(problems, "JWT_SECRET must be at least 32 characters")
	}
	cfg.JWTSecret = []byte(secret)

	if cfg.IsProduction() && strings.Contains(secret, "change-me") {
		problems = append(problems, "JWT_SECRET is still the example value")
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}
