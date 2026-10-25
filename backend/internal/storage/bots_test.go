package storage

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

func TestBotsCRUD(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user, err := NewUsers(pool).Create(ctx, "bots-crud@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = NewUsers(pool).ByEmail(ctx, "bots-crud@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}

	store := NewBots(pool)
	before, err := store.Count(ctx, user.ID)
	if err != nil {
		t.Fatalf("count before: %v", err)
	}
	created, err := store.Create(ctx, Bot{UserID: user.ID, Name: "Herbie", Instructions: "research assistant"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Emoji != "🤖" || created.Color != "sage" || !created.RagEnabled {
		t.Fatalf("defaults not applied: %+v", created)
	}
	after, err := store.Count(ctx, user.ID)
	if err != nil || after != before+1 {
		t.Fatalf("count after create: %d (before %d): %v", after, before, err)
	}

	got, err := store.Get(ctx, user.ID, created.ID)
	if err != nil || got.Name != "Herbie" || got.Instructions != "research assistant" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := store.Get(ctx, user.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	newName := "Dot"
	archived := true
	upd, err := store.Update(ctx, user.ID, created.ID, BotPatch{Name: &newName, IsArchived: &archived})
	if err != nil || upd.Name != "Dot" || !upd.IsArchived {
		t.Fatalf("update: %v %+v", err, upd)
	}
	temp := 0.7
	if upd, err = store.Update(ctx, user.ID, created.ID, BotPatch{Temperature: &temp}); err != nil || upd.Temperature == nil || math.Abs(*upd.Temperature-temp) > 1e-6 {
		t.Fatalf("set temperature: %v %+v", err, upd)
	}
	if upd, err = store.Update(ctx, user.ID, created.ID, BotPatch{ClearTemperature: true}); err != nil || upd.Temperature != nil {
		t.Fatalf("clear temperature: %v %+v", err, upd)
	}

	// Archived bots are excluded from List and Count.
	list, err := store.List(ctx, user.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, b := range list {
		if b.ID == created.ID {
			t.Fatalf("archived bot must not be listed")
		}
	}
	if n, err := store.Count(ctx, user.ID); err != nil || n != before {
		t.Fatalf("archived bot must not be counted: %d (before %d): %v", n, before, err)
	}

	if _, err := store.Update(ctx, user.ID, "00000000-0000-0000-0000-000000000000", BotPatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	if err := store.Delete(ctx, user.ID, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.Delete(ctx, user.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

func TestBotMemoriesCRUD(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user, err := NewUsers(pool).Create(ctx, "bot-memories@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = NewUsers(pool).ByEmail(ctx, "bot-memories@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	bot, err := NewBots(pool).Create(ctx, Bot{UserID: user.ID, Name: "Herbie"})
	if err != nil {
		t.Fatalf("bot setup: %v", err)
	}

	store := NewBotMemories(pool)
	first, err := store.Add(ctx, BotMemory{BotID: bot.ID, UserID: user.ID, Content: "likes concise answers"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if first.ID == "" || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("add must return generated fields: %+v", first)
	}
	second, err := store.Add(ctx, BotMemory{BotID: bot.ID, UserID: user.ID, Content: "prefers rust"})
	if err != nil {
		t.Fatalf("add second: %v", err)
	}

	list, err := store.ListRecent(ctx, user.ID, bot.ID, 10)
	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID || list[0].Content != "prefers rust" {
		t.Fatalf("list recent (newest first): %+v %v", list, err)
	}
	limited, err := store.ListRecent(ctx, user.ID, bot.ID, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != second.ID {
		t.Fatalf("limit: %+v %v", limited, err)
	}
	foreignUser := "00000000-0000-0000-0000-000000000000"
	if list, err = store.ListRecent(ctx, foreignUser, bot.ID, 10); err != nil || len(list) != 0 {
		t.Fatalf("cross-user leak: %+v %v", list, err)
	}

	if err := store.Delete(ctx, foreignUser, bot.ID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign delete: want ErrNotFound, got %v", err)
	}
	if err := store.Delete(ctx, user.ID, bot.ID, first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := store.Delete(ctx, user.ID, bot.ID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}
