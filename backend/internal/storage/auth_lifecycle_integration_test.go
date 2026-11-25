package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/google/uuid"
)

func TestAuthTokenConsumeAndResetTransaction(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	email := fmt.Sprintf("lifecycle-%d@test.dev", time.Now().UnixNano())
	user, err := users.Create(ctx, email, "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	tokens := NewAuthTokens(pool)
	_, verifyHash, _ := auth.NewRefreshToken()
	if err := tokens.Create(ctx, user.ID, verifyHash, auth.PurposeVerifyEmail, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.ConsumeVerify(ctx, verifyHash, time.Now()); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := tokens.ConsumeVerify(ctx, verifyHash, time.Now()); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("verify reuse = %v", err)
	}
	got, err := users.ByID(ctx, user.ID)
	if err != nil || !got.Verified() {
		t.Fatalf("user not verified: %+v %v", got, err)
	}

	// A live session must die with the reset, and the new password hash
	// must be committed together with the token spend.
	rt := NewRefreshTokens(pool)
	sessionID := uuid.NewString()
	_, sessionHash, _ := auth.NewRefreshToken()
	if err := rt.Create(ctx, user.ID, sessionID, sessionHash, time.Now().Add(time.Hour), auth.SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	newHash, err := auth.HashPassword("longenough1")
	if err != nil {
		t.Fatal(err)
	}
	_, resetHash, _ := auth.NewRefreshToken()
	if err := tokens.Create(ctx, user.ID, resetHash, auth.PurposeResetPassword, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.ConsumeReset(ctx, resetHash, newHash, time.Now()); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := tokens.ConsumeReset(ctx, resetHash, newHash, time.Now()); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("reset reuse = %v", err)
	}
	got, _ = users.ByID(ctx, user.ID)
	if err := auth.CheckPassword(got.PasswordHash, "longenough1"); err != nil {
		t.Fatalf("password not replaced: %v", err)
	}
	rec, err := rt.Get(ctx, sessionHash)
	if err != nil || !rec.Revoked {
		t.Fatalf("session not revoked by reset: %+v %v", rec, err)
	}
}

func TestRefreshSessionsListAndRevoke(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	email := fmt.Sprintf("sessions-%d@test.dev", time.Now().UnixNano())
	user, err := users.Create(ctx, email, "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	other, err := users.Create(ctx, fmt.Sprintf("other-%d@test.dev", time.Now().UnixNano()), "hash")
	if err != nil {
		t.Fatalf("seed other: %v", err)
	}

	rt := NewRefreshTokens(pool)
	sid1, sid2 := uuid.NewString(), uuid.NewString()
	_, h1, _ := auth.NewRefreshToken()
	_, h2, _ := auth.NewRefreshToken()
	if err := rt.Create(ctx, user.ID, sid1, h1, time.Now().Add(time.Hour), auth.SessionMeta{UserAgent: "A", IP: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Create(ctx, user.ID, sid2, h2, time.Now().Add(time.Hour), auth.SessionMeta{UserAgent: "B", IP: "2.2.2.2"}); err != nil {
		t.Fatal(err)
	}

	sessions, err := rt.ListSessions(ctx, user.ID)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions = %+v err=%v", sessions, err)
	}
	found := map[string]auth.Session{}
	for _, s := range sessions {
		found[s.ID] = s
	}
	if found[sid1].UserAgent != "A" || found[sid2].IP != "2.2.2.2" {
		t.Fatalf("metadata: %+v", sessions)
	}

	if err := rt.RevokeSession(ctx, other.ID, sid1); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("cross-user revoke = %v", err)
	}
	if err := rt.RevokeSession(ctx, user.ID, sid1); err != nil {
		t.Fatal(err)
	}
	if err := rt.RevokeSession(ctx, user.ID, sid1); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("double revoke = %v", err)
	}
	sessions, _ = rt.ListSessions(ctx, user.ID)
	if len(sessions) != 1 || sessions[0].ID != sid2 {
		t.Fatalf("after revoke: %+v", sessions)
	}
	if err := rt.RevokeAll(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	sessions, _ = rt.ListSessions(ctx, user.ID)
	if len(sessions) != 0 {
		t.Fatalf("after revoke all: %+v", sessions)
	}
}
