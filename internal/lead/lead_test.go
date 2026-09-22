package lead

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlausiblePhone(t *testing.T) {
	good := []string{
		"9822041190", "+91 98220 41190", "+919822041190",
		"98220-41190", "91 7020533449", "6123456789",
	}
	for _, p := range good {
		if !plausiblePhone(p) {
			t.Errorf("%q should be accepted", p)
		}
	}

	bad := []string{
		"", "12345", "98220411901234",
		"5123456789",  // Indian mobiles do not start below 6
		"0822041190",  // leading zero
		"abcdefghij",
		"+1 415 555 0132", // ten digits but not an Indian mobile prefix
	}
	for _, p := range bad {
		if plausiblePhone(p) {
			t.Errorf("%q should be rejected", p)
		}
	}
}

// The fingerprint is a spam signal, and hashing means no raw IP is stored
// against a lead.
func TestClientFingerprint(t *testing.T) {
	base := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/public/enquiries", nil)
		r.RemoteAddr = "203.0.113.7:51514"
		r.Header.Set("User-Agent", "Mozilla/5.0")
		return r
	}

	a := clientFingerprint(base())
	if len(a) != 32 {
		t.Errorf("fingerprint length = %d, want 32 hex chars", len(a))
	}
	if a == clientFingerprint(func() *http.Request {
		r := base()
		r.Header.Set("User-Agent", "curl/8")
		return r
	}()) {
		t.Error("a different user agent should change the fingerprint")
	}
	if clientFingerprint(base()) != a {
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

	withHeader := clientFingerprint(r)

	r2 := httptest.NewRequest(http.MethodPost, "/x", nil)
	r2.RemoteAddr = "10.0.0.1:1234"
	if withHeader == clientFingerprint(r2) {
		t.Error("behind a proxy the forwarded client address should be used")
	}
}

func TestClientFingerprintWithoutPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.RemoteAddr = "not-a-host-port"
	if got := clientFingerprint(r); len(got) != 32 {
		t.Errorf("an unparseable RemoteAddr should still hash, got %q", got)
	}
}

func TestLimiterAllowsThenBlocks(t *testing.T) {
	l := newLimiter()
	const key = "client"

	for i := 0; i < 5; i++ {
		if !l.allow(key, 5, time.Hour) {
			t.Fatalf("request %d should have been allowed", i+1)
		}
	}
	if l.allow(key, 5, time.Hour) {
		t.Fatal("the sixth request within the window should be blocked")
	}

	// A different client is unaffected.
	if !l.allow("other", 5, time.Hour) {
		t.Error("a different client should not inherit the block")
	}
}

func TestLimiterWindowExpires(t *testing.T) {
	l := newLimiter()
	const key = "client"

	if !l.allow(key, 1, time.Millisecond) {
		t.Fatal("the first request should be allowed")
	}
	if l.allow(key, 1, time.Millisecond) {
		t.Fatal("the second request inside the window should be blocked")
	}

	time.Sleep(3 * time.Millisecond)
	if !l.allow(key, 1, time.Millisecond) {
		t.Error("the window should have expired")
	}
}

// The limiter keeps state per client, so it must not grow without bound.
func TestLimiterSweepsStaleEntries(t *testing.T) {
	l := newLimiter()
	for i := 0; i < 4200; i++ {
		l.allow(string(rune(i%1000))+string(rune(i/1000))+"-"+time.Now().String(), 5, time.Nanosecond)
	}
	if len(l.hits) > 4200 {
		t.Errorf("the limiter map grew to %d entries without sweeping", len(l.hits))
	}
}

func TestValidEnquiryStatus(t *testing.T) {
	for _, s := range []string{"new", "contacted", "visit_scheduled", "converted", "lost"} {
		if !validEnquiryStatus(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "NEW", "won", "closed"} {
		if validEnquiryStatus(s) {
			t.Errorf("%q should not be valid", s)
		}
	}
}
