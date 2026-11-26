package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func TestMaybeCompactAppliesPersistedSummary(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	conv := convs.mustCreate("u-1", "")
	convs.summaries[conv.ID] = storage.ConversationSummary{Text: "old decisions", Covers: 2}
	srv := &Server{deps: ServerDeps{Convos: convs}}

	history := []model.Message{
		{Role: model.RoleUser, Content: "one"},
		{Role: model.RoleAssistant, Content: "two"},
		{Role: model.RoleUser, Content: "three"},
	}
	spec, recent, compacted := srv.maybeCompact(context.Background(), "u-1", conv.ID, chat.RunSpec{}, history)
	if compacted {
		t.Fatal("persisted summary is not ephemeral compaction")
	}
	if len(recent) != 1 || recent[0].Content != "three" {
		t.Fatalf("history not sliced: %+v", recent)
	}
	if !strings.Contains(spec.SystemPrompt, "old decisions") {
		t.Fatalf("summary not injected: %q", spec.SystemPrompt)
	}
}

func TestSummaryEndpoints(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	conv := convs.mustCreate("u-1", "")
	comp := chat.NewCompactor(testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "generated summary"},
	}), "test-summary-model", 40000, 2, 100)
	srv := &Server{deps: ServerDeps{Convos: convs, Msgs: msgs, Compactor: comp}}

	call := func(method, path, body string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, path, nil)
		} else {
			r = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		}
		r.SetPathValue("id", conv.ID)
		r = r.WithContext(withUser(r.Context(), "u-1", "user"))
		w := httptest.NewRecorder()
		switch method {
		case http.MethodGet:
			srv.handleGetSummary(w, r)
		case http.MethodPut:
			srv.handlePutSummary(w, r)
		case http.MethodDelete:
			srv.handleClearSummary(w, r)
		case http.MethodPost:
			srv.handleCompactNow(w, r)
		}
		return w
	}

	// Manually edit a summary; covers stays 0 until compacted.
	if rec := call(http.MethodPut, "/api/conversations/"+conv.ID+"/summary", `{"summary":"my notes"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	if convs.summaries[conv.ID].Text != "my notes" {
		t.Fatalf("summary not stored: %+v", convs.summaries[conv.ID])
	}
	if rec := call(http.MethodGet, "/api/conversations/"+conv.ID+"/summary", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "my notes") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodDelete, "/api/conversations/"+conv.ID+"/summary", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if convs.summaries[conv.ID].Text != "" {
		t.Fatal("summary not cleared")
	}

	// Seed a conversation long enough to summarize.
	parent := ""
	for i := 0; i < 5; i++ {
		payload, _ := json.Marshal(model.Message{Role: model.RoleUser, Content: "message"})
		id, err := msgs.Add(context.Background(), storage.Message{
			ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "message", Data: payload, ParentID: parent,
		})
		if err != nil {
			t.Fatal(err)
		}
		parent = id
	}
	rec := call(http.MethodPost, "/api/conversations/"+conv.ID+"/compact", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("compact: %d %s", rec.Code, rec.Body.String())
	}
	if convs.summaries[conv.ID].Text != "generated summary" || convs.summaries[conv.ID].Covers != 3 {
		t.Fatalf("compacted summary: %+v", convs.summaries[conv.ID])
	}
}
