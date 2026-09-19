package storage

import (
	"context"
	"testing"
)

func TestConversationSearch(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	convs := NewConversations(pool)
	user, err := NewUsers(pool).Create(ctx, "search@test.dev", "hash")
	if err != nil {
		t.Skipf("user setup: %v", err)
	}
	for _, title := range []string{"Moon landing plans", "Grocery list", "MOONSHOT ideas"} {
		if _, err := convs.Create(ctx, user.ID, title, ConversationPatch{}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	all, err := convs.List(ctx, user.ID, "")
	if err != nil || len(all) < 3 {
		t.Fatalf("unfiltered list: %+v %v", all, err)
	}
	moon, err := convs.List(ctx, user.ID, "moon")
	if err != nil || len(moon) != 2 {
		t.Fatalf("case-insensitive title search: %+v %v", moon, err)
	}
	none, err := convs.List(ctx, user.ID, "zzz-no-match")
	if err != nil || len(none) != 0 {
		t.Fatalf("empty search: %+v %v", none, err)
	}
	literal, err := convs.List(ctx, user.ID, "%")
	if err != nil || len(literal) != 0 {
		t.Fatalf("metacharacters must match literally: %+v %v", literal, err)
	}
	foreign, err := convs.List(ctx, "00000000-0000-0000-0000-000000000000", "moon")
	if err != nil || len(foreign) != 0 {
		t.Fatalf("cross-user leak: %+v %v", foreign, err)
	}
}

func TestNullByteSanitizationIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	convs := NewConversations(pool)
	msgs := NewMessages(pool)

	user, err := users.Create(ctx, "nullbyte@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "nullbyte@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}

	conv, err := convs.Create(ctx, user.ID, "Conv\x00Title", ConversationPatch{})
	if err != nil {
		t.Fatalf("create conv with null byte: %v", err)
	}
	if conv.Title != "ConvTitle" {
		t.Fatalf("expected title without null byte, got %q", conv.Title)
	}

	if err := convs.SetTitle(ctx, conv.ID, user.ID, "Updated\x00Title"); err != nil {
		t.Fatalf("set title with null byte: %v", err)
	}

	// Insert message containing null bytes in content and data
	msg := Message{
		ConversationID: conv.ID,
		UserID:         user.ID,
		Role:           "user",
		Content:        "Hello\x00World from PDF extraction!",
		Data:           []byte(`{"role":"user","content":"Hello\u0000World from PDF extraction!"}`),
		Model:          "test\x00model",
	}
	if err := msgs.Add(ctx, msg); err != nil {
		t.Fatalf("msgs.Add with null bytes failed: %v", err)
	}

	list, err := msgs.ForConversation(ctx, conv.ID, user.ID)
	if err != nil || len(list) == 0 {
		t.Fatalf("messages lookup: %v", err)
	}
	if list[0].Content != "HelloWorld from PDF extraction!" {
		t.Fatalf("expected sanitized content, got %q", list[0].Content)
	}
}
