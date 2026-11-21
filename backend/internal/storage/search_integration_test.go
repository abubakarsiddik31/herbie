package storage

import (
	"context"
	"strings"
	"testing"
)

func TestMessageSearchIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "chat-search@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "chat-search@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	other, err := users.Create(ctx, "chat-search-other@test.dev", "hash")
	if err != nil {
		other, err = users.ByEmail(ctx, "chat-search-other@test.dev")
	}
	if err != nil {
		t.Fatalf("other user setup: %v", err)
	}
	// Re-runnable: clearing conversations cascades their messages.
	for _, id := range []string{user.ID, other.ID} {
		if _, err := pool.Exec(ctx, `DELETE FROM conversations WHERE user_id = $1`, id); err != nil {
			t.Fatalf("clean conversations: %v", err)
		}
	}

	convs := NewConversations(pool)
	msgs := NewMessages(pool)
	search := NewMessageSearch(pool)

	conv, err := convs.Create(ctx, user.ID, "Deploy notes", ConversationPatch{})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	archived, err := convs.Create(ctx, user.ID, "Archived notes", ConversationPatch{})
	if err != nil {
		t.Fatalf("create archived conversation: %v", err)
	}
	if err := convs.SetArchived(ctx, archived.ID, user.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	foreign, err := convs.Create(ctx, other.ID, "Other user", ConversationPatch{})
	if err != nil {
		t.Fatalf("create foreign conversation: %v", err)
	}

	add := func(convID, userID, role, content string) string {
		t.Helper()
		id, err := msgs.Add(ctx, Message{
			ConversationID: convID, UserID: userID, Role: role,
			Content: content, Data: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("add %s message: %v", role, err)
		}
		return id
	}
	add(conv.ID, user.ID, "user", "Postgres replication lag troubleshooting guide")
	add(conv.ID, user.ID, "assistant", "Check wal_keep_size and replication slots")
	add(conv.ID, user.ID, "tool", "replication internal tool output")
	add(archived.ID, user.ID, "user", "replication in the archive")
	add(foreign.ID, other.ID, "user", "replication for someone else")

	hits, hasMore, err := search.Search(ctx, user.ID, "replic", 20, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 || hasMore {
		t.Fatalf("prefix hits = %d hasMore=%v", len(hits), hasMore)
	}
	for _, h := range hits {
		if !strings.Contains(h.Snippet, SnippetHitStart) || !strings.Contains(h.Snippet, SnippetHitStop) {
			t.Fatalf("snippet missing sentinels: %q", h.Snippet)
		}
		if h.ConversationID != conv.ID {
			t.Fatalf("archived/foreign conversation leaked: %+v", h)
		}
	}

	// Prefix matching on a compound term and operator escaping.
	if hits, _, err := search.Search(ctx, user.ID, "wal_keep", 10, 0); err != nil || len(hits) != 1 {
		t.Fatalf("wal_keep hits = %d err=%v", len(hits), err)
	}
	if hits, _, err := search.Search(ctx, user.ID, "replic & | ! ( )", 10, 0); err != nil || len(hits) != 2 {
		t.Fatalf("escaped query hits = %d err=%v", len(hits), err)
	}

	// Pagination: three user-owned visible rows share the term if we add one.
	add(conv.ID, user.ID, "assistant", "Replication lag dashboard")
	hits, hasMore, err = search.Search(ctx, user.ID, "replication", 2, 0)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(hits) != 2 || !hasMore {
		t.Fatalf("page 1 = %d hasMore=%v", len(hits), hasMore)
	}
	second, hasMore, err := search.Search(ctx, user.ID, "replication", 2, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(second) != 1 || hasMore {
		t.Fatalf("page 2 = %d hasMore=%v", len(second), hasMore)
	}

	if hits, _, err := search.Search(ctx, other.ID, "replication", 10, 0); err != nil || len(hits) != 1 {
		t.Fatalf("other user hits = %d err=%v", len(hits), err)
	}
	if hits, _, err := search.Search(ctx, user.ID, "&&&", 10, 0); err != nil || len(hits) != 0 {
		t.Fatalf("operator-only query hits = %d err=%v", len(hits), err)
	}
}
