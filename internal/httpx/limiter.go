package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter is a fixed-window counter keyed on whatever the caller chooses.
//
// It lives here rather than in one feature package because two very different
// things need it: the unauthenticated enquiry form, and login. It is per
// instance and deliberately so — it is spam and brute-force friction, not a
// security boundary, and pretending otherwise would invite leaning on it.
type Limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func NewLimiter() *Limiter { return &Limiter{hits: map[string][]time.Time{}} }

// Allow records an attempt and reports whether it is within the budget.
func (l *Limiter) Allow(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-window)
	kept := make([]time.Time, 0, len(l.hits[key]))
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, time.Now())

	// Opportunistic sweep so the map cannot grow without bound.
	if len(l.hits) > 4096 {
		for k, v := range l.hits {
			if len(v) == 0 || v[len(v)-1].Before(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

// Forget clears a key, so a successful login does not leave the caller one
// failed attempt away from a lockout they have already disproved.
func (l *Limiter) Forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

// ClientFingerprint hashes the caller's address and user agent. It is a
// throttling key only, and hashing means no raw address is stored anywhere.
func ClientFingerprint(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if comma := strings.IndexByte(ip, ','); comma > 0 {
		ip = ip[:comma]
	}
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		} else {
			ip = r.RemoteAddr
		}
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip) + "|" + r.UserAgent()))
	return hex.EncodeToString(sum[:16])
}

// TooManyRequests is the response for a caller who has run out of budget.
func TooManyRequests(msg string) *Error {
	return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: msg}
}
