package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

// pausedConversation runs the pause leg: a gated tool, a scripted model whose
// first response calls it, and the send-message request that parks the
// conversation. The scripted model's second response serves the resume run.
func pausedConversation(t *testing.T, gatedHits *int) (*fakeConvos, *fakeMsgs, *fakePending, http.Handler, string, storage.Conversation, string) {
	t.Helper()
	gated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*gatedHits++
		_, _ = io.WriteString(w, `{"result":"acted"}`)
	}))
	t.Cleanup(gated.Close)

	agent, _ := chat.New(testmodel.New().
		Respond(toolCallResponse("call-1", "get_weather", json.RawMessage(`{"city":"x"}`))).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "done after approval"},
			Usage: model.Usage{InputTokens: 4, OutputTokens: 2}}),
		golem.UsageLimit{}, chat.DefaultToolEnv())

	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	tools := newFakeToolStore()
	_, _ = tools.Create(context.Background(), storedWeatherTool(t, "u-1", gated.URL+"/wx/{{city}}", true))
	pending := &fakePending{}
	h, token := newHandlerServerFull(t, agent, convs, msgs, newFakeUsage(), tools, pending)

	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "go")
	if !strings.Contains(rec.Body.String(), "event: approval_request") {
		t.Fatalf("pause leg failed:\n%s", rec.Body.String())
	}
	return convs, msgs, pending, h, token, conv, announcedCallID(t, rec.Body.String())
}

// announcedCallID extracts the rewritten, conversation-unique call ID from an
// approval_request SSE body.
func announcedCallID(t *testing.T, body string) string {
	t.Helper()
	const marker = `event: approval_request`
	idx := strings.Index(body, marker)
	if idx < 0 {
		t.Fatalf("no approval_request in body")
	}
	var payload struct {
		Calls []struct {
			CallID string `json:"callId"`
		} `json:"calls"`
	}
	line := body[idx:]
	dataIdx := strings.Index(line, "data: ")
	if err := json.Unmarshal([]byte(strings.TrimSpace(line[dataIdx+6:strings.Index(line, "\n\n")])), &payload); err != nil {
		t.Fatalf("decode approval_request: %v", err)
	}
	if len(payload.Calls) == 0 || payload.Calls[0].CallID == "" {
		t.Fatalf("approval_request without calls")
	}
	return payload.Calls[0].CallID
}

func TestApprovalsResume(t *testing.T) {
	hits := 0
	_, msgs, pending, h, token, conv, callID := pausedConversation(t, &hits)

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": callID, "approved": true}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "event: delta") || !strings.Contains(body, "done after approval") || !strings.Contains(body, "event: done") {
		t.Fatalf("resume stream:\n%s", body)
	}
	if hits != 1 {
		t.Fatalf("gated API hits = %d, want 1", hits)
	}
	if pending.rows[0].Status != "approved" {
		t.Fatalf("row status = %q", pending.rows[0].Status)
	}
	// user, assistant tool-call, tool result, final assistant.
	roles := map[string]int{}
	for _, row := range msgs.rows {
		roles[row.Role]++
	}
	if roles["tool"] != 1 || roles["assistant"] != 2 || roles["user"] != 1 {
		t.Fatalf("persisted roles after resume: %v", roles)
	}
}

// TestApprovalsResumeWithCollidingCallIDs reproduces the Gemini ID scheme:
// two runs in one conversation both synthesize "call-1". The earlier run's
// call was answered; the paused run's call must still be resumable.
func TestApprovalsResumeWithCollidingCallIDs(t *testing.T) {
	agent, _ := chat.New(testmodel.New().
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "hn results"},
			Usage: model.Usage{InputTokens: 4, OutputTokens: 2}}),
		golem.UsageLimit{}, chat.DefaultToolEnv())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	pending := &fakePending{}
	// The approved re-run executes through the real tool, so it must be
	// declared in the user's enabled set.
	tools := newFakeToolStore()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"hits":[]}`)
	}))
	t.Cleanup(srv.Close)
	_, _ = tools.Create(context.Background(), storage.UserTool{
		UserID: "u-1", Name: "search_hn", Description: "hn", Method: "GET",
		URLTemplate: srv.URL, Params: json.RawMessage(`[]`), Headers: json.RawMessage(`{}`),
		Enabled: true,
	})
	h, token := newHandlerServerFull(t, agent, convs, msgs, newFakeUsage(), tools, pending)

	conv := convs.mustCreate("u-1", "")
	// Run 1 (answered) and the paused run, both with Gemini-style call-1.
	callMsg := func(name string) model.Message {
		return model.Message{Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{{ID: "call-1", Name: name, Args: json.RawMessage(`{}`)}}}
	}
	for _, m := range []model.Message{
		{Role: model.RoleUser, Content: "weather?"},
		callMsg("get_weather"),
		{Role: model.RoleTool, ToolCallID: "call-1", ToolName: "get_weather", Content: "21C"},
		{Role: model.RoleAssistant, Content: "It is 21C"},
		{Role: model.RoleUser, Content: "search hn"},
		callMsg("search_hn"),
	} {
		_ = msgs.Add(context.Background(), storage.Message{
			ConversationID: conv.ID, UserID: "u-1", Role: string(m.Role),
			Content: m.Content, Data: mustJSON(m),
		})
	}
	_ = pending.Add(context.Background(), []storage.PendingToolCall{
		{CallID: "call-1", ConversationID: conv.ID, UserID: "u-1", ToolName: "search_hn", Args: []byte(`{}`), Status: "pending"},
	})

	// The paused run's handcrafted history carries the raw Gemini-style
	// "call-1"; the decision must use exactly that ID.
	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": "call-1", "approved": true}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hn results") || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("resume with colliding IDs: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestApprovalsDenial(t *testing.T) {
	hits := 0
	_, _, pending, h, token, conv, callID := pausedConversation(t, &hits)

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": callID, "approved": false, "reason": "user denied"}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("denial status %d body %s", rec.Code, rec.Body.String())
	}
	if hits != 0 {
		t.Fatalf("denied call executed (%d hits)", hits)
	}
	if pending.rows[0].Status != "denied" {
		t.Fatalf("row status = %q", pending.rows[0].Status)
	}
}

func TestApprovalsValidation(t *testing.T) {
	hits := 0
	_, _, pending, h, token, conv, callID := pausedConversation(t, &hits)

	badCases := []struct {
		name string
		body map[string]any
	}{
		{"unknown call", map[string]any{"decisions": []map[string]any{{"callId": "call-zzz", "approved": true}}}},
		{"missing decision", map[string]any{}},
		{"duplicate decision", map[string]any{"decisions": []map[string]any{
			{"callId": callID, "approved": true},
			{"callId": callID, "approved": true},
		}}},
	}
	for _, tc := range badCases {
		req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", tc.body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d body %s", tc.name, rec.Code, rec.Body.String())
		}
	}

	// Validation errors must not have consumed the pending call.
	still, _ := pending.ForConversation(context.Background(), conv.ID, "u-1")
	if len(still) != 1 {
		t.Fatalf("validation errors consumed pending calls: %+v", still)
	}
}

func TestApprovalsNoPending(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	agent, _ := chat.New(testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "hi"},
	}), golem.UsageLimit{}, chat.DefaultToolEnv())
	h, token := newHandlerServerFull(t, agent, convs, msgs, newFakeUsage(), newFakeToolStore(), &fakePending{})
	conv := convs.mustCreate("u-1", "")

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": "call-1", "approved": true}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no-pending status = %d", rec.Code)
	}
}
