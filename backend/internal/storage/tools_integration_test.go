package storage

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

// testUser creates (or reuses) a user for FK targets, keeping tests re-runnable.
func testUser(t *testing.T, ctx context.Context, email string) string {
	t.Helper()
	users := NewUsers(newTestPool(t))
	user, err := users.Create(ctx, email, "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = users.ByEmail(ctx, email)
	}
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user.ID
}

// jsonEqual compares two JSON documents value-wise: jsonb normalizes
// whitespace and key order, so raw bytes never match after a round-trip.
func jsonEqual(a, b []byte) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

func TestToolsCRUDPersistence(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	userID := testUser(t, ctx, "tools-crud@test.dev")
	if _, err := pool.Exec(ctx, `DELETE FROM tools WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	tools := NewTools(pool)
	in := UserTool{
		UserID:          userID,
		Name:            "get_weather",
		Description:     "Get current weather",
		Method:          "GET",
		URLTemplate:     "https://api.example.com/forecast",
		Params:          json.RawMessage(`[{"name":"lat","in":"query","type":"number","required":true,"description":"latitude"}]`),
		Headers:         json.RawMessage(`{"Authorization":"Bearer secret"}`),
		RequireApproval: false,
		Enabled:         true,
	}
	out, err := tools.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.ID == "" || out.Name != "get_weather" || out.CreatedAt.IsZero() {
		t.Fatalf("create returned incomplete row: %+v", out)
	}
	if !jsonEqual(in.Params, out.Params) || !jsonEqual(in.Headers, out.Headers) {
		t.Fatalf("jsonb columns did not round-trip: %s %s", out.Params, out.Headers)
	}

	if _, err := tools.Create(ctx, in); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate create err = %v, want ErrDuplicate", err)
	}

	got, err := tools.ByID(ctx, out.ID, userID)
	if err != nil || got.ID != out.ID || got.Headers == nil {
		t.Fatalf("by id: %+v %v", got, err)
	}
	if _, err := tools.ByID(ctx, out.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong owner should be ErrNotFound, got %v", err)
	}

	list, err := tools.List(ctx, userID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %d tools, err %v", len(list), err)
	}
	enabled, err := tools.ListEnabled(ctx, userID)
	if err != nil || len(enabled) != 1 {
		t.Fatalf("list enabled: %d tools, err %v", len(enabled), err)
	}

	out.Enabled = false
	out.RequireApproval = true
	updated, err := tools.Update(ctx, out)
	if err != nil || updated.Enabled || !updated.RequireApproval {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if enabled, _ := tools.ListEnabled(ctx, userID); len(enabled) != 0 {
		t.Fatalf("disabled tool still listed as enabled")
	}

	if err := tools.Delete(ctx, out.ID, userID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := tools.ByID(ctx, out.ID, userID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-delete read err = %v", err)
	}
	if err := tools.Delete(ctx, out.ID, userID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete err = %v", err)
	}
}

func TestPendingCallsPersistence(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	userID := testUser(t, ctx, "tools-pending@test.dev")
	if _, err := pool.Exec(ctx, `DELETE FROM pending_tool_calls WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	conv, err := NewConversations(pool).Create(ctx, userID, "t")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	pending := NewPendingCalls(pool)
	err = pending.Add(ctx, []PendingToolCall{
		{CallID: "call-a", ConversationID: conv.ID, UserID: userID, ToolName: "search_hn", Args: []byte(`{"query":"golem"}`), Reason: "user approval required"},
		{CallID: "call-b", ConversationID: conv.ID, UserID: userID, ToolName: "search_hn", Args: []byte(`{}`), Reason: "user approval required"},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	got, err := pending.ForConversation(ctx, conv.ID, userID)
	if err != nil || len(got) != 2 {
		t.Fatalf("for conversation: %d calls, err %v", len(got), err)
	}
	if got[0].CallID != "call-a" || got[0].Status != "pending" || len(got[0].Args) == 0 {
		t.Fatalf("unexpected first call: %+v", got[0])
	}

	if err := pending.SetStatus(ctx, userID, "call-a", "approved"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	if remaining, _ := pending.ForConversation(ctx, conv.ID, userID); len(remaining) != 1 || remaining[0].CallID != "call-b" {
		t.Fatalf("after approval remaining = %+v", remaining)
	}
	if err := pending.SetStatus(ctx, userID, "call-zzz", "approved"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown call err = %v", err)
	}
	if err := pending.SetStatus(ctx, "00000000-0000-0000-0000-000000000000", "call-b", "denied"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong owner err = %v", err)
	}

	// Provider-synthesized call IDs repeat across pauses: re-pausing with a
	// call ID that was already resolved must insert a fresh row.
	err = pending.Add(ctx, []PendingToolCall{
		{CallID: "call-a", ConversationID: conv.ID, UserID: userID, ToolName: "search_hn", Args: []byte(`{}`), Status: "pending"},
	})
	if err != nil {
		t.Fatalf("re-pause with resolved call id: %v", err)
	}
	if live, _ := pending.ForConversation(ctx, conv.ID, userID); len(live) != 2 || live[1].CallID != "call-a" {
		t.Fatalf("live pending after re-pause = %+v", live)
	}
}
