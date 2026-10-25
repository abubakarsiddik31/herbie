package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

func TestBotFilesLifecycle(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user, err := NewUsers(pool).Create(ctx, "bot-files@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = NewUsers(pool).ByEmail(ctx, "bot-files@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	other, err := NewUsers(pool).Create(ctx, "bot-files-other@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		other, err = NewUsers(pool).ByEmail(ctx, "bot-files-other@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("other user setup: %v", err)
	}
	bot, err := NewBots(pool).Create(ctx, Bot{UserID: user.ID, Name: "Herbie"})
	if err != nil {
		t.Fatalf("bot setup: %v", err)
	}
	otherBot, err := NewBots(pool).Create(ctx, Bot{UserID: other.ID, Name: "Bolt"})
	if err != nil {
		t.Fatalf("other bot setup: %v", err)
	}

	store := NewBotFiles(pool)

	// Upsert writes a file and returns server-assigned fields.
	first, err := store.Upsert(ctx, BotFile{
		BotID: bot.ID, UserID: user.ID, Path: "notes/todo.md", Content: "ship the thing",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if first.ID == "" || first.SizeBytes != int64(len("ship the thing")) {
		t.Fatalf("upsert defaults: %+v", first)
	}
	if first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", first)
	}

	// Get round-trips content, size and path.
	got, err := store.Get(ctx, user.ID, bot.ID, "notes/todo.md")
	if err != nil || got.ID != first.ID || got.Content != "ship the thing" ||
		got.SizeBytes != int64(len("ship the thing")) || got.Path != "notes/todo.md" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := store.Get(ctx, user.ID, bot.ID, "notes/missing.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: want ErrNotFound, got %v", err)
	}

	// Upserting the same path overwrites in place: still one row, same id and
	// created_at, new content/size and a refreshed updated_at.
	again, err := store.Upsert(ctx, BotFile{
		BotID: bot.ID, UserID: user.ID, Path: "notes/todo.md", Content: "shipped it",
	})
	if err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	if again.ID != first.ID || again.Content != "shipped it" ||
		again.SizeBytes != int64(len("shipped it")) || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("overwrite did not keep identity: first=%+v again=%+v", first, again)
	}
	if !again.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("updated_at not refreshed: first=%v again=%v", first.UpdatedAt, again.UpdatedAt)
	}
	n, err := store.Count(ctx, bot.ID)
	if err != nil || n != 1 {
		t.Fatalf("count after overwrite: %v %d", err, n)
	}

	// List returns metadata ordered by path WITHOUT content.
	second, err := store.Upsert(ctx, BotFile{
		BotID: bot.ID, UserID: user.ID, Path: "notes/ideas.md", Content: "idea one",
	})
	if err != nil {
		t.Fatalf("upsert second: %v", err)
	}
	list, err := store.List(ctx, user.ID, bot.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %+v", err, list)
	}
	if list[0].Path != "notes/ideas.md" || list[1].Path != "notes/todo.md" {
		t.Fatalf("list not ordered by path: %v %v", list[0].Path, list[1].Path)
	}
	for _, file := range list {
		if file.Content != "" {
			t.Fatalf("list must not include content: %+v", file)
		}
		if file.ID == second.ID && file.SizeBytes != int64(len("idea one")) {
			t.Fatalf("list metadata wrong: %+v", file)
		}
	}

	// Cross-bot isolation: the same path in another bot's namespace is a
	// separate row, and that bot's user cannot see the first bot's files.
	cross, err := store.Upsert(ctx, BotFile{
		BotID: otherBot.ID, UserID: other.ID, Path: "notes/todo.md", Content: "other bot's file",
	})
	if err != nil {
		t.Fatalf("upsert cross-bot: %v", err)
	}
	if cross.ID == first.ID {
		t.Fatalf("cross-bot upsert collided with first bot's row")
	}
	if _, err := store.Get(ctx, other.ID, bot.ID, "notes/todo.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user read first bot's file: want ErrNotFound, got %v", err)
	}
	if files, err := store.List(ctx, other.ID, bot.ID); err != nil || len(files) != 0 {
		t.Fatalf("other user list first bot's files: %v %+v", err, files)
	}
	n, err = store.Count(ctx, otherBot.ID)
	if err != nil || n != 1 {
		t.Fatalf("count other bot: %v %d", err, n)
	}

	// Delete removes one file; the rest survive; a second delete maps to
	// ErrNotFound.
	if err := store.Delete(ctx, user.ID, bot.ID, "notes/ideas.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, user.ID, bot.ID, "notes/ideas.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.Delete(ctx, user.ID, bot.ID, "notes/ideas.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
	n, err = store.Count(ctx, bot.ID)
	if err != nil || n != 1 {
		t.Fatalf("count after delete: %v %d", err, n)
	}

	// Cascade cleanup: deleting the bots wipes their files
	// (bot_files.bot_id ON DELETE CASCADE), so re-running the test starts
	// from clean namespaces.
	if err := NewBots(pool).Delete(ctx, user.ID, bot.ID); err != nil {
		t.Fatalf("bot cleanup: %v", err)
	}
	if err := NewBots(pool).Delete(ctx, other.ID, otherBot.ID); err != nil {
		t.Fatalf("other bot cleanup: %v", err)
	}
}
