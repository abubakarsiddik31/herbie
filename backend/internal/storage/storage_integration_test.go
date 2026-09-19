package storage

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func TestRefreshRotationPersistence(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "rotate@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = users.ByEmail(ctx, "rotate@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	rt := NewRefreshTokens(pool)
	_, oldHash, _ := auth.NewRefreshToken()
	if err := rt.Create(ctx, user.ID, oldHash, time.Now().Add(auth.RefreshTTL)); err != nil {
		t.Fatalf("create: %v", err)
	}
	newTok, newHash, _ := auth.NewRefreshToken()
	if err := rt.Rotate(ctx, oldHash, newHash, time.Now().Add(auth.RefreshTTL)); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := rt.Get(ctx, oldHash); err != nil {
		t.Fatalf("old hash unreadable: %v", err)
	}
	rec, err := rt.Get(ctx, oldHash)
	if err != nil || !rec.Revoked {
		t.Fatalf("old token should read back revoked: %+v %v", rec, err)
	}
	if rec2, err := rt.Get(ctx, auth.HashRefreshToken(newTok)); err != nil || rec2.Revoked {
		t.Fatalf("new token should be active: %+v %v", rec2, err)
	}
}

func TestSearchQueryTrackingIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "searchtrack@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = users.ByEmail(ctx, "searchtrack@test.dev")
	}
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	usage := NewUsage(pool)
	sq := SearchQuery{
		UserID:       user.ID,
		Query:        "latest news about AI",
		Kind:         "web_search",
		Provider:     "tavily",
		ResultsCount: 5,
		DurationMs:   250,
	}
	if err := usage.RecordSearch(ctx, sq); err != nil {
		t.Fatalf("record search query: %v", err)
	}

	sum, err := usage.Summary(ctx, user.ID, 7)
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	if sum.Searches.TotalQueries == 0 {
		t.Fatalf("expected at least 1 search query in summary, got 0")
	}
	if sum.Searches.WebQueries == 0 {
		t.Fatalf("expected at least 1 web query in summary, got 0")
	}
	if len(sum.Searches.Recent) == 0 || sum.Searches.Recent[0].Query != "latest news about AI" {
		t.Fatalf("expected recent query 'latest news about AI', got %+v", sum.Searches.Recent)
	}
}

