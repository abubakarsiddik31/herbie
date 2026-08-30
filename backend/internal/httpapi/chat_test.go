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

// newHandlerServer wires NewServer with the scripted-model agent and the
// in-memory fakes; it also mints a bearer token for user u-1.
func newHandlerServer(t *testing.T, agent *chat.Agent, convs ConvoStore, msgs MsgStore, usage UsageStore) (http.Handler, string) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(ServerDeps{
		Cfg:    config.Config{},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:   authtest.NewService(testSecret),
		Tokens: tm,
		Convos: convs,
		Msgs:   msgs,
		Usage:  usage,
		Agent:  agent,
		Rates:  cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5},
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

func (f *fakeConvos) Create(_ context.Context, userID, title string) (storage.Conversation, error) {
	conv := storage.Conversation{ID: fmt.Sprintf("c-%d", len(f.convs)+1), UserID: userID, Title: title}
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
	conv, _ := f.Create(context.Background(), userID, title)
	return conv
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

func (f *fakeMsgs) count() int { return len(f.rows) }

type fakeUsage struct{ events []storage.UsageEvent }

func newFakeUsage() *fakeUsage { return &fakeUsage{} }

func (f *fakeUsage) Add(_ context.Context, e storage.UsageEvent) error {
	f.events = append(f.events, e)
	return nil
}

func TestSendMessageStreamsAndPersists(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "Hi there!"},
		Usage:   model.Usage{InputTokens: 12, OutputTokens: 7},
	})
	agent, err := chat.New(m, golem.UsageLimit{})
	if err != nil {
		t.Fatal(err)
	}
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
	agent, err := chat.New(m, golem.UsageLimit{})
	if err != nil {
		t.Fatal(err)
	}
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
