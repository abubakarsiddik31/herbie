package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMessageClientMessageIDUniquePerConversation(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, fmt.Sprintf("idem-%d@test.dev", time.Now().UnixNano()), "hash")
	if err != nil {
		t.Fatal(err)
	}
	convs := NewConversations(pool)
	conv1, err := convs.Create(ctx, user.ID, "one", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	conv2, err := convs.Create(ctx, user.ID, "two", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	msgs := NewMessages(pool)
	msg := Message{
		ConversationID: conv1.ID, UserID: user.ID, Role: "user", ClientMessageID: "client-key",
		Content: "hi", Data: []byte(`{"role":"user","content":"hi"}`),
	}
	id, err := msgs.Add(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Add(ctx, msg); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate = %v", err)
	}
	found, err := msgs.ByClientMessageID(ctx, conv1.ID, user.ID, "client-key")
	if err != nil || found.ID != id || found.ClientMessageID != "client-key" {
		t.Fatalf("by client id: %+v %v", found, err)
	}
	// The same key is allowed in another conversation.
	msg.ConversationID = conv2.ID
	if _, err := msgs.Add(ctx, msg); err != nil {
		t.Fatalf("other conversation: %v", err)
	}
}
