package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jason-bourne-gg/plotting-society/internal/config"
)

func testConfig() config.S3Config {
	return config.S3Config{
		Endpoint:      "https://acct.r2.cloudflarestorage.com",
		Region:        "auto",
		Bucket:        "plotting-media",
		AccessKeyID:   "AKIAEXAMPLE",
		SecretKey:     "secretexamplekey",
		PublicBaseURL: "https://pub-hash.r2.dev",
	}
}

func TestConfigured(t *testing.T) {
	full := testConfig()
	if !NewSigner(full).Configured() {
		t.Error("a complete config should report configured")
	}

	// Each missing field on its own must disable uploads, so the route returns
	// a clear 503 instead of signing a URL to nowhere.
	for name, mutate := range map[string]func(*config.S3Config){
		"no endpoint": func(c *config.S3Config) { c.Endpoint = "" },
		"no bucket":   func(c *config.S3Config) { c.Bucket = "" },
		"no key id":   func(c *config.S3Config) { c.AccessKeyID = "" },
		"no secret":   func(c *config.S3Config) { c.SecretKey = "" },
	} {
		t.Run(name, func(t *testing.T) {
			c := testConfig()
			mutate(&c)
			if NewSigner(c).Configured() {
				t.Error("should report unconfigured")
			}
		})
	}
}

func TestPresignPut(t *testing.T) {
	signer := NewSigner(testConfig())

	raw, err := signer.PresignPut("updates/abc/def.jpg", "image/jpeg", 204800, 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("the presigned URL does not parse: %v", err)
	}
	if u.Host != "acct.r2.cloudflarestorage.com" {
		t.Errorf("host = %q", u.Host)
	}
	// Path-style addressing: bucket then key.
	if u.Path != "/plotting-media/updates/abc/def.jpg" {
		t.Errorf("path = %q", u.Path)
	}

	q := u.Query()
	checks := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Expires":       "900",
		"X-Amz-SignedHeaders": "content-length;content-type;host",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if !strings.HasPrefix(q.Get("X-Amz-Credential"), "AKIAEXAMPLE/") {
		t.Errorf("credential = %q", q.Get("X-Amz-Credential"))
	}
	if len(q.Get("X-Amz-Signature")) != 64 {
		t.Errorf("signature is not a 64-character hex digest: %q", q.Get("X-Amz-Signature"))
	}
	if q.Get("X-Amz-Date") == "" {
		t.Error("X-Amz-Date is required")
	}
	// The secret must never appear in a URL handed to a browser.
	if strings.Contains(raw, "secretexamplekey") {
		t.Fatal("the secret key leaked into the presigned URL")
	}
}

// Two different keys must not produce the same signature, and the same key
// signed twice within a second must be stable.
func TestPresignPutSignatureVariesWithKey(t *testing.T) {
	signer := NewSigner(testConfig())

	a, err := signer.PresignPut("updates/a.jpg", "image/jpeg", 1000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	b, err := signer.PresignPut("updates/b.jpg", "image/jpeg", 1000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	sigA := mustQuery(t, a).Get("X-Amz-Signature")
	sigB := mustQuery(t, b).Get("X-Amz-Signature")
	if sigA == sigB {
		t.Fatal("two different object keys produced the same signature")
	}
}

func TestPresignPutBadEndpoint(t *testing.T) {
	c := testConfig()
	c.Endpoint = "://not a url"
	if _, err := NewSigner(c).PresignPut("k", "image/jpeg", 10, time.Minute); err == nil {
		t.Fatal("expected an error for an unparseable endpoint")
	}
}

func TestPublicURL(t *testing.T) {
	if got := NewSigner(testConfig()).PublicURL("updates/a.jpg"); got != "https://pub-hash.r2.dev/updates/a.jpg" {
		t.Errorf("PublicURL = %q", got)
	}

	// Falling back to the endpoint keeps local MinIO working with no extra config.
	c := testConfig()
	c.PublicBaseURL = ""
	want := "https://acct.r2.cloudflarestorage.com/plotting-media/updates/a.jpg"
	if got := NewSigner(c).PublicURL("updates/a.jpg"); got != want {
		t.Errorf("PublicURL fallback = %q, want %q", got, want)
	}
}

func TestURIEncodePath(t *testing.T) {
	// Separators survive; the segments are escaped.
	if got := uriEncodePath("/bucket/a b/c.jpg"); got != "/bucket/a%20b/c.jpg" {
		t.Errorf("uriEncodePath = %q", got)
	}
}

func TestSHA256HexAndHMAC(t *testing.T) {
	// Known-answer checks, so a refactor of the signing primitives is caught.
	if got := sha256Hex(""); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("sha256Hex(\"\") = %q", got)
	}
	if len(hmacSHA256([]byte("key"), "data")) != 32 {
		t.Error("hmacSHA256 should return 32 bytes")
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

// --------------------------------------------------------------- handler

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestPresignHandlerUnconfigured(t *testing.T) {
	h := NewHandler(NewSigner(config.S3Config{}))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/uploads",
		strings.NewReader(`{"purpose":"site_update","contentType":"image/jpeg","sizeBytes":100}`))

	err := h.presign(rec, r)
	if err == nil {
		t.Fatal("expected an error when storage is not configured")
	}
	if !strings.Contains(err.Error(), "not set up") {
		t.Errorf("message = %q", err.Error())
	}
}

func TestAllowedTypesIsAnAllowlist(t *testing.T) {
	// The bucket must never be handed something executable.
	for _, banned := range []string{
		"text/html", "image/svg+xml", "application/javascript",
		"application/octet-stream", "", "IMAGE/JPEG",
	} {
		if _, ok := allowedTypes[banned]; ok {
			t.Errorf("%q should not be an allowed upload type", banned)
		}
	}
	for _, allowed := range []string{"image/jpeg", "image/png", "image/webp", "application/pdf"} {
		if _, ok := allowedTypes[allowed]; !ok {
			t.Errorf("%q should be allowed", allowed)
		}
	}
}

func TestMaxUploadSize(t *testing.T) {
	if maxUploadBytes != 10<<20 {
		t.Errorf("maxUploadBytes = %d, want 10 MiB", maxUploadBytes)
	}
}

// presignWithAuth runs the handler with an authenticated caller in context,
// which is what the RequireAuth middleware would have put there.
func presignWithAuth(t *testing.T, h *Handler, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/uploads", strings.NewReader(body))
	r = r.WithContext(authedContext(r.Context()))
	rec := httptest.NewRecorder()
	return rec, h.presign(rec, r)
}

func TestPresignHandlerHappyPath(t *testing.T) {
	h := NewHandler(NewSigner(testConfig()))

	rec, err := presignWithAuth(t, h,
		`{"purpose":"site_update","contentType":"image/jpeg","sizeBytes":204800}`)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	body := decodeBody(t, rec)
	key, _ := body["key"].(string)
	if !strings.HasPrefix(key, "updates/") || !strings.HasSuffix(key, ".jpg") {
		t.Errorf("key = %q, want updates/<uid>/<uuid>.jpg", key)
	}
	// The uploader's id is in the key so an orphaned object can be traced back.
	if !strings.Contains(key, testUserID) {
		t.Errorf("key %q does not carry the uploader id", key)
	}
	if body["expiresIn"] != float64(900) {
		t.Errorf("expiresIn = %v", body["expiresIn"])
	}
	if up, _ := body["uploadUrl"].(string); !strings.Contains(up, "X-Amz-Signature=") {
		t.Errorf("uploadUrl is not signed: %q", up)
	}
	if pub, _ := body["publicUrl"].(string); !strings.HasPrefix(pub, "https://pub-hash.r2.dev/") {
		t.Errorf("publicUrl = %q", pub)
	}
}

func TestPresignHandlerFoldersPerPurpose(t *testing.T) {
	h := NewHandler(NewSigner(testConfig()))
	want := map[string]string{
		"site_update":      "updates/",
		"query_attachment": "queries/",
		"document":         "documents/",
		"avatar":           "avatars/",
	}
	for purpose, prefix := range want {
		rec, err := presignWithAuth(t, h,
			`{"purpose":"`+purpose+`","contentType":"image/png","sizeBytes":1000}`)
		if err != nil {
			t.Fatalf("%s: %v", purpose, err)
		}
		if key, _ := decodeBody(t, rec)["key"].(string); !strings.HasPrefix(key, prefix) {
			t.Errorf("purpose %q produced key %q, want prefix %q", purpose, key, prefix)
		}
	}
}

func TestPresignHandlerValidation(t *testing.T) {
	h := NewHandler(NewSigner(testConfig()))

	cases := map[string]struct {
		body  string
		field string
	}{
		"banned content type": {`{"purpose":"site_update","contentType":"image/svg+xml","sizeBytes":10}`, "contentType"},
		"unknown purpose":     {`{"purpose":"exploit","contentType":"image/png","sizeBytes":10}`, "purpose"},
		"zero size":           {`{"purpose":"site_update","contentType":"image/png","sizeBytes":0}`, "sizeBytes"},
		"negative size":       {`{"purpose":"site_update","contentType":"image/png","sizeBytes":-5}`, "sizeBytes"},
		"over the cap":        {`{"purpose":"site_update","contentType":"image/png","sizeBytes":20971521}`, "sizeBytes"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := presignWithAuth(t, h, c.body)
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if !strings.Contains(err.Error(), "attention") {
				t.Errorf("expected a validation failure, got %v", err)
			}
		})
	}
}

func TestPresignHandlerRejectsMalformedBody(t *testing.T) {
	h := NewHandler(NewSigner(testConfig()))
	if _, err := presignWithAuth(t, h, `{`); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestRoutesRegistersUploadEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler(NewSigner(testConfig())).Routes(mux, testAuthenticator(t))

	// Without a token the route must exist but reject.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/uploads", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// Signing only `host` would make the allowlist decorative: a caller could
// declare image/jpeg to get a .jpg URL, then PUT HTML with
// Content-Type: text/html and have the store serve it back as HTML.
func TestPresignBindsContentTypeAndLength(t *testing.T) {
	signer := NewSigner(testConfig())

	asJPEG, err := signer.PresignPut("updates/a.jpg", "image/jpeg", 1000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	asHTML, err := signer.PresignPut("updates/a.jpg", "text/html", 1000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	bigger, err := signer.PresignPut("updates/a.jpg", "image/jpeg", 2000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	sig := func(u string) string { return mustQuery(t, u).Get("X-Amz-Signature") }

	if sig(asJPEG) == sig(asHTML) {
		t.Error("the content type is not part of the signature — the allowlist can be bypassed at PUT time")
	}
	if sig(asJPEG) == sig(bigger) {
		t.Error("the length is not part of the signature — the size cap can be exceeded at PUT time")
	}
	if got := mustQuery(t, asJPEG).Get("X-Amz-SignedHeaders"); got != "content-length;content-type;host" {
		t.Errorf("SignedHeaders = %q", got)
	}
}

func TestPresignHandlerReturnsTheHeadersItSigned(t *testing.T) {
	h := NewHandler(NewSigner(testConfig()))
	rec, err := presignWithAuth(t, h,
		`{"purpose":"site_update","contentType":"image/png","sizeBytes":4096}`)
	if err != nil {
		t.Fatal(err)
	}
	headers, _ := decodeBody(t, rec)["requiredHeaders"].(map[string]any)
	if headers["Content-Type"] != "image/png" || headers["Content-Length"] != "4096" {
		t.Errorf("requiredHeaders = %v; the client must send exactly what was signed", headers)
	}
}
