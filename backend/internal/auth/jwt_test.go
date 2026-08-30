package auth

import (
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	tm, err := NewTokenMaker("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewTokenMaker: %v", err)
	}
	now := time.Now()
	token, exp, err := tm.Issue("u-1", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !exp.After(now.Add(14 * time.Minute)) {
		t.Fatalf("exp too early: %v", exp)
	}
	got, err := tm.Verify(token)
	if err != nil || got != "u-1" {
		t.Fatalf("Verify = %q, %v", got, err)
	}
}

func TestTokenSecretTooShort(t *testing.T) {
	if _, err := NewTokenMaker("short"); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	tm, _ := NewTokenMaker("0123456789abcdef0123456789abcdef")
	token, _, _ := tm.Issue("u-1", time.Now().Add(-16*time.Minute))
	if _, err := tm.Verify(token); err == nil {
		t.Fatal("expected expired token to fail")
	}
}
