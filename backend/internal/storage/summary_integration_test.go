package storage

import (
	"context"
	"errors"
	"testing"
)

func TestConversationSummaryPersistence(t *testing.T) {
	pool, _ := newTempTestDatabase(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := NewUsers(pool)
	user, err := users.Create(ctx, "summary@test.dev", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.Create(ctx, "summary-other@test.dev", "hash")
	if err != nil {
		t.Fatal(err)
	}
	conv, err := NewConversations(pool).Create(ctx, user.ID, "Chat", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	convs := NewConversations(pool)

	sum, err := convs.Summary(ctx, conv.ID, user.ID)
	if err != nil || sum.Text != "" || sum.Covers != 0 {
		t.Fatalf("initial summary: %+v %v", sum, err)
	}
	if err := convs.SetSummary(ctx, conv.ID, user.ID, "decisions", 3); err != nil {
		t.Fatal(err)
	}
	sum, err = convs.Summary(ctx, conv.ID, user.ID)
	if err != nil || sum.Text != "decisions" || sum.Covers != 3 || sum.UpdatedAt == nil {
		t.Fatalf("stored summary: %+v %v", sum, err)
	}
	if err := convs.SetSummary(ctx, conv.ID, other.ID, "hijack", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user set = %v", err)
	}
	if err := convs.ClearSummary(ctx, conv.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	sum, _ = convs.Summary(ctx, conv.ID, user.ID)
	if sum.Text != "" || sum.Covers != 0 || sum.UpdatedAt != nil {
		t.Fatalf("cleared summary: %+v", sum)
	}
}
