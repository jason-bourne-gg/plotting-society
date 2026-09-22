package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decode(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestErrorConstructors(t *testing.T) {
	cases := []struct {
		err    *Error
		status int
		code   string
	}{
		{BadRequest("x"), http.StatusBadRequest, "bad_request"},
		{Unauthorized("x"), http.StatusUnauthorized, "unauthorized"},
		{Forbidden("x"), http.StatusForbidden, "forbidden"},
		{NotFound("x"), http.StatusNotFound, "not_found"},
		{Conflict("x"), http.StatusConflict, "conflict"},
		{Invalid(map[string]string{"a": "b"}), http.StatusUnprocessableEntity, "validation_failed"},
		{Internal(errors.New("boom")), http.StatusInternalServerError, "internal_error"},
	}
	for _, c := range cases {
		if c.err.Status != c.status {
			t.Errorf("%s: status = %d, want %d", c.code, c.err.Status, c.status)
		}
		if c.err.Code != c.code {
			t.Errorf("code = %q, want %q", c.err.Code, c.code)
		}
	}
}

// The cause of a 500 must be reachable by the logger and absent from the body.
func TestInternalWrapsButDoesNotLeak(t *testing.T) {
	cause := errors.New("pq: password authentication failed for user")
	err := Internal(cause)

	if !errors.Is(err, cause) {
		t.Error("Internal should wrap the cause so errors.Is finds it")
	}
	if !strings.Contains(err.Error(), "password authentication failed") {
		t.Error("Error() should carry the cause for logging")
	}

	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), err)

	if body := rec.Body.String(); strings.Contains(body, "password authentication") {
		t.Errorf("the response body leaked the cause: %s", body)
	}
}

func TestErrorWithoutCause(t *testing.T) {
	err := BadRequest("just this")
	if err.Error() != "just this" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Unwrap() != nil {
		t.Error("Unwrap should be nil when there is no cause")
	}
}

func TestHandlerServeHTTPWritesErrors(t *testing.T) {
	h := Handler(func(http.ResponseWriter, *http.Request) error {
		return Forbidden("nope")
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	body := decode(t, rec.Body)
	errObj, _ := body["error"].(map[string]any)
	if errObj["message"] != "nope" {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestHandlerServeHTTPSuccessIsUntouched(t *testing.T) {
	h := Handler(func(w http.ResponseWriter, _ *http.Request) error {
		return JSON(w, http.StatusTeapot, map[string]string{"ok": "yes"})
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decode(t, rec.Body)["ok"]; got != "yes" {
		t.Errorf("body = %v", got)
	}
}

// A non-API error must never reach the client as anything but a generic 500.
func TestWriteErrorWithPlainError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), errors.New("raw"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	errObj := decode(t, rec.Body)["error"].(map[string]any)
	if errObj["code"] != "internal_error" {
		t.Errorf("code = %v", errObj["code"])
	}
	if strings.Contains(rec.Body.String(), "raw") {
		t.Error("the raw error leaked into the body")
	}
}

func TestJSONNilBody(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := JSON(rec, http.StatusAccepted, nil); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected an empty body, got %q", rec.Body.String())
	}
}

// An unencodable value cannot change the status line, which is already sent.
func TestJSONEncodeFailureKeepsStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := JSON(rec, http.StatusOK, map[string]any{"c": make(chan int)}); err != nil {
		t.Fatalf("JSON should not return the encode error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestNoContent(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := NoContent(rec); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("valid", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"a"}`))
		var p payload
		if err := DecodeJSON(r, &p); err != nil {
			t.Fatal(err)
		}
		if p.Name != "a" {
			t.Errorf("name = %q", p.Name)
		}
	})

	// A client typo must fail loudly rather than being silently dropped.
	t.Run("unknown field", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"nmae":"a"}`))
		var p payload
		err := DecodeJSON(r, &p)
		if err == nil {
			t.Fatal("expected an error for an unknown field")
		}
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
			t.Errorf("expected a 400, got %v", err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{`))
		var p payload
		if err := DecodeJSON(r, &p); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("trailing content", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"a"}{"name":"b"}`))
		var p payload
		err := DecodeJSON(r, &p)
		if err == nil || !strings.Contains(err.Error(), "single JSON object") {
			t.Fatalf("expected a single-object error, got %v", err)
		}
	})

	t.Run("oversized body", func(t *testing.T) {
		big := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(big))
		var p payload
		if err := DecodeJSON(r, &p); err == nil {
			t.Fatal("expected the size cap to reject this")
		}
	})
}

func TestQueryInt(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"", 50},          // absent -> fallback
		{"?limit=10", 10}, // in range
		{"?limit=0", 1},   // below min -> clamped
		{"?limit=999", 100}, // above max -> clamped
		{"?limit=abc", 50},  // unparseable -> fallback
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/x"+c.query, nil)
		if got := QueryInt(r, "limit", 50, 1, 100); got != c.want {
			t.Errorf("QueryInt(%q) = %d, want %d", c.query, got, c.want)
		}
	}
}

func TestChainRunsInOrder(t *testing.T) {
	var order []string
	mw := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	final := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	})

	Chain(final, mw("first"), mw("second")).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	want := []string{"first", "second", "handler"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// A panic in one request must not take the process down with it.
func TestRecoverer(t *testing.T) {
	h := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "kaboom") {
		t.Error("the panic value leaked into the response")
	}
}

func TestRecovererPassesThrough(t *testing.T) {
	h := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequestLogger(t *testing.T) {
	h := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestCORS(t *testing.T) {
	allowed := "https://app.example.in"
	h := CORS([]string{allowed + "/"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("allowed origin", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.Header.Set("Origin", allowed)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowed {
			t.Errorf("allow-origin = %q, want %q", got, allowed)
		}
		if rec.Header().Get("Vary") != "Origin" {
			t.Error("Vary: Origin is required or a cache will serve the wrong headers")
		}
	})

	// The important case: an origin nobody configured gets no CORS headers.
	t.Run("unknown origin", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.Header.Set("Origin", "https://evil.example.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("allow-origin = %q, want empty", got)
		}
	})

	t.Run("no origin header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("preflight short-circuits", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/x", nil)
		r.Header.Set("Origin", allowed)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("preflight status = %d, want 204", rec.Code)
		}
	})
}
