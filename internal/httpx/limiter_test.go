package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The fingerprint is a spam signal, and hashing means no raw IP is stored
// against a lead.
func TestClientFingerprint(t *testing.T) {
	base := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/public/enquiries", nil)
		r.RemoteAddr = "203.0.113.7:51514"
		r.Header.Set("User-Agent", "Mozilla/5.0")
		return r
	}

	a := ClientFingerprint(base())
	if len(a) != 32 {
		t.Errorf("fingerprint length = %d, want 32 hex chars", len(a))
	}
	if a == ClientFingerprint(func() *http.Request {
		r := base()
		r.Header.Set("User-Agent", "curl/8")
		return r
	}()) {
		t.Error("a different user agent should change the fingerprint")
	}
	if ClientFingerprint(base()) != a {
		t.Error("the same client should produce the same fingerprint")
	}

	// The raw address must not be recoverable from the stored value.
	if a == "203.0.113.7" || len(a) != 32 {
		t.Error("the fingerprint should be a hash, not the address")
	}
}

func TestClientFingerprintUsesForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")

	withHeader := ClientFingerprint(r)

	r2 := httptest.NewRequest(http.MethodPost, "/x", nil)
	r2.RemoteAddr = "10.0.0.1:1234"
	if withHeader == ClientFingerprint(r2) {
		t.Error("behind a proxy the forwarded client address should be used")
	}
}

func TestClientFingerprintWithoutPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.RemoteAddr = "not-a-host-port"
	if got := ClientFingerprint(r); len(got) != 32 {
		t.Errorf("an unparseable RemoteAddr should still hash, got %q", got)
	}
}

func TestLimiterAllowsThenBlocks(t *testing.T) {
	l := NewLimiter()
	const key = "client"

	for i := 0; i < 5; i++ {
		if !l.Allow(key, 5, time.Hour) {
			t.Fatalf("request %d should have been allowed", i+1)
		}
	}
	if l.Allow(key, 5, time.Hour) {
		t.Fatal("the sixth request within the window should be blocked")
	}

	// A different client is unaffected.
	if !l.Allow("other", 5, time.Hour) {
		t.Error("a different client should not inherit the block")
	}
}

func TestLimiterWindowExpires(t *testing.T) {
	l := NewLimiter()
	const key = "client"

	if !l.Allow(key, 1, time.Millisecond) {
		t.Fatal("the first request should be allowed")
	}
	if l.Allow(key, 1, time.Millisecond) {
		t.Fatal("the second request inside the window should be blocked")
	}

	time.Sleep(3 * time.Millisecond)
	if !l.Allow(key, 1, time.Millisecond) {
		t.Error("the window should have expired")
	}
}

// The limiter keeps state per client, so it must not grow without bound.
func TestLimiterSweepsStaleEntries(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < 4200; i++ {
		l.Allow(string(rune(i%1000))+string(rune(i/1000))+"-"+time.Now().String(), 5, time.Nanosecond)
	}
	if len(l.hits) > 4200 {
		t.Errorf("the limiter map grew to %d entries without sweeping", len(l.hits))
	}
}

// A correct password should clear the budget, so someone who has just proved
// who they are is not one typo from a lockout.
func TestForgetClearsAKey(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < 3; i++ {
		l.Allow("k", 3, time.Hour)
	}
	if l.Allow("k", 3, time.Hour) {
		t.Fatal("the budget should be spent")
	}
	l.Forget("k")
	if !l.Allow("k", 3, time.Hour) {
		t.Error("Forget did not clear the key")
	}
}

func TestTooManyRequests(t *testing.T) {
	err := TooManyRequests("slow down")
	if err.Status != http.StatusTooManyRequests || err.Code != "rate_limited" {
		t.Errorf("unexpected: %+v", err)
	}
}
