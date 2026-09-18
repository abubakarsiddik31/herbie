package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

type failRotateStore struct {
	*authtest.FakeRefreshStore
}

func (s *failRotateStore) Rotate(_ context.Context, _, _ string, _ time.Time) error {
	return auth.ErrInvalidRefresh
}

func TestRefreshRotationConflictRevokesFamily(t *testing.T) {
	ctx := context.Background()
	users := authtest.NewFakeUsers()
	fakeRefresh := authtest.NewFakeRefreshStore()
	store := &failRotateStore{FakeRefreshStore: fakeRefresh}
	svc, err := auth.NewService(users, store, authtest.NewFakeOAuthStore(users), "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}

	reg, err := svc.Register(ctx, "race@b.co", "longenough1")
	if err != nil {
		t.Fatal(err)
	}

	// Rotate returning ErrInvalidRefresh represents concurrent rotation
	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh on rotate conflict, got %v", err)
	}
	// Verify family was revoked
	rec, err := fakeRefresh.Get(ctx, auth.HashRefreshToken(reg.RefreshToken))
	if err != nil || !rec.Revoked {
		t.Fatalf("family should have been revoked on conflict: %+v %v", rec, err)
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

func TestOAuthLoginProvisionLinkAndRelogin(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	first, err := svc.OAuthLogin(ctx, "google", "sub-1", "o@x.co")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if first.User.Email != "o@x.co" || first.AccessToken == "" {
		t.Fatalf("bad provision result: %+v", first.User)
	}
	if _, err := svc.Login(ctx, "o@x.co", "anything"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("password login on oauth-only account should fail, got %v", err)
	}

	second, err := svc.OAuthLogin(ctx, "google", "sub-1", "o@x.co")
	if err != nil {
		t.Fatalf("relogin: %v", err)
	}
	if second.User.ID != first.User.ID {
		t.Fatal("relogin resolved a different user")
	}

	if _, err := svc.Register(ctx, "p@x.co", "longenough1"); err != nil {
		t.Fatalf("register: %v", err)
	}
	linked, err := svc.OAuthLogin(ctx, "github", "gh-9", "p@x.co")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	pw, err := svc.Login(ctx, "p@x.co", "longenough1")
	if err != nil || pw.User.ID != linked.User.ID {
		t.Fatalf("password login after link: %v", err)
	}
}

func TestOAuthCodeSingleUse(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	res, err := svc.OAuthLogin(ctx, "google", "sub-7", "c@x.co")
	if err != nil {
		t.Fatal(err)
	}
	code, err := svc.IssueOAuthCode(ctx, res.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConsumeOAuthCode(ctx, code); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if _, err := svc.ConsumeOAuthCode(ctx, code); !errors.Is(err, auth.ErrInvalidOAuthCode) {
		t.Fatalf("expected single use, got %v", err)
	}
	if _, err := svc.ConsumeOAuthCode(ctx, "bogus"); !errors.Is(err, auth.ErrInvalidOAuthCode) {
		t.Fatalf("expected ErrInvalidOAuthCode, got %v", err)
	}
}
