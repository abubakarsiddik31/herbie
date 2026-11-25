package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAccountExportIncludesOwnedDataAndOmitsSecrets(t *testing.T) {
	pool, _ := newTempTestDatabase(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := NewUsers(pool)
	hash, _ := auth.HashPassword("longenough1")
	user, err := users.Create(ctx, "export@test.dev", hash)
	if err != nil {
		t.Fatal(err)
	}
	projects := NewProjects(pool)
	project, err := projects.Create(ctx, Project{UserID: user.ID, Name: "P"})
	if err != nil {
		t.Fatal(err)
	}
	convs := NewConversations(pool)
	conv, err := convs.Create(ctx, user.ID, "Chat", ConversationPatch{ProjectID: &project.ID})
	if err != nil {
		t.Fatal(err)
	}
	msgs := NewMessages(pool)
	if _, err := msgs.Add(ctx, Message{
		ConversationID: conv.ID, UserID: user.ID, Role: "user", Content: "hello",
		Data: json.RawMessage(`{"role":"user","content":"hello"}`),
	}); err != nil {
		t.Fatal(err)
	}
	docs := NewDocuments(pool)
	doc, err := docs.Create(ctx, Document{
		UserID: user.ID, ProjectID: &project.ID, ObjectKey: user.ID + "/doc/a.txt",
		Filename: "a.txt", Mime: "text/plain", SizeBytes: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMemories(pool).Create(ctx, user.ID, "remember this"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSavedPrompts(pool).Create(ctx, user.ID, "p", "body", 100); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	files, err := NewAccounts(pool).WriteExport(ctx, user.ID, &buf)
	if err != nil {
		t.Fatal(err)
	}
	raw := buf.String()
	for _, forbidden := range []string{"password_hash", "search_vector", "encrypted_key", "token_hash"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("export leaked %q", forbidden)
		}
	}
	var bundle map[string]any
	if err := json.Unmarshal(buf.Bytes(), &bundle); err != nil {
		t.Fatalf("invalid export json: %v", err)
	}
	for _, key := range []string{"profile", "conversations", "messages", "projects", "documents", "memories", "savedPrompts"} {
		if _, ok := bundle[key]; !ok {
			t.Fatalf("missing %q in %v", key, keys(bundle))
		}
	}
	if len(files) != 1 || files[0].DocumentID != doc.ID || files[0].ObjectKey != doc.ObjectKey {
		t.Fatalf("files = %+v", files)
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestAccountDeleteCascadesAndProtectsLastAdmin(t *testing.T) {
	pool, _ := newTempTestDatabase(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := NewUsers(pool)
	hash, _ := auth.HashPassword("longenough1")
	user, err := users.Create(ctx, "delete@test.dev", hash)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := NewConversations(pool).Create(ctx, user.ID, "Chat", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := NewDocuments(pool).Create(ctx, Document{
		UserID: user.ID, ObjectKey: user.ID + "/doc/a.txt", Filename: "a.txt",
		Mime: "text/plain", SizeBytes: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	acct := NewAccounts(pool)
	if err := acct.Delete(ctx, user.ID, "wrong-password"); !errors.Is(err, auth.ErrInvalidPassword) {
		t.Fatalf("wrong password = %v", err)
	}
	if err := acct.Delete(ctx, user.ID, "longenough1"); err != nil {
		t.Fatal(err)
	}
	if _, err := users.ByID(ctx, user.ID); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("user survived: %v", err)
	}
	if _, err := NewConversations(pool).ByID(ctx, conv.ID, user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("conversation survived: %v", err)
	}
	if _, err := NewDocuments(pool).Get(ctx, doc.ID, user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("document survived: %v", err)
	}
}

func TestAccountDeleteLastAdmin(t *testing.T) {
	pool, _ := newTempTestDatabase(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("longenough1")
	admin, err := NewUsers(pool).Create(ctx, "admin@test.dev", hash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := NewAccounts(pool).Delete(ctx, admin.ID, "longenough1"); !errors.Is(err, auth.ErrLastAdmin) {
		t.Fatalf("last admin = %v", err)
	}
	if _, err := NewUsers(pool).ByID(ctx, admin.ID); err != nil {
		t.Fatalf("last admin was deleted: %v", err)
	}
}
