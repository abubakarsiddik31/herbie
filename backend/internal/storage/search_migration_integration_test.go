package storage

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestChatSearchMigrationBackfillAndDown(t *testing.T) {
	pool, dsn := newTempTestDatabase(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	db := openGoose(t, dsn)
	defer db.Close()

	// Roll back to just before the search migration, then insert a row with
	// no search vector to exercise the backfill.
	const beforeSearch = int64(20261116210000)
	if err := goose.DownToContext(ctx, db, "migrations", beforeSearch); err != nil {
		t.Fatalf("down to pre-search: %v", err)
	}

	var userID, convID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ('migrate-search@test.dev', 'hash') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title) VALUES ($1, 'legacy search') RETURNING id`, userID).Scan(&convID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO messages (conversation_id, user_id, role, content, data)
		 VALUES ($1, $2, 'user', 'Legacy deployment notes', '{"role":"user","content":"Legacy deployment notes"}'::jsonb)`,
		convID, userID); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		t.Fatalf("up search migration: %v", err)
	}
	var matched bool
	if err := pool.QueryRow(ctx,
		`SELECT search_vector @@ to_tsquery('simple', 'deploy:*') FROM messages WHERE conversation_id = $1`,
		convID).Scan(&matched); err != nil {
		t.Fatalf("read backfilled vector: %v", err)
	}
	if !matched {
		t.Fatal("backfill did not index existing content")
	}

	// New rows are indexed by the trigger.
	if _, err := pool.Exec(ctx,
		`INSERT INTO messages (conversation_id, user_id, role, content, data)
		 VALUES ($1, $2, 'assistant', 'Fresh indexing works', '{"role":"assistant","content":"Fresh indexing works"}'::jsonb)`,
		convID, userID); err != nil {
		t.Fatalf("insert triggered message: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM messages WHERE conversation_id = $1
		   AND search_vector @@ to_tsquery('simple', 'index:*')`, convID).Scan(new(int)); err != nil {
		t.Fatalf("triggered search: %v", err)
	}

	if err := goose.DownToContext(ctx, db, "migrations", beforeSearch); err != nil {
		t.Fatalf("down search migration: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_name = 'messages' AND column_name = 'search_vector'`).Scan(&n); err != nil {
		t.Fatalf("check column: %v", err)
	}
	if n != 0 {
		t.Fatal("down left search_vector behind")
	}
}
