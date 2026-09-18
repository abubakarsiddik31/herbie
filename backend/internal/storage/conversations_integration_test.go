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
