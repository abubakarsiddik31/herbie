package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// newTempTestDatabase creates an isolated database so migration up/down tests
// never disturb the shared integration database. It skips when the configured
// role cannot create databases.
func newTempTestDatabase(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	admin, err := Open(ctx, base)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	t.Cleanup(admin.Close)

	name := fmt.Sprintf("herbie_branch_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, name)); err != nil {
		t.Skipf("cannot create temporary database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, name))
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open temp database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, dsn
}

func openGoose(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open goose db: %v", err)
	}
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMessageBranchesMigrationBackfillAndDown(t *testing.T) {
	pool, dsn := newTempTestDatabase(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	db := openGoose(t, dsn)
	defer db.Close()

	// Roll back to just before the branch migration, then populate the
	// pre-tree schema with controlled IDs and created_at ties.
	const beforeBranches = int64(20261025130000)
	if err := goose.DownToContext(ctx, db, "migrations", beforeBranches); err != nil {
		t.Fatalf("down to pre-branches: %v", err)
	}

	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ('migrate-branches@test.dev', 'hash') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var conv1, conv2, conv3 string
	if err := pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title) VALUES ($1, 'legacy') RETURNING id`, userID).Scan(&conv1); err != nil {
		t.Fatalf("insert conv1: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title) VALUES ($1, 'paused') RETURNING id`, userID).Scan(&conv2); err != nil {
		t.Fatalf("insert conv2: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title) VALUES ($1, 'empty') RETURNING id`, userID).Scan(&conv3); err != nil {
		t.Fatalf("insert conv3: %v", err)
	}

	insertMsg := func(id, conv, role, content, data, created string, truncated bool) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO messages (id, conversation_id, user_id, role, content, data, created_at, truncated)
			 VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::timestamptz,$8)`,
			id, conv, userID, role, content, data, created, truncated); err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}
	// Forced ties: m2 and m2b share created_at; (created_at, id) decides.
	m1 := "00000000-0000-0000-0000-000000000001"
	m2 := "00000000-0000-0000-0000-000000000002"
	m2b := "00000000-0000-0000-0000-000000000020"
	m3 := "00000000-0000-0000-0000-000000000003"
	m4 := "00000000-0000-0000-0000-000000000004"
	insertMsg(m1, conv1, "user", "one", `{"role":"user","content":"one"}`, "2026-01-01T00:00:10Z", false)
	insertMsg(m2, conv1, "assistant", "", `{"role":"assistant","toolCalls":[{"id":"call-1","name":"t"}]}`, "2026-01-01T00:00:20Z", false)
	insertMsg(m2b, conv1, "assistant", "tie", `{"role":"assistant","content":"tie"}`, "2026-01-01T00:00:20Z", true)
	insertMsg(m3, conv1, "tool", "ok", `{"role":"tool","toolCallId":"call-1","content":"ok"}`, "2026-01-01T00:00:30Z", false)
	insertMsg(m4, conv1, "assistant", "four", `{"role":"assistant","content":"four"}`, "2026-01-01T00:00:40Z", false)
	n1 := "00000000-0000-0000-0000-000000000101"
	n2 := "00000000-0000-0000-0000-000000000102"
	insertMsg(n1, conv2, "user", "q", `{"role":"user","content":"q"}`, "2026-01-01T00:01:00Z", false)
	insertMsg(n2, conv2, "assistant", "", `{"role":"assistant","toolCalls":[{"id":"call-2","name":"t"}]}`, "2026-01-01T00:01:10Z", false)
	if _, err := pool.Exec(ctx,
		`INSERT INTO pending_tool_calls (call_id, conversation_id, user_id, tool_name, args, status)
		 VALUES ('call-1',$1,$2,'t','{}'::jsonb,'pending'),
		        ('call-2',$3,$2,'t','{}'::jsonb,'pending'),
		        ('call-3',$1,$2,'t','{}'::jsonb,'approved'),
		        ('call-4',$4,$2,'t','{}'::jsonb,'pending')`,
		conv1, userID, conv2, conv3); err != nil {
		t.Fatalf("insert pending calls: %v", err)
	}

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		t.Fatalf("up branches: %v", err)
	}

	// The backfill chains rows in (created_at, id) order and picks the last
	// row as the active leaf.
	rows, err := pool.Query(ctx,
		`SELECT id::text, COALESCE(parent_id::text, '') FROM messages
		 WHERE conversation_id = $1 ORDER BY created_at, id`, conv1)
	if err != nil {
		t.Fatal(err)
	}
	var ids, parents []string
	for rows.Next() {
		var id, parent string
		if err := rows.Scan(&id, &parent); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		parents = append(parents, parent)
	}
	rows.Close()
	want := []string{m1, m2, m2b, m3, m4}
	if len(ids) != len(want) {
		t.Fatalf("backfilled rows = %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("backfill order = %v, want %v", ids, want)
		}
		wantParent := ""
		if i > 0 {
			wantParent = want[i-1]
		}
		if parents[i] != wantParent {
			t.Fatalf("row %s parent = %q, want %q", ids[i], parents[i], wantParent)
		}
	}
	var leaf1, leaf2 string
	if err := pool.QueryRow(ctx, `SELECT active_leaf_id::text FROM conversations WHERE id = $1`, conv1).Scan(&leaf1); err != nil || leaf1 != m4 {
		t.Fatalf("conv1 leaf = %q %v, want %s", leaf1, err, m4)
	}
	if err := pool.QueryRow(ctx, `SELECT active_leaf_id::text FROM conversations WHERE id = $1`, conv2).Scan(&leaf2); err != nil || leaf2 != n2 {
		t.Fatalf("conv2 leaf = %q %v, want %s", leaf2, err, n2)
	}
	// Parked approvals anchor at the conversation's paused tail (its active
	// leaf), not the assistant call row: mixed batches persist executed tool
	// results after the call row and the resume must replay them.
	var pendingMsg, pendingMsg2 string
	if err := pool.QueryRow(ctx, `SELECT message_id::text FROM pending_tool_calls WHERE call_id = 'call-1'`).Scan(&pendingMsg); err != nil || pendingMsg != m4 {
		t.Fatalf("parked call-1 message = %q %v, want tail %s", pendingMsg, err, m4)
	}
	if err := pool.QueryRow(ctx, `SELECT message_id::text FROM pending_tool_calls WHERE call_id = 'call-2'`).Scan(&pendingMsg2); err != nil || pendingMsg2 != n2 {
		t.Fatalf("parked call-2 message = %q %v, want tail %s", pendingMsg2, err, n2)
	}
	// Resolved calls are not resume anchors, and a conversation with no rows
	// has no tail to anchor at: both stay NULL.
	var approvedMsg, emptyMsg sql.NullString
	if err := pool.QueryRow(ctx, `SELECT message_id::text FROM pending_tool_calls WHERE call_id = 'call-3'`).Scan(&approvedMsg); err != nil || approvedMsg.Valid {
		t.Fatalf("resolved call-3 message = %v %v, want NULL", approvedMsg, err)
	}
	if err := pool.QueryRow(ctx, `SELECT message_id::text FROM pending_tool_calls WHERE call_id = 'call-4'`).Scan(&emptyMsg); err != nil || emptyMsg.Valid {
		t.Fatalf("empty-conversation call-4 message = %v %v, want NULL", emptyMsg, err)
	}
	var truncated bool
	if err := pool.QueryRow(ctx, `SELECT truncated FROM messages WHERE id = $1`, m2b).Scan(&truncated); err != nil || !truncated {
		t.Fatalf("truncated tail lost: %v %v", truncated, err)
	}

	// An off-path branch added after the backfill must be pruned by Down;
	// the active path survives as a valid linear conversation.
	offPath := "00000000-0000-0000-0000-000000000099"
	if _, err := pool.Exec(ctx,
		`INSERT INTO messages (id, conversation_id, user_id, parent_id, role, content, data)
		 VALUES ($1,$2,$3,$4,'assistant','offpath','{"role":"assistant","content":"offpath"}'::jsonb)`,
		offPath, conv1, userID, m2); err != nil {
		t.Fatalf("insert off-path branch: %v", err)
	}
	if err := goose.DownToContext(ctx, db, "migrations", beforeBranches); err != nil {
		t.Fatalf("down branches: %v", err)
	}
	var gone int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE id = $1`, offPath).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("off-path row survived Down: %d %v", gone, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'messages' AND column_name = 'parent_id'`).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("messages.parent_id survived Down: %d %v", gone, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'conversations' AND column_name = 'active_leaf_id'`).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("conversations.active_leaf_id survived Down: %d %v", gone, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'pending_tool_calls' AND column_name = 'message_id'`).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("pending_tool_calls.message_id survived Down: %d %v", gone, err)
	}
	var kept int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE conversation_id = $1`, conv1).Scan(&kept); err != nil || kept != 5 {
		t.Fatalf("Down kept %d active-path rows, want 5: %v", kept, err)
	}

	// Re-running Up on the now-linear data backfills a clean chain again.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate up after down: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT active_leaf_id::text FROM conversations WHERE id = $1`, conv1).Scan(&leaf1); err != nil || leaf1 != m4 {
		t.Fatalf("leaf after re-Up = %q %v", leaf1, err)
	}
}
