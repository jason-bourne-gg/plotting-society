package config

import (
	"strings"
	"testing"
	"time"
)

// valid is the minimum that makes Load succeed.
func valid(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("JWT_SECRET", strings.Repeat("k", 48))
}

func TestLoadDefaults(t *testing.T) {
	valid(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "development" {
		t.Errorf("Env = %q", cfg.Env)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Errorf("AccessTokenTTL = %v", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTTL != 720*time.Hour {
		t.Errorf("RefreshTTL = %v", cfg.RefreshTTL)
	}
	if cfg.IsProduction() {
		t.Error("development config should not report as production")
	}
	if len(cfg.CORSOrigins) != 0 {
		t.Errorf("CORSOrigins = %v, want empty", cfg.CORSOrigins)
	}
}

func TestLoadOverrides(t *testing.T) {
	valid(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_ADDR", ":9999")
	t.Setenv("PUBLIC_BASE_URL", "https://app.example.in")
	t.Setenv("ACCESS_TOKEN_TTL", "5m")
	t.Setenv("REFRESH_TOKEN_TTL", "48h")
	t.Setenv("S3_BUCKET", "media")
	t.Setenv("RESEND_API_KEY", "re_x")
	t.Setenv("MAIL_FROM", "a@b.in")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.IsProduction() {
		t.Error("APP_ENV=production should report as production")
	}
	if cfg.HTTPAddr != ":9999" || cfg.AccessTokenTTL != 5*time.Minute ||
		cfg.RefreshTTL != 48*time.Hour || cfg.S3.Bucket != "media" ||
		cfg.ResendAPIKey != "re_x" || cfg.MailFrom != "a@b.in" ||
		cfg.PublicBaseURL != "https://app.example.in" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

// Blank entries and stray whitespace in the list must not become an origin,
// because an empty allowed origin would match nothing and hide a typo.
func TestLoadCORSOrigins(t *testing.T) {
	valid(t)
	t.Setenv("CORS_ORIGINS", " https://a.in , ,https://b.in,  ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"https://a.in", "https://b.in"}
	if len(cfg.CORSOrigins) != len(want) {
		t.Fatalf("CORSOrigins = %v, want %v", cfg.CORSOrigins, want)
	}
	for i, o := range want {
		if cfg.CORSOrigins[i] != o {
			t.Errorf("origin[%d] = %q, want %q", i, cfg.CORSOrigins[i], o)
		}
	}
}

func TestLoadMissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error when required values are missing")
	}
	// Both problems should be reported together, not one per run.
	if !strings.Contains(err.Error(), "DATABASE_URL") || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Errorf("error should name every missing key, got: %v", err)
	}
}

func TestLoadShortSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "too-short")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "at least 32") {
		t.Fatalf("expected a minimum-length complaint, got %v", err)
	}
}

// The example secret in .env.example must never boot a production deploy.
func TestLoadRejectsExampleSecretInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "change-me-to-a-48-byte-random-string-padding")
	t.Setenv("APP_ENV", "production")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "example value") {
		t.Fatalf("expected the example secret to be rejected, got %v", err)
	}
}

func TestLoadAllowsExampleSecretInDevelopment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "change-me-to-a-48-byte-random-string-padding")
	t.Setenv("APP_ENV", "development")

	if _, err := Load(); err != nil {
		t.Fatalf("development should tolerate the example secret: %v", err)
	}
}

func TestLoadBadDuration(t *testing.T) {
	valid(t)
	t.Setenv("ACCESS_TOKEN_TTL", "fifteen minutes")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ACCESS_TOKEN_TTL") {
		t.Fatalf("expected a duration complaint, got %v", err)
	}
}
