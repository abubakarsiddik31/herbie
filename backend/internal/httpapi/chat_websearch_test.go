package httpapi

import (
	"context"
	"encoding/json"
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
	"github.com/abubakarsiddik31/golem-chatbot/internal/websearch"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

type mockWebSearcher struct {
	results []websearch.Result
	err     error
	called  string
}

func (m *mockWebSearcher) Search(_ context.Context, query string) ([]websearch.Result, error) {
	m.called = query
	return m.results, m.err
}

func TestUserToolsIncludesWebSearch(t *testing.T) {
	sWithout := &Server{deps: ServerDeps{}}
	toolsWithout, err := sWithout.userTools(context.Background(), "u-1", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range toolsWithout {
		if tl.Name == chat.WebSearchToolName {
			t.Fatal("web_search should be absent when WebSearch is nil")
		}
	}

	searcher := &mockWebSearcher{}
	sWith := &Server{deps: ServerDeps{WebSearch: searcher}}
	toolsWith, err := sWith.userTools(context.Background(), "u-1", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tl := range toolsWith {
		if tl.Name == chat.WebSearchToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("web_search should be present when WebSearch is configured")
	}
}

func TestWebSearchApprovalAndResume(t *testing.T) {
	searcher := &mockWebSearcher{
		results: []websearch.Result{
			{Title: "Go 1.25", URL: "https://golang.org/1.25", Snippet: "Go 1.25 release notes."},
		},
	}

	scripted := testmodel.New().
		Respond(model.Response{
			Message: model.Message{
				Role: model.RoleAssistant,
				ToolCalls: []model.ToolCall{
					{ID: "call-1", Name: chat.WebSearchToolName, Args: json.RawMessage(`{"query":"golang 1.25"}`)},
				},
			},
			Usage: model.Usage{InputTokens: 10, OutputTokens: 5},
		}).
		Respond(model.Response{
			Message: model.Message{Role: model.RoleAssistant, Content: "Go 1.25 is out: [Go 1.25](https://golang.org/1.25)."},
			Usage:   model.Usage{InputTokens: 20, OutputTokens: 10},
		})

	agent := newTestAgent(t, scripted)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	pending := &fakePending{}
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}

	usageStore := newFakeUsage()
	cfg := config.Config{WebSearchRequireApproval: true}
	h := NewServer(ServerDeps{
		Cfg:       cfg,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:      authtest.NewService(testSecret),
		Tokens:    tm,
		Profiles:  newFakeProfiles(),
		Convos:    convs,
		Msgs:      msgs,
		Usage:     usageStore,
		Pending:   pending,
		Agent:     agent,
		Rates:     cost.Table{Default: cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5}},
		ModelKeys: chat.ProviderKeys{Gemini: "test"},
		WebSearch: searcher,
	})
	token, _, _ := tm.Issue("u-1", time.Now())
	conv := convs.mustCreate("u-1", "")

	// 1. First send: model calls web_search -> requires approval -> pauses
	rec := postMessage(t, h, token, conv.ID, "what is new in Go 1.25?")
	body := rec.Body.String()
	if !strings.Contains(body, "event: approval_request") {
		t.Fatalf("expected approval_request event:\n%s", body)
	}
	if !strings.Contains(body, chat.WebSearchToolName) {
		t.Fatalf("expected toolName web_search in approval request:\n%s", body)
	}
	callID := announcedCallID(t, body)

	// 2. Resume via approvals
	appReq := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": callID, "approved": true}},
	})
	appReq.Header.Set("Authorization", "Bearer "+token)
	appRec := httptest.NewRecorder()
	h.ServeHTTP(appRec, appReq)
	if appRec.Code != http.StatusOK {
		t.Fatalf("approvals resume status %d: %s", appRec.Code, appRec.Body.String())
	}
	if searcher.called != "golang 1.25" {
		t.Fatalf("expected search query 'golang 1.25', got %q", searcher.called)
	}
	if !strings.Contains(appRec.Body.String(), "Go 1.25 is out") {
		t.Fatalf("expected resumed answer in stream:\n%s", appRec.Body.String())
	}
	if len(usageStore.searches) == 0 || usageStore.searches[0].Query != "golang 1.25" {
		t.Fatalf("expected recorded search query 'golang 1.25', got %+v", usageStore.searches)
	}
	foundWebSearchEvent := false
	for _, ev := range usageStore.events {
		if ev.Kind == "web_search" {
			foundWebSearchEvent = true
			break
		}
	}
	if !foundWebSearchEvent {
		t.Fatalf("expected usage event with kind 'web_search', got events: %+v", usageStore.events)
	}
}
