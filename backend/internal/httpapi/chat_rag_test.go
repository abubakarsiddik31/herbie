package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:     authtest.NewService(testSecret),
		Tokens:   tm,
		Profiles: newFakeProfiles(),
		Cfg:      cfg,
		Convos:   convs,
		Msgs:     msgs,
		Usage:    usage,
		Tools:    newFakeToolStore(),
		Pending:  &fakePending{},
		Agent:    agent,
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
	ragSearch := func(_ context.Context, userID, query string, k int, _ []string) ([]rag.Scored, rag.UsageReport, error) {
		queries = append(queries, userID+":"+query)
		gotK = k
		return []rag.Scored{{Chunk: rag.Chunk{DocumentID: "d1", DocTitle: "notes.md", Content: "Paris is the capital of France"}, Score: 0.9}},
			rag.UsageReport{Embed: rag.EmbedUsage{InputTokens: 7}}, nil
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

func TestSendMessageRecordsRerankUsage(t *testing.T) {
	m := testmodel.New().
		Respond(toolCallResponse("call-1", "search_documents", json.RawMessage(`{"query":"capital of France"}`))).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "The capital is Paris [1]."},
			Usage: model.Usage{InputTokens: 10, OutputTokens: 5}})
	agent := newTestAgent(t, m)
	usage := newFakeUsage()
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)

	ragSearch := func(_ context.Context, _, _ string, _ int, _ []string) ([]rag.Scored, rag.UsageReport, error) {
		return []rag.Scored{{Chunk: rag.Chunk{DocumentID: "d1", DocTitle: "notes.md", Content: "Paris is the capital of France"}, Score: 0.9}},
			rag.UsageReport{
				Embed:       rag.EmbedUsage{InputTokens: 7},
				Reranked:    true,
				RerankModel: "gemini-2.5-flash",
				RerankIn:    5000,
				RerankOut:   50,
			}, nil
	}

	h, token := newRagHandlerServer(t, agent, convs, msgs, usage, ragSearch)
	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "what is the capital?")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	var e storage.UsageEvent
	for _, ev := range usage.events {
		if ev.Kind == "rerank" {
			e = ev
		}
	}
	if e.Kind != "rerank" {
		t.Fatalf("no rerank event: %+v", usage.events)
	}
	if e.Model != "gemini-2.5-flash" || e.InputTokens != 5000 || e.OutputTokens != 50 || e.Estimated {
		t.Fatalf("rerank event wrong: %+v", e)
	}
	if e.ConversationID == nil || *e.ConversationID != conv.ID {
		t.Fatalf("conversation not attached: %+v", e)
	}
	if e.UserID != "u-1" {
		t.Fatalf("user not attached: %+v", e)
	}
}

func TestSendMessagePersistsSourcesOnFinalMessage(t *testing.T) {
	m := testmodel.New().
		Respond(toolCallResponse("call-1", "search_documents", json.RawMessage(`{"query":"capital of France"}`))).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "The capital is Paris [1]."},
			Usage: model.Usage{InputTokens: 10, OutputTokens: 5}})
	agent := newTestAgent(t, m)
	usage := newFakeUsage()
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)

	ragSearch := func(_ context.Context, _, _ string, _ int, _ []string) ([]rag.Scored, rag.UsageReport, error) {
		return []rag.Scored{{Chunk: rag.Chunk{DocumentID: "d1", DocTitle: "notes.md", Content: "Paris is the capital of France"}, Score: 0.9}},
			rag.UsageReport{Embed: rag.EmbedUsage{InputTokens: 7}}, nil
	}

	h, token := newRagHandlerServer(t, agent, convs, msgs, usage, ragSearch)
	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "what is the capital?")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// The run's final assistant row carries the retrieved sources; the user
	// prompt row carries none.
	var final *storage.Message
	for i := range msgs.rows {
		row := &msgs.rows[i]
		if row.Role == "assistant" && row.Content == "The capital is Paris [1]." {
			final = row
		}
		if row.Role == "user" && len(row.Sources) > 0 && string(row.Sources) != "[]" {
			t.Fatalf("user row carries sources: %s", row.Sources)
		}
	}
	if final == nil {
		t.Fatalf("final assistant row missing: %+v", msgs.rows)
	}
	var got []map[string]any
	if err := json.Unmarshal(final.Sources, &got); err != nil {
		t.Fatalf("sources not JSON: %s", final.Sources)
	}
	if len(got) != 1 || got[0]["documentId"] != "d1" || got[0]["title"] != "notes.md" {
		t.Fatalf("sources wrong: %s", final.Sources)
	}

	// History reloads serve the same rows so the cards (and citation jumps)
	// survive a refresh.
	req := httptest.NewRequest(http.MethodGet, "/api/conversations/"+conv.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("history status %d: %s", rec2.Code, rec2.Body.String())
	}
	var detail struct {
		Messages []struct {
			Role    string           `json:"role"`
			Content string           `json:"content"`
			Sources []map[string]any `json:"sources"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range detail.Messages {
		if m.Role == "assistant" && m.Content == "The capital is Paris [1]." {
			found = true
			if len(m.Sources) != 1 || m.Sources[0]["documentId"] != "d1" {
				t.Fatalf("history sources wrong: %+v", m.Sources)
			}
		}
		if m.Role == "user" && len(m.Sources) != 0 {
			t.Fatalf("user history message carries sources: %+v", m.Sources)
		}
	}
	if !found {
		t.Fatalf("final answer missing from history: %s", rec2.Body.String())
	}
}

func TestUserToolsRespectConversationRagToggle(t *testing.T) {
	ragSearch := func(_ context.Context, _, _ string, _ int, _ []string) ([]rag.Scored, rag.UsageReport, error) {
		return nil, rag.UsageReport{}, nil
	}
	srv := &Server{deps: ServerDeps{RagSearch: ragSearch}}

	off, err := srv.userTools(context.Background(), "u-1", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range off {
		if tl.Name == chat.SearchToolName {
			t.Fatal("search_documents offered to a RAG-disabled conversation")
		}
	}

	on, err := srv.userTools(context.Background(), "u-1", true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tl := range on {
		if tl.Name == chat.SearchToolName {
			found = true
		}
	}
	if !found {
		t.Fatal("search_documents missing for a RAG-enabled conversation")
	}
}

func TestConversationRagToggleRoundTrip(t *testing.T) {
	m := testmodel.New()
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	ragSearch := func(_ context.Context, _, _ string, _ int, _ []string) ([]rag.Scored, rag.UsageReport, error) {
		return nil, rag.UsageReport{}, nil
	}
	h, token := newRagHandlerServer(t, agent, convs, msgs, newFakeUsage(), ragSearch)
	conv := convs.mustCreate("u-1", "")

	patch := reqJSON(http.MethodPatch, "/api/conversations/"+conv.ID, map[string]any{"ragEnabled": false})
	patch.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, patch)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/conversations/"+conv.ID, nil)
	get.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, get)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get status %d: %s", rec2.Code, rec2.Body.String())
	}
	var detail struct {
		Conversation struct {
			RagEnabled bool `json:"ragEnabled"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Conversation.RagEnabled {
		t.Fatalf("ragEnabled still true: %s", rec2.Body.String())
	}
}

func TestRunTurnCompactsHotHistory(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "still here"},
		Usage:   model.Usage{InputTokens: 5, OutputTokens: 3},
	})
	agent := newTestAgent(t, m)
	usage := newFakeUsage()
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	conv := convs.mustCreate("u-1", "")

	// Seed 12 long turns so the decoded history blows past the tiny
	// threshold below.
	for i := 0; i < 12; i++ {
		role := model.RoleUser
		if i%2 == 1 {
			role = model.RoleAssistant
		}
		content := strings.Repeat(fmt.Sprintf("message %d. ", i), 200)
		if err := msgs.Add(context.Background(), storage.Message{
			ConversationID: conv.ID, UserID: "u-1", Role: string(role),
			Content: content, Data: mustJSON(model.Message{Role: role, Content: content}),
		}); err != nil {
			t.Fatal(err)
		}
	}

	summarizer := testmodel.New().Respond(model.Response{
		Message: model.Message{Content: "SUMMARY"},
		Usage:   model.Usage{InputTokens: 9000, OutputTokens: 20},
	})
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(ServerDeps{
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:      authtest.NewService(testSecret),
		Tokens:    tm,
		Profiles:  newFakeProfiles(),
		Cfg:       config.Config{},
		Convos:    convs,
		Msgs:      msgs,
		Usage:     usage,
		Tools:     newFakeToolStore(),
		Pending:   &fakePending{},
		Agent:     agent,
		Rates:     cost.Table{Default: cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5}},
		ModelKeys: chat.ProviderKeys{Gemini: "test"},
		Compactor: chat.NewCompactor(summarizer, "gemini-2.5-flash", 100, 4, 800),
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	rec := postMessage(t, h, token, conv.ID, "continue please")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// The client is told the history was summarized, so it can say so.
	if !strings.Contains(rec.Body.String(), `"type":"compacted"`) {
		t.Fatalf("no compacted meta event in SSE:\n%s", rec.Body.String())
	}

	// The agent ran on the compacted window: 4 recent history messages
	// plus the fresh prompt (golem carries instructions out of band).
	reqs := m.Requests()
	if len(reqs) == 0 {
		t.Fatal("agent never called the model")
	}
	got := reqs[0].Messages
	// 4 recent history messages + the fresh prompt + the system
	// instructions carrying the compacted summary.
	if len(got) != 6 {
		t.Fatalf("model saw %d messages, want 6 (system + 4 recent + prompt)", len(got))
	}
	if got[0].Role != model.RoleSystem || !strings.Contains(got[0].Content, "SUMMARY") {
		t.Fatalf("summary missing from instructions: %.100q", got[0].Content)
	}
	if !strings.Contains(got[0].Content, "helpful assistant") {
		t.Fatalf("compaction dropped the default persona: %.100q", got[0].Content)
	}
	if !strings.Contains(got[1].Content, "message 8.") {
		t.Fatalf("stale history not trimmed: %.60q", got[1].Content)
	}
	if got[5].Content != "continue please" {
		t.Fatalf("fresh prompt missing: %.60q", got[5].Content)
	}

	var e storage.UsageEvent
	for _, ev := range usage.events {
		if ev.Kind == "compaction" {
			e = ev
		}
	}
	if e.Kind != "compaction" {
		t.Fatalf("no compaction event: %+v", usage.events)
	}
	if e.Model != "gemini-2.5-flash" || e.InputTokens != 9000 || e.OutputTokens != 20 || e.Estimated {
		t.Fatalf("compaction event wrong: %+v", e)
	}
	if e.ConversationID == nil || *e.ConversationID != conv.ID {
		t.Fatalf("conversation not attached: %+v", e)
	}
	if e.UserID != "u-1" {
		t.Fatalf("user not attached: %+v", e)
	}
}

func TestEmitSourcesExpandedContext(t *testing.T) {
	rec := httptest.NewRecorder()
	sink := &sseSink{w: rec, flush: rec}
	sources := []rag.Scored{{
		Chunk:   rag.Chunk{DocumentID: "d1", DocTitle: "notes.md", Heading: "Intro", Page: 2, Content: "core text"},
		Context: "neighbor-before core text neighbor-after",
		Score:   0.9,
	}}
	emitSources(sink, sources)
	body := rec.Body.String()
	for _, want := range []string{`"heading":"Intro"`, `"page":2`, "neighbor-before", "neighbor-after"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in sources event:\n%s", want, body)
		}
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
