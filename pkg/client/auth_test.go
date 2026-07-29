package client

import (
	"testing"
	"time"
)

// Expected values cross-checked against an independent Python implementation
// (base64.b32decode + hmac/hashlib, same RFC 6238 30s-step SHA1 algorithm)
// for the well-known example secret "JBSWY3DPEHPK3PXP", not just recomputed
// from this same Go code.
func TestGenerateTOTPAt(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	tests := []struct {
		unixTime int64
		want     string
	}{
		{59, "996554"},
		{1111111109, "071271"},
		{20000000000, "752434"},
	}
	for _, tt := range tests {
		got, err := generateTOTPAt(secret, time.Unix(tt.unixTime, 0).UTC())
		if err != nil {
			t.Fatalf("unixTime=%d: unexpected error: %v", tt.unixTime, err)
		}
		if got != tt.want {
			t.Errorf("unixTime=%d: got %q, want %q", tt.unixTime, got, tt.want)
		}
	}
}

func TestGenerateTOTPAt_InvalidSecret(t *testing.T) {
	if _, err := generateTOTPAt("not-valid-base32!!", time.Now()); err == nil {
		t.Fatal("expected an error for an invalid base32 secret, got nil")
	}
}

func TestGenerateTOTP_SameStepIsStable(t *testing.T) {
	// Two calls within the same 30s window must return the same code -
	// this is what makes it safe to call GenerateTOTP once per login
	// attempt rather than per HTTP request.
	a, err := GenerateTOTP("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := GenerateTOTP("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != b {
		t.Errorf("got two different codes %q and %q for back-to-back calls", a, b)
	}
}
