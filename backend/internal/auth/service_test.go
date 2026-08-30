package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeUsers struct {
	byEmail map[string]UserRecord
	created []UserRecord
}

func (f *fakeUsers) Create(_ context.Context, email, hash string) (UserRecord, error) {
	if _, ok := f.byEmail[email]; ok {
		return UserRecord{}, ErrEmailTaken
	}
	rec := UserRecord{ID: "u-" + email, Email: email, PasswordHash: hash}
	f.byEmail[email] = rec
	f.created = append(f.created, rec)
	return rec, nil
}

func (f *fakeUsers) ByEmail(_ context.Context, email string) (UserRecord, error) {
	rec, ok := f.byEmail[email]
	if !ok {
		return UserRecord{}, ErrInvalidCredentials
	}
	return rec, nil
}

func (f *fakeUsers) ByID(_ context.Context, id string) (UserRecord, error) {
	for _, rec := range f.created {
		if rec.ID == id {
			return rec, nil
		}
	}
	return UserRecord{}, ErrInvalidRefresh
}

type fakeRefresh struct {
	tokens map[string]RefreshRecord
}

func (f *fakeRefresh) Create(_ context.Context, userID, hash string, exp time.Time) error {
	f.tokens[hash] = RefreshRecord{UserID: userID, ExpiresAt: exp}
	return nil
}
func (f *fakeRefresh) Get(_ context.Context, hash string) (RefreshRecord, error) {
	rec, ok := f.tokens[hash]
	if !ok {
		return RefreshRecord{}, ErrInvalidRefresh
	}
	return rec, nil
}
func (f *fakeRefresh) Rotate(_ context.Context, oldHash, newHash string, exp time.Time) error {
	old := f.tokens[oldHash]
	old.Revoked = true
	f.tokens[oldHash] = old
	f.tokens[newHash] = RefreshRecord{UserID: old.UserID, ExpiresAt: exp}
	return nil
}
func (f *fakeRefresh) RevokeFamily(_ context.Context, _ string) error {
	for h, rec := range f.tokens {
		rec.Revoked = true
		f.tokens[h] = rec
	}
	return nil
}

func newTestService() (*Service, *fakeRefresh) {
	users := &fakeUsers{byEmail: map[string]UserRecord{}}
	refresh := &fakeRefresh{tokens: map[string]RefreshRecord{}}
	svc, err := NewService(users, refresh, "0123456789abcdef0123456789abcdef")
	if err != nil {
		panic(err)
	}
	return svc, refresh
}

func TestRegisterLoginFlow(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	res, err := svc.Register(ctx, "a@b.co", "longenough1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Fatal("expected both tokens")
	}
	if _, err := svc.Register(ctx, "a@b.co", "longenough1"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
	if _, err := svc.Login(ctx, "a@b.co", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if _, err := svc.Login(ctx, "a@b.co", "longenough1"); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	reg, err := svc.Register(ctx, "a@b.co", "longenough1")
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Refresh(ctx, reg.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Presenting the rotated-out token again is reuse: the whole family dies.
	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh on reuse, got %v", err)
	}
	if _, err := svc.Refresh(ctx, second.RefreshToken); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("family should be revoked after reuse, got %v", err)
	}
}

func TestWeakPasswordRejected(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Register(context.Background(), "a@b.co", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}

// TestWrongAlgTokenRejected is the Task 5 review follow-up: prove an
// alg=none forgery is rejected at the TokenMaker.Verify primitive level.
func TestWrongAlgTokenRejected(t *testing.T) {
	tm, err := NewTokenMaker("0123456789abcdef0123456789abcdef")
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
