// Package httpx holds the HTTP plumbing shared by every module: JSON encoding,
// a typed error that carries its own status code, and the middleware chain.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Error is an API error that knows its own HTTP status. Handlers return one of
// these and the writer turns it into a JSON body; nothing else decides status
// codes, so a 500 always means "we did not anticipate this".
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// Fields carries per-field validation errors for forms.
	Fields map[string]string `json:"fields,omitempty"`
	err    error
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.err }

func BadRequest(msg string) *Error   { return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: msg} }
func Unauthorized(msg string) *Error { return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: msg} }
func Forbidden(msg string) *Error    { return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: msg} }
func NotFound(msg string) *Error     { return &Error{Status: http.StatusNotFound, Code: "not_found", Message: msg} }
func Conflict(msg string) *Error     { return &Error{Status: http.StatusConflict, Code: "conflict", Message: msg} }

func Invalid(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed",
		Message: "Some fields need attention.", Fields: fields}
}

// Internal wraps an unexpected error. The cause is logged, never returned.
func Internal(err error) *Error {
	return &Error{Status: http.StatusInternalServerError, Code: "internal_error",
		Message: "Something went wrong on our side.", err: err}
}

// Handler is a http.HandlerFunc that may fail. Returning an error is the only
// way a handler reports a problem, which keeps every response shape identical.
type Handler func(http.ResponseWriter, *http.Request) error

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err)
	}
}

func JSON(w http.ResponseWriter, status int, body any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return nil
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status line is already out; all we can do is leave a trail.
		slog.Error("encode response", "error", err)
	}
	return nil
}

func NoContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		apiErr = Internal(err)
	}
	if apiErr.Status >= 500 {
		slog.Error("request failed",
			"method", r.Method, "path", r.URL.Path, "error", apiErr.Error())
	}
	_ = JSON(w, apiErr.Status, map[string]any{"error": apiErr})
}

// DecodeJSON reads a JSON body with a size cap and rejects unknown fields, so a
// typo in a client payload fails loudly instead of being silently dropped.
func DecodeJSON(r *http.Request, dst any) error {
	const maxBody = 1 << 20 // 1 MiB
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return BadRequest("Request body is not valid JSON: " + err.Error())
	}
	if dec.More() {
		return BadRequest("Request body must contain a single JSON object.")
	}
	return nil
}

// QueryInt reads an integer query parameter, clamped into [min, max].
func QueryInt(r *http.Request, key string, fallback, min, max int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// ---------------------------------------------------------------- middleware

type Middleware func(http.Handler) http.Handler

func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status)
	})
}

// Recoverer turns a panic into a 500 instead of killing the process.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "path", r.URL.Path, "panic", rec)
				WriteError(w, r, Internal(fmt.Errorf("panic: %v", rec)))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS allows exactly the configured origins. An empty list disables CORS,
// which is the correct default when the API and the app share a domain.
func CORS(origins []string) Middleware {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if origin != "" && allowed[origin] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				h.Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets the handful that matter for a JSON API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
