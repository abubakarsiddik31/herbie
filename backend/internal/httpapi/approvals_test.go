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
func pausedConversation(t *testing.T, gatedHits *int) (*fakeConvos, *fakeMsgs, *fakePending, http.Handler, string, storage.Conversation) {
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
	return convs, msgs, pending, h, token, conv
}

func TestApprovalsResume(t *testing.T) {
	hits := 0
	_, msgs, pending, h, token, conv := pausedConversation(t, &hits)

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": "call-1", "approved": true}},
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

func TestApprovalsDenial(t *testing.T) {
	hits := 0
	_, _, pending, h, token, conv := pausedConversation(t, &hits)

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": "call-1", "approved": false, "reason": "user denied"}},
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
	_, _, pending, h, token, conv := pausedConversation(t, &hits)

	badCases := []struct {
		name string
		body map[string]any
	}{
		{"unknown call", map[string]any{"decisions": []map[string]any{{"callId": "call-zzz", "approved": true}}}},
		{"missing decision", map[string]any{}},
		{"duplicate decision", map[string]any{"decisions": []map[string]any{
			{"callId": "call-1", "approved": true},
			{"callId": "call-1", "approved": true},
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
