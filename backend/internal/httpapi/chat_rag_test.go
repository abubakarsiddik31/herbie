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
	"sync"
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
	if len(usage.searches) == 0 || usage.searches[0].Kind != "document_search" || usage.searches[0].Query != "capital of France" {
		t.Fatalf("document search query not recorded: %+v", usage.searches)
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

	off, err := srv.userTools(context.Background(), "u-1", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range off {
		if tl.Name == chat.SearchToolName || tl.Name == chat.ListDocumentsToolName || tl.Name == chat.ReadDocumentToolName {
			t.Fatalf("%s offered to a RAG-disabled conversation", tl.Name)
		}
	}

	on, err := srv.userTools(context.Background(), "u-1", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundSearch := false
	foundList := false
	foundRead := false
	for _, tl := range on {
		if tl.Name == chat.SearchToolName {
			foundSearch = true
		}
		if tl.Name == chat.ListDocumentsToolName {
			foundList = true
		}
		if tl.Name == chat.ReadDocumentToolName {
			foundRead = true
		}
	}
	if !foundSearch {
		t.Fatal("search_documents missing for a RAG-enabled conversation")
	}
	if !foundList {
		t.Fatal("list_documents missing for a RAG-enabled conversation")
	}
	if !foundRead {
		t.Fatal("read_document missing for a RAG-enabled conversation")
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
	prev := ""
	for i := 0; i < 12; i++ {
		role := model.RoleUser
		if i%2 == 1 {
			role = model.RoleAssistant
		}
		content := strings.Repeat(fmt.Sprintf("message %d. ", i), 200)
		id, err := msgs.Add(context.Background(), storage.Message{
			ConversationID: conv.ID, UserID: "u-1", Role: string(role), ParentID: prev,
			Content: content, Data: mustJSON(model.Message{Role: role, Content: content}),
		})
		if err != nil {
			t.Fatal(err)
		}
		prev = id
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
	if strings.Contains(rec.Body.String(), "list_documents") {
		t.Fatal("list tool must not be registered when RAG is disabled")
	}
	if strings.Contains(rec.Body.String(), "read_document") {
		t.Fatal("read tool must not be registered when RAG is disabled")
	}
}

func TestListDocsDepsScoping(t *testing.T) {
	docs := &fakeDocs{
		rows: []storage.Document{
			{ID: "d1", UserID: "u-1", Filename: "user_doc.txt", Status: "ready", ChunkCount: 3, SizeBytes: 1024},
		},
	}
	srv := &Server{deps: ServerDeps{Docs: docs}}

	// 1. Regular conversation -> returns user docs
	fn1 := srv.listDocsDeps("u-1", "")
	res1, err := fn1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res1) != 1 || res1[0].ID != "d1" {
		t.Fatalf("expected 1 user doc, got %+v", res1)
	}

	// 2. Project conversation -> returns project docs
	projID := "p-123"
	docs.rows = append(docs.rows, storage.Document{
		ID: "d2", UserID: "u-1", ProjectID: &projID, Filename: "project_doc.pdf", Status: "ready", ChunkCount: 10, SizeBytes: 50000,
	})
	fn2 := srv.listDocsDeps("u-1", projID)
	res2, err := fn2(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res2) != 1 || res2[0].ID != "d2" {
		t.Fatalf("expected 1 project doc, got %+v", res2)
	}
}

type fakeChatVectorsStore struct {
	chunks []rag.Chunk
}

func (f *fakeChatVectorsStore) UpsertChunks(_ context.Context, _, _, _ string, _ []rag.Chunk, _ [][]float32) error {
	return nil
}
func (f *fakeChatVectorsStore) DeleteDocument(_ context.Context, _ string) error { return nil }
func (f *fakeChatVectorsStore) HybridSearch(_ context.Context, _, _ string, _ []float32, _ float64, _ int, _ []string) ([]rag.Scored, error) {
	return nil, nil
}
func (f *fakeChatVectorsStore) ExpandRange(_ context.Context, _, docID string, lo, hi int) ([]rag.Chunk, error) {
	var out []rag.Chunk
	for _, ch := range f.chunks {
		if ch.DocumentID == docID && ch.Index >= lo && ch.Index <= hi {
			out = append(out, ch)
		}
	}
	return out, nil
}

func TestReadDocDepsScopingAndPaging(t *testing.T) {
	docs := &fakeDocs{
		rows: []storage.Document{
			{ID: "doc-1", UserID: "u-1", Filename: "paper.pdf", Status: "ready", ChunkCount: 5, SizeBytes: 10240},
			{ID: "doc-pending", UserID: "u-1", Filename: "raw.pdf", Status: "processing", ChunkCount: 0, SizeBytes: 10240},
		},
	}
	vecs := &fakeChatVectorsStore{
		chunks: []rag.Chunk{
			{DocumentID: "doc-1", DocTitle: "paper.pdf", Index: 0, Content: "chunk 0 content", Heading: "Abstract"},
			{DocumentID: "doc-1", DocTitle: "paper.pdf", Index: 1, Content: "chunk 1 content", Heading: "Intro"},
			{DocumentID: "doc-1", DocTitle: "paper.pdf", Index: 2, Content: "chunk 2 content", Heading: "Methods"},
			{DocumentID: "doc-1", DocTitle: "paper.pdf", Index: 3, Content: "chunk 3 content", Heading: "Results"},
			{DocumentID: "doc-1", DocTitle: "paper.pdf", Index: 4, Content: "chunk 4 content", Heading: "Discussion"},
		},
	}
	srv := &Server{deps: ServerDeps{Docs: docs, Vectors: vecs}}

	var sources []rag.Scored
	fn := srv.readDocDeps("u-1", "", &sources)
	if fn == nil {
		t.Fatal("readDocDeps should not be nil")
	}

	// 1. Read first 2 chunks
	res, err := fn(context.Background(), "doc-1", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalChunks != 5 || len(res.Chunks) != 2 || res.Offset != 0 {
		t.Fatalf("unexpected read result: %+v", res)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources appended, got %d", len(sources))
	}

	// 2. Read next 2 chunks (paging)
	res2, err := fn(context.Background(), "doc-1", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Chunks) != 2 || res2.Chunks[0].Index != 2 {
		t.Fatalf("unexpected second page: %+v", res2)
	}
	if len(sources) != 4 {
		t.Fatalf("expected 4 sources total, got %d", len(sources))
	}

	// 3. Read pending doc -> returns error
	_, err = fn(context.Background(), "doc-pending", 0, 2)
	if err == nil {
		t.Fatal("expected error for unready doc")
	}

	// 4. Read non-existent doc -> returns error
	_, err = fn(context.Background(), "doc-missing", 0, 2)
	if err == nil {
		t.Fatal("expected error for missing doc")
	}
}

func TestSearchDepsScopesToProject(t *testing.T) {
	p1, p2 := "p-1", "p-2"
	docs := &fakeDocs{rows: []storage.Document{
		{ID: "d1", UserID: "u-1", ProjectID: &p1, Filename: "one.txt", Status: "ready", ChunkCount: 1},
		{ID: "d2", UserID: "u-1", ProjectID: &p2, Filename: "two.txt", Status: "ready", ChunkCount: 1},
		{ID: "d3", UserID: "u-1", Filename: "global.txt", Status: "ready", ChunkCount: 1},
		{ID: "d4", UserID: "u-1", ProjectID: &p1, Filename: "pending.txt", Status: "processing"},
	}}
	var gotDocIDs [][]string
	ragSearch := func(_ context.Context, _, _ string, _ int, docIDs []string) ([]rag.Scored, rag.UsageReport, error) {
		gotDocIDs = append(gotDocIDs, append([]string(nil), docIDs...))
		return []rag.Scored{
			{Chunk: rag.Chunk{DocumentID: "d1", DocTitle: "one.txt", Content: "project hit"}},
			{Chunk: rag.Chunk{DocumentID: "d2", DocTitle: "two.txt", Content: "other project"}},
			{Chunk: rag.Chunk{DocumentID: "d3", DocTitle: "global.txt", Content: "global"}},
		}, rag.UsageReport{}, nil
	}
	srv := &Server{deps: ServerDeps{Docs: docs, RagSearch: ragSearch}}
	var sources []rag.Scored
	fn := srv.searchDeps("u-1", "c-1", p1, &sources)

	// Implicit search: only the project's ready documents are retrieved.
	scored, offset, err := fn(context.Background(), "q", 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotDocIDs) != 1 || len(gotDocIDs[0]) != 1 || gotDocIDs[0][0] != "d1" {
		t.Fatalf("project docs passed to rag: %v", gotDocIDs)
	}
	if len(scored) != 1 || scored[0].Chunk.DocumentID != "d1" || offset != 0 {
		t.Fatalf("scoped results: %+v offset=%d", scored, offset)
	}
	if len(sources) != 1 || sources[0].Chunk.DocumentID != "d1" {
		t.Fatalf("sources leaked: %+v", sources)
	}

	// Explicit out-of-project IDs: no retrieval at all.
	before := len(gotDocIDs)
	scored, _, err = fn(context.Background(), "q", 5, []string{"d2", "d3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(scored) != 0 || len(gotDocIDs) != before {
		t.Fatalf("out-of-project search should not call rag: %+v calls=%d", scored, len(gotDocIDs))
	}

	// Mixed IDs: intersect to the project and keep cumulative offsets.
	scored, offset, err = fn(context.Background(), "q", 5, []string{"d1", "d2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotDocIDs) != before+1 || len(gotDocIDs[before]) != 1 || gotDocIDs[before][0] != "d1" {
		t.Fatalf("mixed ids passed: %v", gotDocIDs[before:])
	}
	if offset != 1 || len(scored) != 1 || scored[0].Chunk.DocumentID != "d1" {
		t.Fatalf("mixed results: %+v offset=%d", scored, offset)
	}
	if len(sources) != 2 || sources[1].Chunk.DocumentID != "d1" {
		t.Fatalf("sources after mixed: %+v", sources)
	}
}

func TestSearchDepsEmptyProjectDoesNotSearchUserCorpus(t *testing.T) {
	called := false
	ragSearch := func(_ context.Context, _, _ string, _ int, _ []string) ([]rag.Scored, rag.UsageReport, error) {
		called = true
		return []rag.Scored{{Chunk: rag.Chunk{DocumentID: "global"}}}, rag.UsageReport{}, nil
	}
	docs := &fakeDocs{rows: []storage.Document{
		{ID: "d1", UserID: "u-1", Filename: "global.txt", Status: "ready", ChunkCount: 1},
	}}
	srv := &Server{deps: ServerDeps{Docs: docs, RagSearch: ragSearch}}
	var sources []rag.Scored
	fn := srv.searchDeps("u-1", "c-1", "empty-project", &sources)

	for _, ids := range [][]string{nil, {"d1"}} {
		scored, offset, err := fn(context.Background(), "q", 5, ids)
		if err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatalf("rag must not be called for an empty project (ids=%v)", ids)
		}
		if len(scored) != 0 || offset != 0 {
			t.Fatalf("empty project results: %+v offset=%d", scored, offset)
		}
	}
	if len(sources) != 0 {
		t.Fatalf("empty project leaked sources: %+v", sources)
	}
}

func TestReadDocDepsRejectsOtherProject(t *testing.T) {
	p1, p2 := "p-1", "p-2"
	docs := &fakeDocs{rows: []storage.Document{
		{ID: "d1", UserID: "u-1", ProjectID: &p1, Filename: "own.pdf", Status: "ready", ChunkCount: 1},
		{ID: "d2", UserID: "u-1", ProjectID: &p2, Filename: "other.pdf", Status: "ready", ChunkCount: 1},
		{ID: "d3", UserID: "u-1", Filename: "global.pdf", Status: "ready", ChunkCount: 1},
	}}
	vecs := &fakeChatVectorsStore{chunks: []rag.Chunk{
		{DocumentID: "d1", DocTitle: "own.pdf", Index: 0, Content: "own"},
		{DocumentID: "d2", DocTitle: "other.pdf", Index: 0, Content: "other"},
		{DocumentID: "d3", DocTitle: "global.pdf", Index: 0, Content: "global"},
	}}
	srv := &Server{deps: ServerDeps{Docs: docs, Vectors: vecs}}
	var sources []rag.Scored
	fn := srv.readDocDeps("u-1", p1, &sources)

	for _, id := range []string{"d2", "d3"} {
		if _, err := fn(context.Background(), id, 0, 1); err == nil {
			t.Fatalf("read %q must be rejected in project %q", id, p1)
		}
	}
	if len(sources) != 0 {
		t.Fatalf("rejected reads leaked sources: %+v", sources)
	}
	res, err := fn(context.Background(), "d1", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.DocumentID != "d1" || len(sources) != 1 {
		t.Fatalf("own project read failed: %+v sources=%d", res, len(sources))
	}
}

func TestListDocsDepsProjectScope(t *testing.T) {
	p1 := "p-1"
	docs := &fakeDocs{rows: []storage.Document{
		{ID: "d1", UserID: "u-1", Filename: "global.txt", Status: "ready"},
		{ID: "d2", UserID: "u-1", ProjectID: &p1, Filename: "one.txt", Status: "ready"},
	}}
	srv := &Server{deps: ServerDeps{Docs: docs}}
	ctx := context.Background()

	all, err := srv.listDocsDeps("u-1", "")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("unscoped docs: %+v", all)
	}
	p1docs, err := srv.listDocsDeps("u-1", p1)(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1docs) != 1 || p1docs[0].ID != "d2" {
		t.Fatalf("project docs: %+v", p1docs)
	}
	empty, err := srv.listDocsDeps("u-1", "p-empty")(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty project must not fall back to user docs: %+v", empty)
	}
}

func TestBuildDepsThreadsProjectScope(t *testing.T) {
	p1 := "p-1"
	docs := &fakeDocs{rows: []storage.Document{
		{ID: "d1", UserID: "u-1", ProjectID: &p1, Filename: "one.txt", Status: "ready", ChunkCount: 1},
		{ID: "d2", UserID: "u-1", Filename: "global.txt", Status: "ready", ChunkCount: 1},
	}}
	var got []string
	ragSearch := func(_ context.Context, _, _ string, _ int, docIDs []string) ([]rag.Scored, rag.UsageReport, error) {
		got = append([]string(nil), docIDs...)
		return []rag.Scored{{Chunk: rag.Chunk{DocumentID: "d1"}}}, rag.UsageReport{}, nil
	}
	srv := &Server{deps: ServerDeps{Docs: docs, RagSearch: ragSearch, Vectors: &fakeChatVectorsStore{}}}
	var sources []rag.Scored
	var mu sync.Mutex
	deps := srv.buildDeps("u-1", "c-1", p1, &sources, &mu, nil)

	if _, _, err := deps.Search(context.Background(), "q", 5, nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "d1" {
		t.Fatalf("buildDeps search scope: %v", got)
	}
	listed, err := deps.ListDocs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != "d1" {
		t.Fatalf("buildDeps list scope: %+v", listed)
	}
	if _, err := deps.ReadDoc(context.Background(), "d2", 0, 1); err == nil {
		t.Fatal("buildDeps read must reject a non-project document")
	}
}

var _ = storage.ErrNotFound // keep storage imported for the fake types' package
