package storage

import (
	"context"
	"errors"
	"testing"
)

func TestSavedPromptsIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "prompts@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "prompts@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	other, err := users.Create(ctx, "prompts-other@test.dev", "hash")
	if err != nil {
		other, err = users.ByEmail(ctx, "prompts-other@test.dev")
	}
	if err != nil {
		t.Fatalf("other user setup: %v", err)
	}
	// Keep the test re-runnable against the shared integration database.
	if _, err := pool.Exec(ctx, `DELETE FROM saved_prompts WHERE user_id IN ($1, $2)`, user.ID, other.ID); err != nil {
		t.Fatalf("clean prompts: %v", err)
	}
	store := NewSavedPrompts(pool)

	created, err := store.Create(ctx, user.ID, "Standup", "Summarize yesterday")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Title != "Standup" || created.Body != "Summarize yesterday" {
		t.Fatalf("created = %+v", created)
	}
	second, err := store.Create(ctx, user.ID, "Second", "Body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, other.ID, "Theirs", "Secret"); err != nil {
		t.Fatal(err)
	}

	list, err := store.List(ctx, user.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v %v", list, err)
	}
	n, err := store.Count(ctx, user.ID)
	if err != nil || n != 2 {
		t.Fatalf("count = %d %v", n, err)
	}

	newTitle := "Daily"
	newBody := "Recap"
	updated, err := store.Update(ctx, created.ID, user.ID, &newTitle, &newBody)
	if err != nil || updated.Title != "Daily" || updated.Body != "Recap" {
		t.Fatalf("update = %+v %v", updated, err)
	}
	if _, err := store.Update(ctx, created.ID, other.ID, &newTitle, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign update = %v", err)
	}
	if err := store.Delete(ctx, second.ID, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign delete = %v", err)
	}
	if err := store.Delete(ctx, second.ID, user.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, _ := store.Count(ctx, user.ID); n != 1 {
		t.Fatalf("count after delete = %d", n)
	}
}
