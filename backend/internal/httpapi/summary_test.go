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

func addPathMessages(t *testing.T, msgs *fakeMsgs, convID string, n int) []string {
	t.Helper()
	var ids []string
	parent := ""
	for i := 0; i < n; i++ {
		payload, _ := json.Marshal(model.Message{Role: model.RoleUser, Content: "message"})
		id, err := msgs.Add(context.Background(), storage.Message{
			ConversationID: convID, UserID: "u-1", Role: "user", Content: "message", Data: payload, ParentID: parent,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		parent = id
	}
	return ids
}

func TestMaybeCompactAppliesPersistedSummaryOnlyOnFullPath(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	conv := convs.mustCreate("u-1", "")
	ids := addPathMessages(t, msgs, conv.ID, 3)
	convs.summaries[conv.ID] = storage.ConversationSummary{Text: "old decisions", ThroughID: &ids[1]}
	srv := &Server{deps: ServerDeps{Convos: convs, Msgs: msgs}}

	history := []model.Message{
		{Role: model.RoleUser, Content: "one"},
		{Role: model.RoleUser, Content: "two"},
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

	// An approvals/suffix history that is not the full path must not apply.
	spec, recent, _ = srv.maybeCompact(context.Background(), "u-1", conv.ID, chat.RunSpec{}, history[1:])
	if len(recent) != 2 || strings.Contains(spec.SystemPrompt, "old decisions") {
		t.Fatalf("suffix applied summary: %+v %q", recent, spec.SystemPrompt)
	}

	// A boundary that is no longer on the active path is ignored.
	gone := "00000000-0000-0000-0000-000000000099"
	convs.summaries[conv.ID] = storage.ConversationSummary{Text: "stale", ThroughID: &gone}
	spec, recent, _ = srv.maybeCompact(context.Background(), "u-1", conv.ID, chat.RunSpec{}, history)
	if len(recent) != 3 || strings.Contains(spec.SystemPrompt, "stale") {
		t.Fatalf("stale boundary applied: %+v %q", recent, spec.SystemPrompt)
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

	call := func(method, body string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/", nil)
		} else {
			r = httptest.NewRequest(method, "/", bytes.NewReader([]byte(body)))
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

	if rec := call(http.MethodPut, `{"summary":"my notes"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	if convs.summaries[conv.ID].Text != "my notes" {
		t.Fatalf("summary not stored: %+v", convs.summaries[conv.ID])
	}
	if rec := call(http.MethodGet, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "my notes") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodDelete, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}

	ids := addPathMessages(t, msgs, conv.ID, 5)
	rec := call(http.MethodPost, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("compact: %d %s", rec.Code, rec.Body.String())
	}
	sum := convs.summaries[conv.ID]
	if sum.Text != "generated summary" || sum.ThroughID == nil || *sum.ThroughID != ids[2] {
		t.Fatalf("compacted summary: %+v (want boundary %s)", sum, ids[2])
	}
	if rec := call(http.MethodGet, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"coveredMessages":3`) {
		t.Fatalf("get after compact: %d %s", rec.Code, rec.Body.String())
	}
}
