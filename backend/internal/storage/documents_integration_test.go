package storage

import (
	"context"
	"errors"
	"fmt"
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

func TestReadyIDsByProjectScopesAndFiltersStatus(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "docs-ids@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "docs-ids@test.dev")
	}
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	projects := NewProjects(pool)
	p1, err := projects.Create(ctx, Project{UserID: user.ID, Name: "P1"})
	if err != nil {
		t.Fatalf("create p1: %v", err)
	}
	p2, err := projects.Create(ctx, Project{UserID: user.ID, Name: "P2"})
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}

	repo := NewDocuments(pool)
	n := 0
	mk := func(projectID *string, status string) Document {
		n++
		d, err := repo.Create(ctx, Document{
			UserID: user.ID, ObjectKey: fmt.Sprintf("%s/%d.txt", user.ID, n),
			Filename: fmt.Sprintf("f-%d.txt", n), Mime: "text/plain", SizeBytes: 1,
			ProjectID: projectID,
		})
		if err != nil {
			t.Fatalf("create doc %d: %v", n, err)
		}
		if status != "processing" {
			if err := repo.SetStatus(ctx, d.ID, user.ID, status, "", 1); err != nil {
				t.Fatalf("set status %d: %v", n, err)
			}
		}
		return d
	}
	ready := mk(&p1.ID, "ready")
	mk(&p1.ID, "processing")
	mk(&p1.ID, "failed")
	mk(&p2.ID, "ready")
	mk(nil, "ready")

	ids, err := repo.ReadyIDsByProject(ctx, p1.ID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != ready.ID {
		t.Fatalf("p1 ready ids = %v, want [%s]", ids, ready.ID)
	}
	if other, err := repo.ReadyIDsByProject(ctx, p2.ID, user.ID); err != nil || len(other) != 1 {
		t.Fatalf("p2 ready ids = %v err=%v", other, err)
	}
	if none, err := repo.ReadyIDsByProject(ctx, p1.ID, "00000000-0000-0000-0000-000000000000"); err != nil || len(none) != 0 {
		t.Fatalf("cross-user ids = %v err=%v", none, err)
	}
}
