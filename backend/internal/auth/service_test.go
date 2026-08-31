package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/golang-jwt/jwt/v5"
)

func newTestService() *auth.Service {
	return authtest.NewService("0123456789abcdef0123456789abcdef")
}

func TestRegisterLoginFlow(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	res, err := svc.Register(ctx, "a@b.co", "longenough1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Fatal("expected both tokens")
	}
	if _, err := svc.Register(ctx, "a@b.co", "longenough1"); !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
	if _, err := svc.Login(ctx, "a@b.co", "wrong"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if _, err := svc.Login(ctx, "a@b.co", "longenough1"); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	reg, err := svc.Register(ctx, "a@b.co", "longenough1")
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Refresh(ctx, reg.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Presenting the rotated-out token again is reuse: the whole family dies.
	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh on reuse, got %v", err)
	}
	if _, err := svc.Refresh(ctx, second.RefreshToken); !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("family should be revoked after reuse, got %v", err)
	}
}

func TestWeakPasswordRejected(t *testing.T) {
	svc := newTestService()
	if _, err := svc.Register(context.Background(), "a@b.co", "short"); !errors.Is(err, auth.ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}

// TestWrongAlgTokenRejected is the Task 5 review follow-up: prove an
// alg=none forgery is rejected at the TokenMaker.Verify primitive level.
func TestWrongAlgTokenRejected(t *testing.T) {
	tm, err := auth.NewTokenMaker("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	forged := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "u-1"})
	signed, err := forged.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tm.Verify(signed); err == nil {
		t.Fatal("alg=none forgery must be rejected")
	}
}
