package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

const testSecret = "0123456789abcdef0123456789abcdef"

// newTestAgent builds the chat Agent over a registry that serves m for
// every model, so handler tests run offline against scripted responses.
func newTestAgent(t *testing.T, m model.StreamingModel) *chat.Agent {
	t.Helper()
	reg := chat.NewModelRegistryWithFactory(chat.ProviderKeys{Gemini: "test"}, func(string, string, *float64) (model.StreamingModel, error) {
		return m, nil
	})
	agent, err := chat.New(reg, golem.UsageLimit{}, chat.DefaultToolEnv())
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

// newHandlerServer wires NewServer with the scripted-model agent and the
// in-memory fakes; it also mints a bearer token for user u-1.
func newHandlerServer(t *testing.T, agent *chat.Agent, convs ConvoStore, msgs MsgStore, usage UsageStore) (http.Handler, string) {
	t.Helper()
	return newHandlerServerWithTools(t, agent, convs, msgs, usage, nil)
}

// newHandlerServerWithTools additionally wires a tools store (nil = no store;
// the tools routes then only ever see handler errors, which the old tests
// never touch).
func newHandlerServerWithTools(t *testing.T, agent *chat.Agent, convs ConvoStore, msgs MsgStore, usage UsageStore, tools ToolStore) (http.Handler, string) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(ServerDeps{
		Cfg:       config.Config{},
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:      authtest.NewService(testSecret),
		Tokens:    tm,
		Convos:    convs,
		Msgs:      msgs,
		Usage:     usage,
		Tools:     tools,
		Agent:     agent,
		Rates:     cost.Table{Default: cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5}},
		ModelKeys: chat.ProviderKeys{Gemini: "test"},
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token
}

func postMessage(t *testing.T, h http.Handler, token, convID, content string) *httptest.ResponseRecorder {
	t.Helper()
	req := reqJSON(http.MethodPost, "/api/conversations/"+convID+"/messages", map[string]string{"content": content})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type fakeConvos struct {
	convs map[string]storage.Conversation
	msgs  *fakeMsgs
}

func newFakeConvos(msgs *fakeMsgs) *fakeConvos {
	return &fakeConvos{convs: map[string]storage.Conversation{}, msgs: msgs}
}

var _ ConvoStore = (*fakeConvos)(nil)

func (f *fakeConvos) Create(_ context.Context, userID, title string, patch storage.ConversationPatch) (storage.Conversation, error) {
	conv := storage.Conversation{ID: fmt.Sprintf("c-%d", len(f.convs)+1), UserID: userID, Title: title}
	if patch.Model != nil {
		conv.Model = *patch.Model
	}
	if patch.Temperature != nil {
		t := *patch.Temperature
		conv.Temperature = &t
	}
	if patch.SystemPrompt != nil {
		conv.SystemPrompt = *patch.SystemPrompt
	}
	f.convs[conv.ID] = conv
	return conv, nil
}

func (f *fakeConvos) List(_ context.Context, userID string) ([]storage.Conversation, error) {
	var out []storage.Conversation
	for _, c := range f.convs {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeConvos) mustCreate(userID, title string) storage.Conversation {
	conv, _ := f.Create(context.Background(), userID, title, storage.ConversationPatch{})
	return conv
}

func (f *fakeConvos) SetSettings(_ context.Context, id, _ string, patch storage.ConversationPatch) error {
	conv, ok := f.convs[id]
	if !ok {
		return storage.ErrNotFound
	}
	if patch.Model != nil {
		conv.Model = *patch.Model
	}
	if patch.ClearTemperature {
		conv.Temperature = nil
	} else if patch.Temperature != nil {
		t := *patch.Temperature
		conv.Temperature = &t
	}
	if patch.SystemPrompt != nil {
		conv.SystemPrompt = *patch.SystemPrompt
	}
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) Delete(_ context.Context, id, userID string) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.convs, id)
	return nil
}

func (f *fakeConvos) ByID(_ context.Context, id, userID string) (storage.Conversation, error) {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.Conversation{}, storage.ErrNotFound
	}
	return conv, nil
}

func (f *fakeConvos) SetTitle(_ context.Context, id, _, title string) error {
	conv := f.convs[id]
	conv.Title = title
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) Touch(context.Context, string) error { return nil }

func (f *fakeConvos) CountMessages(_ context.Context, id, userID string) (int, error) {
	return len(f.msgs.forConv(id, userID)), nil
}

type fakeMsgs struct{ rows []storage.Message }

func newFakeMsgs() *fakeMsgs { return &fakeMsgs{} }

func (f *fakeMsgs) Add(_ context.Context, msg storage.Message) error {
	msg.ID = fmt.Sprintf("m-%d", len(f.rows)+1)
	f.rows = append(f.rows, msg)
	return nil
}

func (f *fakeMsgs) ForConversation(_ context.Context, convID, userID string) ([]storage.Message, error) {
	return f.forConv(convID, userID), nil
}

func (f *fakeMsgs) forConv(convID, userID string) []storage.Message {
	var out []storage.Message
	for _, m := range f.rows {
		if m.ConversationID == convID && m.UserID == userID {
			out = append(out, m)
		}
	}
	return out
}

func (f *fakeMsgs) ByID(_ context.Context, msgID, userID string) (storage.Message, error) {
	for _, m := range f.rows {
		if m.ID == msgID && m.UserID == userID {
			return m, nil
		}
	}
	return storage.Message{}, storage.ErrNotFound
}

func (f *fakeMsgs) UpdateContent(_ context.Context, msgID, userID, content string, data []byte) error {
	for i := range f.rows {
		if f.rows[i].ID == msgID && f.rows[i].UserID == userID {
			f.rows[i].Content = content
			f.rows[i].Data = data
			return nil
		}
	}
	return storage.ErrNotFound
}

// DeleteAfter mirrors the SQL (created_at, id) tuple comparison the real
// store performs for the truncate-and-resend endpoints.
func (f *fakeMsgs) DeleteAfter(_ context.Context, convID, userID string, after time.Time, afterID string) (int64, error) {
	var kept []storage.Message
	var removed int64
	for _, m := range f.rows {
		if m.ConversationID == convID && m.UserID == userID &&
			(m.CreatedAt.After(after) || (m.CreatedAt.Equal(after) && m.ID > afterID)) {
			removed++
			continue
		}
		kept = append(kept, m)
	}
	f.rows = kept
	return removed, nil
}

func (f *fakeMsgs) count() int { return len(f.rows) }

type fakeUsage struct{ events []storage.UsageEvent }

func newFakeUsage() *fakeUsage { return &fakeUsage{} }

func (f *fakeUsage) Add(_ context.Context, e storage.UsageEvent) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeUsage) Summary(_ context.Context, _ string, _ int) (storage.Summary, error) {
	return storage.Summary{}, nil
}

func TestSendMessageStreamsAndPersists(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "Hi there!"},
		Usage:   model.Usage{InputTokens: 12, OutputTokens: 7},
	})
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	h, token := newHandlerServer(t, agent, convs, msgs, usage)

	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "hello")

	body := rec.Body.String()
	for _, want := range []string{"event: delta", `"text":"Hi`, "event: done", `"requests":1`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in SSE:\n%s", want, body)
		}
	}
	if msgs.count() != 2 { // user + assistant
		t.Fatalf("messages persisted: %d", msgs.count())
	}
	// 12*0.3 + 7*2.5 = 21.1 micro-USD, rounded to 21.
	e := usage.events[0]
	if e.Kind != "chat" || e.Requests != 1 || e.ConversationID == nil || *e.ConversationID != conv.ID ||
		e.InputTokens != 12 || e.CostMicros != 21 {
		t.Fatalf("usage not recorded: %+v", usage.events)
	}
	if convs.convs[conv.ID].Title != "hello" {
		t.Fatalf("conversation should be auto-titled, got %q", convs.convs[conv.ID].Title)
	}
	// done carries the persisted assistant row's id and the ledger cost.
	if !strings.Contains(body, `"messageId":"m-2"`) || !strings.Contains(body, `"costUsd":0.00002`) {
		t.Fatalf("done event wrong:\n%s", body)
	}
}

func TestFailedRunEmitsErrorEvent(t *testing.T) {
	m := testmodel.StreamFunc(func(ctx context.Context, _ model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		_ = testmodel.Emit(onDelta, model.Delta{Content: "partial "})
		return model.Response{}, errors.New("boom")
	})
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	h, token := newHandlerServer(t, agent, convs, msgs, usage)

	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "hello")

	body := rec.Body.String()
	for _, want := range []string{"event: delta", "event: error", `"stage":"model"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in SSE:\n%s", want, body)
		}
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("done must not be emitted for a failed run:\n%s", body)
	}
	// golem's Partial is nil for a pre-evidence failure: only the user row exists.
	if msgs.count() != 1 {
		t.Fatalf("assistant row persisted on pre-evidence failure: %d", msgs.count())
	}
	if len(usage.events) != 0 {
		t.Fatalf("usage recorded on failed run: %+v", usage.events)
	}
}
