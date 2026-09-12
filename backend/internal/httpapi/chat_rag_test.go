package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

// newRagHandlerServer is newHandlerServerFull with RagSearch wired and an
// embedding rate on the ledger.
func newRagHandlerServer(t *testing.T, agent *chat.Agent, convs ConvoStore, msgs MsgStore, usage *fakeUsage, ragSearch RagSearchFunc) (http.Handler, string) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.RAG.EmbeddingModel = "gemini-embedding-001"
	h := NewServer(ServerDeps{
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:    authtest.NewService(testSecret),
		Tokens:  tm,
		Cfg:     cfg,
		Convos:  convs,
		Msgs:    msgs,
		Usage:   usage,
		Tools:   newFakeToolStore(),
		Pending: &fakePending{},
		Agent:   agent,
		Rates: cost.Table{Default: cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5},
			ByModel: map[string]cost.Rates{"gemini-embedding-001": {ChatInputPerM: 0.15}}},
		ModelKeys: chat.ProviderKeys{Gemini: "test"},
		RagSearch: ragSearch,
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token
}

func TestSendMessageSearchesDocumentsAndMetersEmbedding(t *testing.T) {
	m := testmodel.New().
		Respond(toolCallResponse("call-1", "search_documents", json.RawMessage(`{"query":"capital of France"}`))).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "The capital is Paris [1]."},
			Usage: model.Usage{InputTokens: 10, OutputTokens: 5}})
	agent := newTestAgent(t, m)
	usage := newFakeUsage()
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)

	var queries []string
	var gotK int
	ragSearch := func(_ context.Context, userID, query string, k int) ([]rag.Scored, rag.EmbedUsage, error) {
		queries = append(queries, userID+":"+query)
		gotK = k
		return []rag.Scored{{Chunk: rag.Chunk{DocumentID: "d1", DocTitle: "notes.md", Content: "Paris is the capital of France"}, Score: 0.9}},
			rag.EmbedUsage{InputTokens: 7}, nil
	}

	h, token := newRagHandlerServer(t, agent, convs, msgs, usage, ragSearch)
	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "what is the capital?")
	body := rec.Body.String()

	for _, want := range []string{
		`"type":"tool_start"`, `"name":"search_documents"`,
		`"type":"tool_end"`, "event: sources", `"title":"notes.md"`,
		"event: done", "The capital is Paris [1].",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in SSE:\n%s", want, body)
		}
	}
	if len(queries) != 1 || queries[0] != "u-1:capital of France" {
		t.Fatalf("search queries: %v", queries)
	}
	if gotK != 5 {
		t.Fatalf("default k = %d", gotK)
	}

	// The query embedding is metered: one exact embedding usage event.
	var e storage.UsageEvent
	for _, ev := range usage.events {
		if ev.Kind == "embedding" {
			e = ev
		}
	}
	if e.Kind != "embedding" {
		t.Fatalf("no embedding event: %+v", usage.events)
	}
	if e.InputTokens != 7 || e.Estimated {
		t.Fatalf("embedding event wrong: %+v", e)
	}
	if e.CostMicros != 1 { // 7 tokens at 0.15 USD/MTok → round(7*0.15) = 1
		t.Fatalf("embedding cost: %d", e.CostMicros)
	}
	if e.ConversationID == nil || *e.ConversationID != conv.ID {
		t.Fatalf("conversation not attached: %+v", e)
	}
	if e.UserID != "u-1" {
		t.Fatalf("user not attached: %+v", e)
	}
}

func TestSendMessageWithoutRagHasNoSearchTool(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "plain answer"},
		Usage:   model.Usage{InputTokens: 1, OutputTokens: 1},
	})
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())

	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "hi")
	if strings.Contains(rec.Body.String(), "search_documents") {
		t.Fatal("search tool must not be registered when RAG is disabled")
	}
}

var _ = storage.ErrNotFound // keep storage imported for the fake types' package
