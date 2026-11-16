package storage

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
)

func mustTestJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestMessageTreeIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "tree-integration@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "tree-integration@test.dev")
	}
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	convs := NewConversations(pool)
	msgs := NewMessages(pool)
	conv, err := convs.Create(ctx, user.ID, "tree", ConversationPatch{})
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}

	root, err := msgs.Add(ctx, Message{ConversationID: conv.ID, UserID: user.ID, Role: "user", Content: "root",
		Data: mustTestJSON(model.Message{Role: model.RoleUser, Content: "root"})})
	if err != nil {
		t.Fatalf("add root: %v", err)
	}
	child, err := msgs.Add(ctx, Message{ConversationID: conv.ID, UserID: user.ID, ParentID: root, Role: "assistant", Content: "child",
		Data: mustTestJSON(model.Message{Role: model.RoleAssistant, Content: "child"})})
	if err != nil {
		t.Fatalf("add child: %v", err)
	}
	active, err := msgs.Add(ctx, Message{ConversationID: conv.ID, UserID: user.ID, ParentID: root, Role: "assistant", Content: "active",
		Data: mustTestJSON(model.Message{Role: model.RoleAssistant, Content: "active"})})
	if err != nil {
		t.Fatalf("add active: %v", err)
	}
	if err := convs.SetActiveLeaf(ctx, conv.ID, user.ID, active); err != nil {
		t.Fatalf("set leaf: %v", err)
	}
	path, err := msgs.PathToLeaf(ctx, conv.ID, user.ID, active)
	if err != nil || len(path) != 2 || path[0].ID != root || path[1].ID != active {
		t.Fatalf("path to active leaf: %+v %v", path, err)
	}
	if latest, err := msgs.LatestRowID(ctx, conv.ID, user.ID); err != nil || latest != active {
		t.Fatalf("latest row = %q %v, want %q", latest, err, active)
	}

	// Deleting an inactive branch moves nothing.
	if err := msgs.DeleteBranch(ctx, conv.ID, child, user.ID); err != nil {
		t.Fatalf("delete inactive: %v", err)
	}
	if got, _ := convs.ByID(ctx, conv.ID, user.ID); got.ActiveLeafID == nil || *got.ActiveLeafID != active {
		t.Fatalf("inactive delete moved the leaf: %+v", got.ActiveLeafID)
	}
	if _, err := msgs.ByID(ctx, child, user.ID); err == nil {
		t.Fatal("inactive branch row still present")
	}

	// Deleting the active subtree falls back to the deleted root's parent.
	if err := msgs.DeleteBranch(ctx, conv.ID, active, user.ID); err != nil {
		t.Fatalf("delete active: %v", err)
	}
	if got, _ := convs.ByID(ctx, conv.ID, user.ID); got.ActiveLeafID == nil || *got.ActiveLeafID != root {
		t.Fatalf("active delete leaf = %+v, want %q", got.ActiveLeafID, root)
	}

	// Deleting the root clears the conversation.
	if err := msgs.DeleteBranch(ctx, conv.ID, root, user.ID); err != nil {
		t.Fatalf("delete root: %v", err)
	}
	if got, _ := convs.ByID(ctx, conv.ID, user.ID); got.ActiveLeafID != nil {
		t.Fatalf("root delete must clear the leaf: %+v", got.ActiveLeafID)
	}
	if rows, _ := msgs.ForConversation(ctx, conv.ID, user.ID); len(rows) != 0 {
		t.Fatalf("root delete left rows: %+v", rows)
	}

	// CopyPath mints fresh IDs, remaps the parent chain, and preserves
	// display fields including created_at.
	src, err := convs.Create(ctx, user.ID, "copy source", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := msgs.Add(ctx, Message{ConversationID: src.ID, UserID: user.ID, Role: "user", Content: "q", Data: mustTestJSON(model.Message{Role: model.RoleUser, Content: "q"})})
	b, _ := msgs.Add(ctx, Message{ConversationID: src.ID, UserID: user.ID, ParentID: a, Role: "assistant", Content: "a", Data: mustTestJSON(model.Message{Role: model.RoleAssistant, Content: "a"}), Model: "m", InputTokens: 2, OutputTokens: 3})
	srcPath, err := msgs.PathToLeaf(ctx, src.ID, user.ID, b)
	if err != nil || len(srcPath) != 2 {
		t.Fatalf("src path: %+v %v", srcPath, err)
	}
	dst, err := convs.Create(ctx, user.ID, "copy dest", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	tail, err := msgs.CopyPath(ctx, src.ID, user.ID, dst.ID, srcPath)
	if err != nil || tail == "" {
		t.Fatalf("copy path: %q %v", tail, err)
	}
	if tail == b {
		t.Fatal("copy reused the source tail ID")
	}
	copied, err := msgs.ForConversation(ctx, dst.ID, user.ID)
	if err != nil || len(copied) != 2 {
		t.Fatalf("copied rows: %+v %v", copied, err)
	}
	if copied[0].ID == a || copied[0].ParentID != "" || copied[1].ParentID != copied[0].ID {
		t.Fatalf("copied parent chain not remapped: %+v", copied)
	}
	if !copied[0].CreatedAt.Equal(srcPath[0].CreatedAt) || !copied[1].CreatedAt.Equal(srcPath[1].CreatedAt) {
		t.Fatalf("copy did not preserve created_at")
	}
	if copied[1].Model != "m" || copied[1].InputTokens != 2 || copied[1].OutputTokens != 3 {
		t.Fatalf("copy lost fields: %+v", copied[1])
	}

	// A corrupted parent loop terminates instead of recursing forever.
	cyc, err := convs.Create(ctx, user.ID, "cycle", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := msgs.Add(ctx, Message{ConversationID: cyc.ID, UserID: user.ID, Role: "user", Content: "1", Data: mustTestJSON(model.Message{Role: model.RoleUser, Content: "1"})})
	c2, _ := msgs.Add(ctx, Message{ConversationID: cyc.ID, UserID: user.ID, ParentID: c1, Role: "assistant", Content: "2", Data: mustTestJSON(model.Message{Role: model.RoleAssistant, Content: "2"})})
	if _, err := pool.Exec(ctx, `UPDATE messages SET parent_id = $1 WHERE id = $2`, c2, c1); err != nil {
		t.Fatalf("create cycle: %v", err)
	}
	loop, err := msgs.PathToLeaf(ctx, cyc.ID, user.ID, c1)
	if err != nil {
		t.Fatalf("cycle path error: %v", err)
	}
	if len(loop) != 2 {
		t.Fatalf("cycle path length = %d, want 2", len(loop))
	}
}

func TestPendingToolCallMessageIDIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "tree-pending@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "tree-pending@test.dev")
	}
	if err != nil {
		t.Fatal(err)
	}
	convs := NewConversations(pool)
	msgs := NewMessages(pool)
	pending := NewPendingCalls(pool)
	conv, err := convs.Create(ctx, user.ID, "pending", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	rowID, err := msgs.Add(ctx, Message{ConversationID: conv.ID, UserID: user.ID, Role: "assistant", Content: "",
		Data: mustTestJSON(model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call-x"}}})})
	if err != nil {
		t.Fatal(err)
	}
	if err := pending.Add(ctx, []PendingToolCall{{
		CallID: "call-x", MessageID: rowID, ConversationID: conv.ID, UserID: user.ID,
		ToolName: "t", Args: []byte(`{}`), Status: "pending",
	}}); err != nil {
		t.Fatal(err)
	}
	open, err := pending.ForConversation(ctx, conv.ID, user.ID)
	if err != nil || len(open) != 1 || open[0].MessageID != rowID {
		t.Fatalf("ForConversation message ID: %+v %v", open, err)
	}
	all, err := pending.ListAll(ctx, conv.ID, user.ID)
	if err != nil || len(all) != 1 || all[0].MessageID != rowID {
		t.Fatalf("ListAll message ID: %+v %v", all, err)
	}
	// An empty leaf clears the pointer.
	if err := convs.SetActiveLeaf(ctx, conv.ID, user.ID, rowID); err != nil {
		t.Fatal(err)
	}
	if err := convs.SetActiveLeaf(ctx, conv.ID, user.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := convs.ByID(ctx, conv.ID, user.ID); got.ActiveLeafID != nil {
		t.Fatalf("empty SetActiveLeaf did not clear the pointer: %v", got.ActiveLeafID)
	}
}
