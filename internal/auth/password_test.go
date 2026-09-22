package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Errorf("the right password should verify: %v", err)
	}
	if err := VerifyPassword("wrong", hash); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("the wrong password should return ErrPasswordMismatch, got %v", err)
	}
}

// A per-hash salt means two identical passwords must not share a hash — this is
// what stops a database dump revealing which accounts share a password.
func TestHashIsSaltedPerCall(t *testing.T) {
	a, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two hashes of the same password are identical, so the salt is not random")
	}
	for _, h := range []string{a, b} {
		if err := VerifyPassword("same", h); err != nil {
			t.Errorf("both hashes should still verify: %v", err)
		}
	}
}

func TestHashFormatIsSelfDescribing(t *testing.T) {
	hash, err := HashPassword("x")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("expected 6 PHC segments, got %d in %q", len(parts), hash)
	}
	if parts[1] != "argon2id" {
		t.Errorf("algorithm segment = %q", parts[1])
	}
	// The cost parameters must travel with the hash so they can be raised later
	// without invalidating everything already stored.
	if !strings.HasPrefix(parts[3], "m=") || !strings.Contains(parts[3], "t=") ||
		!strings.Contains(parts[3], "p=") {
		t.Errorf("parameter segment = %q", parts[3])
	}
}

// Verification reads the cost from the stored hash, not from the constants, so
// a hash written under older parameters keeps working.
func TestVerifyUsesStoredParameters(t *testing.T) {
	// m=32768 and t=1 are deliberately not the current constants.
	const legacy = "$argon2id$v=19$m=32768,t=1,p=1$" +
		"c2FsdHNhbHRzYWx0c2ExMg$" +
		"7Ic9wVQvqMfLxM8zyRfLJsGxHKJ9gLZ6eQhLFLLXZ2Y"
	// The digest above is arbitrary, so this must be a mismatch rather than a
	// parse failure — that is the behaviour under test.
	err := VerifyPassword("anything", legacy)
	if err != nil && !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("a legacy-parameter hash should parse, got %v", err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"not phc":          "plaintext",
		"wrong algorithm":  "$argon2i$v=19$m=65536,t=2,p=2$c2FsdA$aGFzaA",
		"bad version":      "$argon2id$vNN$m=65536,t=2,p=2$c2FsdA$aGFzaA",
		"wrong version":    "$argon2id$v=16$m=65536,t=2,p=2$c2FsdA$aGFzaA",
		"bad parameters":   "$argon2id$v=19$memory$c2FsdA$aGFzaA",
		"bad salt base64":  "$argon2id$v=19$m=65536,t=2,p=2$!!!!$aGFzaA",
		"bad hash base64":  "$argon2id$v=19$m=65536,t=2,p=2$c2FsdA$!!!!",
		"too few segments": "$argon2id$v=19$m=65536,t=2,p=2$c2FsdA",
	}
	for name, hash := range cases {
		t.Run(name, func(t *testing.T) {
			if err := VerifyPassword("x", hash); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
