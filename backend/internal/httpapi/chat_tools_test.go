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
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

type fakePending struct{ rows []storage.PendingToolCall }

var _ PendingStore = (*fakePending)(nil)

func (f *fakePending) Add(_ context.Context, calls []storage.PendingToolCall) error {
	f.rows = append(f.rows, calls...)
	return nil
}

func (f *fakePending) ForConversation(_ context.Context, convID, userID string) ([]storage.PendingToolCall, error) {
	var out []storage.PendingToolCall
	for _, c := range f.rows {
		if c.ConversationID == convID && c.UserID == userID && c.Status == "pending" {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakePending) SetStatus(_ context.Context, userID, callID, status string) error {
	for i := range f.rows {
		if f.rows[i].CallID == callID && f.rows[i].UserID == userID && f.rows[i].Status == "pending" {
			f.rows[i].Status = status
			return nil
		}
	}
	return storage.ErrNotFound
}

// newHandlerServerFull is the complete harness: tools + pending stores and a
// config that allows the httptest loopback endpoints the stub tools hit.
func newHandlerServerFull(t *testing.T, agent *chat.Agent, convs ConvoStore, msgs MsgStore, usage UsageStore, tools ToolStore, pending PendingStore) (http.Handler, string) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.ToolAllowPrivateHosts = true
	h := NewServer(ServerDeps{
		Cfg:       cfg,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:      authtest.NewService(testSecret),
		Tokens:    tm,
		Convos:    convs,
		Msgs:      msgs,
		Usage:     usage,
		Tools:     tools,
		Pending:   pending,
		Agent:     agent,
		Rates:     cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5},
		ModelKeys: chat.ProviderKeys{Gemini: "test"},
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token
}

func toolCallResponse(id, name string, args json.RawMessage) model.Response {
	return model.Response{Message: model.Message{
		Role:      model.RoleAssistant,
		ToolCalls: []model.ToolCall{{ID: id, Name: name, Args: args}},
	}, Usage: model.Usage{InputTokens: 10, OutputTokens: 5}}
}

func storedWeatherTool(t *testing.T, userID, template string, approval bool) storage.UserTool {
	t.Helper()
	params, _ := json.Marshal([]chat.ParamDef{
		{Name: "city", In: "path", Type: "string", Required: true, Description: "city"},
	})
	return storage.UserTool{
		UserID: userID, Name: "get_weather", Description: "weather",
		Method: "GET", URLTemplate: template,
		Params: params, Headers: json.RawMessage(`{}`),
		RequireApproval: approval, Enabled: true,
	}
}

func TestSendMessageRunsToolAndPersistsEveryMessage(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"temp":"21C"}`)
	}))
	defer srv.Close()

	m := testmodel.New().
		Respond(toolCallResponse("call-1", "get_weather", json.RawMessage(`{"city":"x"}`))).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "It is 21C"},
			Usage: model.Usage{InputTokens: 10, OutputTokens: 5}})
	agent := newTestAgent(t, m)

	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	tools := newFakeToolStore()
	_, _ = tools.Create(context.Background(), storedWeatherTool(t, "u-1", srv.URL+"/wx/{{city}}", false))
	h, token := newHandlerServerFull(t, agent, convs, msgs, newFakeUsage(), tools, &fakePending{})

	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "weather in x?")

	body := rec.Body.String()
	for _, want := range []string{
		`"type":"tool_start"`, `"name":"get_weather"`,
		`"type":"tool_end"`, "event: done", "It is 21C",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in SSE:\n%s", want, body)
		}
	}
	if hits != 1 {
		t.Fatalf("tool hit the API %d times", hits)
	}

	// user, assistant tool-call, tool result, final assistant — all persisted.
	roles := map[string]int{}
	for _, row := range msgs.rows {
		roles[row.Role]++
	}
	if roles["tool"] != 1 || roles["assistant"] != 2 || roles["user"] != 1 {
		t.Fatalf("persisted roles: %v", roles)
	}

	get, _ := http.NewRequest(http.MethodGet, "/api/conversations/"+conv.ID, nil)
	get.Header.Set("Authorization", "Bearer "+token)
	grec := httptest.NewRecorder()
	h.ServeHTTP(grec, get)
	getBody := grec.Body.String()
	if strings.Contains(getBody, `"role":"tool"`) {
		t.Fatal("tool row leaked into transcript")
	}
	if !strings.Contains(getBody, "It is 21C") {
		t.Fatalf("transcript missing final answer: %s", getBody)
	}
}

func TestSendMessagePausesForApproval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("gated API must not be hit before approval")
	}))
	defer srv.Close()

	m := testmodel.New().
		Respond(toolCallResponse("call-1", "get_weather", json.RawMessage(`{"city":"x"}`)))
	agent := newTestAgent(t, m)

	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	tools := newFakeToolStore()
	_, _ = tools.Create(context.Background(), storedWeatherTool(t, "u-1", srv.URL+"/wx/{{city}}", true))
	pending := &fakePending{}
	h, token := newHandlerServerFull(t, agent, convs, msgs, newFakeUsage(), tools, pending)

	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "weather in x?")

	body := rec.Body.String()
	if !strings.Contains(body, "event: approval_request") || !strings.Contains(body, `"toolName":"get_weather"`) || !strings.Contains(body, `"callId":"`) {
		t.Fatalf("missing approval_request in SSE:\n%s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatal("paused stream must not emit done")
	}
	if len(pending.rows) != 1 || pending.rows[0].ToolName != "get_weather" {
		t.Fatalf("pending rows: %+v", pending.rows)
	}

	// A new message while approval is pending conflicts.
	rec2 := postMessage(t, h, token, conv.ID, "hello again")
	if rec2.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d body %s", rec2.Code, rec2.Body.String())
	}
}

func TestSendMessageWithoutToolsStoreStillWorks(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "plain"},
		Usage:   model.Usage{InputTokens: 1, OutputTokens: 1},
	})
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "hi")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "plain") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if msgs.count() != 2 {
		t.Fatalf("rows = %d", msgs.count())
	}
}
