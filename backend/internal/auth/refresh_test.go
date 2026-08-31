package auth

import (
	"strings"
	"testing"
)

func TestNewRefreshToken(t *testing.T) {
	a, ha, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	b, hb, _ := NewRefreshToken()
	if a == b || ha == hb {
		t.Fatal("tokens must be unique")
	}
	if strings.ContainsAny(a, "+/=") {
		t.Fatalf("token must be URL-safe: %q", a)
	}
	if HashRefreshToken(a) != ha {
		t.Fatal("hash mismatch")
	}
}
