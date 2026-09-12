package storage

import (
	"context"
	"errors"
	"testing"
)

func TestDocumentsCRUD(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "docs-crud@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "docs-crud@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	other, err := users.Create(ctx, "docs-crud-other@test.dev", "hash")
	if err != nil {
		other, err = users.ByEmail(ctx, "docs-crud-other@test.dev")
	}
	if err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	repo := NewDocuments(pool)
	d, err := repo.Create(ctx, Document{
		UserID: user.ID, ObjectKey: user.ID + "/d1/a.txt", Filename: "a.txt",
		Mime: "text/plain", SizeBytes: 12,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.Status != "processing" || d.ChunkCount != 0 {
		t.Fatalf("defaults not applied: %+v", d)
	}

	got, err := repo.Get(ctx, d.ID, user.ID)
	if err != nil || got.Filename != "a.txt" {
		t.Fatalf("get: %v %+v", err, got)
	}

	if err := repo.SetStatus(ctx, d.ID, user.ID, "ready", "", 7); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ = repo.Get(ctx, d.ID, user.ID)
	if got.Status != "ready" || got.ChunkCount != 7 || got.Error != "" {
		t.Fatalf("ready row: %+v", got)
	}

	if err := repo.SetStatus(ctx, d.ID, user.ID, "failed", "boom", 0); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	got, _ = repo.Get(ctx, d.ID, user.ID)
	if got.Status != "failed" || got.Error != "boom" {
		t.Fatalf("failed row: %+v", got)
	}

	list, err := repo.List(ctx, user.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", err, list)
	}

	if cross, _ := repo.List(ctx, other.ID); len(cross) != 0 {
		t.Fatalf("cross-user leak: %v", cross)
	}
	if _, err := repo.Get(ctx, d.ID, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get = %v, want ErrNotFound", err)
	}

	if err := repo.Delete(ctx, d.ID, user.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, d.ID, user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-delete get = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, d.ID, user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete = %v, want ErrNotFound", err)
	}
}
