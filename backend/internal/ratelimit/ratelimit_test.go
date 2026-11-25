package ratelimit

import (
	"testing"
	"time"
)

func TestFixedWindowAllow(t *testing.T) {
	l := New(2, time.Minute)
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("ip", "ip|email"); !ok {
			t.Fatalf("attempt %d should pass", i+1)
		}
	}
	ok, retry := l.Allow("ip", "ip|email")
	if ok || retry <= 0 {
		t.Fatalf("third attempt = %v retry=%v", ok, retry)
	}
	if ok, _ := l.Allow("other-ip"); !ok {
		t.Fatal("another key must not be limited")
	}
}

func TestDisabledLimiterAllowsEverything(t *testing.T) {
	l := New(0, time.Minute)
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatal("disabled limiter must allow")
		}
	}
}
