// Package media hands out short-lived presigned PUT URLs so browsers upload
// photos straight to object storage. The API never touches image bytes, which
// is what keeps a scale-to-zero container inside a free CPU allowance.
//
// SigV4 is implemented here against the standard library rather than pulled in
// from the AWS SDK: it is ~100 lines, it removes three dependencies, and it
// works unchanged against Cloudflare R2, MinIO and S3 itself.
package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/config"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

// allowedTypes is a strict allowlist. Anything not here is refused outright
// rather than sniffed, so the bucket can never be handed an executable.
var allowedTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

const maxUploadBytes = 10 << 20 // 10 MiB

type Signer struct {
	cfg config.S3Config
}

func NewSigner(cfg config.S3Config) *Signer { return &Signer{cfg: cfg} }

// Configured reports whether object storage is wired up. When it is not, the
// upload route returns a clear 503 instead of signing a URL to nowhere.
func (s *Signer) Configured() bool {
	return s.cfg.Endpoint != "" && s.cfg.Bucket != "" && s.cfg.AccessKeyID != "" && s.cfg.SecretKey != ""
}

// PresignPut returns a URL the browser may PUT to for the next 15 minutes.
func (s *Signer) PresignPut(key string, expires time.Duration) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(s.cfg.Endpoint, "/"))
	if err != nil {
		return "", fmt.Errorf("parse S3_ENDPOINT: %w", err)
	}

	// Path-style addressing works on R2, MinIO and S3 alike.
	endpoint.Path = path.Join(endpoint.Path, s.cfg.Bucket, key)

	now := time.Now().UTC()
	stamp := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	scope := strings.Join([]string{day, s.cfg.Region, "s3", "aws4_request"}, "/")

	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", s.cfg.AccessKeyID+"/"+scope)
	q.Set("X-Amz-Date", stamp)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")

	canonicalRequest := strings.Join([]string{
		http.MethodPut,
		uriEncodePath(endpoint.Path),
		q.Encode(),
		"host:" + endpoint.Host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		stamp,
		scope,
		sha256Hex(canonicalRequest),
	}, "\n")

	key1 := hmacSHA256([]byte("AWS4"+s.cfg.SecretKey), day)
	key2 := hmacSHA256(key1, s.cfg.Region)
	key3 := hmacSHA256(key2, "s3")
	signingKey := hmacSHA256(key3, "aws4_request")

	q.Set("X-Amz-Signature", hex.EncodeToString(hmacSHA256(signingKey, stringToSign)))
	endpoint.RawQuery = q.Encode()
	return endpoint.String(), nil
}

// PublicURL is where the object will be readable once uploaded.
func (s *Signer) PublicURL(key string) string {
	base := s.cfg.PublicBaseURL
	if base == "" {
		base = strings.TrimRight(s.cfg.Endpoint, "/") + "/" + s.cfg.Bucket
	}
	return strings.TrimRight(base, "/") + "/" + key
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// uriEncodePath escapes each segment but keeps the separators, which is what
// SigV4 canonicalisation requires.
func uriEncodePath(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// ------------------------------------------------------------------ handler

type Handler struct{ signer *Signer }

func NewHandler(signer *Signer) *Handler { return &Handler{signer: signer} }

func (h *Handler) Routes(mux *http.ServeMux, a *auth.Authenticator) {
	mux.Handle("POST /api/uploads", a.RequireAuth(httpx.Handler(h.presign)))
}

func (h *Handler) presign(w http.ResponseWriter, r *http.Request) error {
	if !h.signer.Configured() {
		return &httpx.Error{
			Status:  http.StatusServiceUnavailable,
			Code:    "storage_unconfigured",
			Message: "File uploads are not set up on this environment yet.",
		}
	}

	identity := auth.MustFromContext(r.Context())

	var req struct {
		// Purpose separates the buckets' folders: site-update photos,
		// query attachments, plot documents.
		Purpose     string `json:"purpose"`
		ContentType string `json:"contentType"`
		SizeBytes   int64  `json:"sizeBytes"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		return err
	}

	fields := map[string]string{}
	ext, ok := allowedTypes[req.ContentType]
	if !ok {
		fields["contentType"] = "Upload a JPG, PNG, WebP or PDF."
	}
	if req.SizeBytes <= 0 || req.SizeBytes > maxUploadBytes {
		fields["sizeBytes"] = "Files must be under 10 MB. Compress the photo and try again."
	}
	folder := map[string]string{
		"site_update":      "updates",
		"query_attachment": "queries",
		"document":         "documents",
		"avatar":           "avatars",
	}[req.Purpose]
	if folder == "" {
		fields["purpose"] = "Unknown upload purpose."
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	// The uploader's id is in the key so an orphaned object can be traced back.
	key := fmt.Sprintf("%s/%s/%s%s", folder, identity.UserID, uuid.NewString(), ext)

	uploadURL, err := h.signer.PresignPut(key, 15*time.Minute)
	if err != nil {
		return httpx.Internal(err)
	}

	return httpx.JSON(w, http.StatusOK, map[string]any{
		"uploadUrl": uploadURL,
		"publicUrl": h.signer.PublicURL(key),
		"key":       key,
		"expiresIn": 900,
	})
}
