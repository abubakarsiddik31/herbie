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
		Profiles:  newFakeProfiles(),
		Memories:  newFakeMemories(),
		Convos:    convs,
		Msgs:      msgs,
		Feedback:  newFakeFeedback(msgs),
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
	f := &fakeConvos{convs: map[string]storage.Conversation{}, msgs: msgs}
	msgs.convs = f
	return f
}

var _ ConvoStore = (*fakeConvos)(nil)

func (f *fakeConvos) Create(_ context.Context, userID, title string, patch storage.ConversationPatch) (storage.Conversation, error) {
	titleSource := "heuristic"
	if patch.TitleSource != nil && *patch.TitleSource != "" {
		titleSource = *patch.TitleSource
	}
	conv := storage.Conversation{ID: fmt.Sprintf("c-%d", len(f.convs)+1), UserID: userID, Title: title, RagEnabled: true, TitleSource: titleSource}
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
	if patch.RagEnabled != nil {
		conv.RagEnabled = *patch.RagEnabled
	}
	if patch.ProjectID != nil {
		p := *patch.ProjectID
		conv.ProjectID = &p
	}
	if patch.BotID != nil {
		b := *patch.BotID
		conv.BotID = &b
	}
	f.convs[conv.ID] = conv
	return conv, nil
}

func (f *fakeConvos) List(_ context.Context, userID, q string, pinned, archived bool) ([]storage.Conversation, error) {
	var out []storage.Conversation
	for _, c := range f.convs {
		if c.UserID != userID {
			continue
		}
		if archived {
			if c.ArchivedAt == nil {
				continue
			}
		} else {
			if c.ArchivedAt != nil {
				continue
			}
			if pinned && c.PinnedAt == nil {
				continue
			}
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Title), strings.ToLower(q)) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeConvos) ListByProject(_ context.Context, projectID, userID string) ([]storage.Conversation, error) {
	var out []storage.Conversation
	for _, c := range f.convs {
		if c.UserID == userID && c.ArchivedAt == nil && c.ProjectID != nil && *c.ProjectID == projectID {
			out = append(out, c)
		}
	}
	return out, nil
}

// ListByBot mirrors the bot-scoped listing; latest = highest c-N id, which
// tracks the fake's creation order.
func (f *fakeConvos) ListByBot(_ context.Context, botID, userID string) ([]storage.Conversation, error) {
	var out []storage.Conversation
	for _, c := range f.convs {
		if c.UserID == userID && c.ArchivedAt == nil && c.BotID != nil && *c.BotID == botID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeConvos) LatestForBot(_ context.Context, botID, userID string) (storage.Conversation, error) {
	var best storage.Conversation
	found := false
	for _, c := range f.convs {
		if c.UserID != userID || c.ArchivedAt != nil || c.BotID == nil || *c.BotID != botID {
			continue
		}
		if !found || c.ID > best.ID {
			best, found = c, true
		}
	}
	if !found {
		return storage.Conversation{}, storage.ErrNotFound
	}
	return best, nil
}

func (f *fakeConvos) LatestIDsByUser(_ context.Context, userID string) (map[string]string, error) {
	out := map[string]string{}
	for _, c := range f.convs {
		if c.UserID != userID || c.BotID == nil {
			continue
		}
		if cur, ok := out[*c.BotID]; !ok || c.ID > cur {
			out[*c.BotID] = c.ID
		}
	}
	return out, nil
}

func (f *fakeConvos) mustCreate(userID, title string) storage.Conversation {
	conv, _ := f.Create(context.Background(), userID, title, storage.ConversationPatch{})
	return conv
}

func (f *fakeConvos) SetSettings(_ context.Context, id, userID string, patch storage.ConversationPatch) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
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
	if patch.RagEnabled != nil {
		conv.RagEnabled = *patch.RagEnabled
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

func (f *fakeConvos) SetHeuristicTitle(_ context.Context, id, userID, title string) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.ErrNotFound
	}
	if conv.TitleSource != "" && conv.TitleSource != "heuristic" {
		return nil
	}
	conv.Title = title
	conv.TitleSource = "heuristic"
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) SetAutoTitle(_ context.Context, id, userID, title string) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.ErrNotFound
	}
	if conv.TitleSource != "" && conv.TitleSource != "heuristic" {
		return nil
	}
	conv.Title = title
	conv.TitleSource = "auto"
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) RenameTitle(_ context.Context, id, userID, title string) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.ErrNotFound
	}
	conv.Title = title
	conv.TitleSource = "manual"
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) SetPinned(_ context.Context, id, userID string, pinned bool) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.ErrNotFound
	}
	if pinned {
		now := time.Now()
		conv.PinnedAt = &now
	} else {
		conv.PinnedAt = nil
	}
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) SetArchived(_ context.Context, id, userID string, archived bool) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.ErrNotFound
	}
	if archived {
		now := time.Now()
		conv.ArchivedAt = &now
	} else {
		conv.ArchivedAt = nil
	}
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) SetActiveLeaf(_ context.Context, id, userID, leafID string) error {
	conv, ok := f.convs[id]
	if !ok || conv.UserID != userID {
		return storage.ErrNotFound
	}
	if leafID == "" {
		conv.ActiveLeafID = nil
	} else {
		leaf := leafID
		conv.ActiveLeafID = &leaf
	}
	f.convs[id] = conv
	return nil
}

func (f *fakeConvos) Touch(context.Context, string) error { return nil }

func (f *fakeConvos) CountMessages(_ context.Context, id, userID string) (int, error) {
	return len(f.msgs.forConv(id, userID)), nil
}

type fakeMsgs struct {
	rows   []storage.Message
	nextID int
	convs  *fakeConvos
}

func newFakeMsgs() *fakeMsgs { return &fakeMsgs{} }

func (f *fakeMsgs) Add(_ context.Context, msg storage.Message) (string, error) {
	if f.nextID == 0 {
		f.nextID = len(f.rows)
	}
	f.nextID++
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("m-%d", f.nextID)
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	f.rows = append(f.rows, msg)
	return msg.ID, nil
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

// PathToLeaf mirrors the upward recursive walk: root→leaf order, cycle-safe.
func (f *fakeMsgs) PathToLeaf(_ context.Context, convID, userID, leafID string) ([]storage.Message, error) {
	if leafID == "" {
		return nil, nil
	}
	byID := map[string]storage.Message{}
	for _, m := range f.rows {
		if m.ConversationID == convID && m.UserID == userID {
			byID[m.ID] = m
		}
	}
	var reversed []storage.Message
	seen := map[string]bool{}
	for id := leafID; id != "" && !seen[id]; {
		m, ok := byID[id]
		if !ok {
			break
		}
		seen[id] = true
		reversed = append(reversed, m)
		id = m.ParentID
	}
	out := make([]storage.Message, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out, nil
}

func (f *fakeMsgs) LatestRowID(_ context.Context, convID, userID string) (string, error) {
	rows := f.forConv(convID, userID)
	return latestRowID(rows), nil
}

// DeleteBranch removes a subtree and only moves the leaf when it lived in
// that subtree, matching the storage transaction.
func (f *fakeMsgs) DeleteBranch(_ context.Context, convID, msgID, userID string) error {
	var target *storage.Message
	for i := range f.rows {
		m := &f.rows[i]
		if m.ID == msgID && m.ConversationID == convID && m.UserID == userID {
			target = m
			break
		}
	}
	if target == nil {
		return storage.ErrNotFound
	}
	subtree := map[string]bool{target.ID: true}
	changed := true
	for changed {
		changed = false
		for _, m := range f.rows {
			if m.ConversationID != convID || m.UserID != userID || subtree[m.ID] {
				continue
			}
			if m.ParentID != "" && subtree[m.ParentID] {
				subtree[m.ID] = true
				changed = true
			}
		}
	}
	var kept []storage.Message
	for _, m := range f.rows {
		if m.ConversationID == convID && m.UserID == userID && subtree[m.ID] {
			continue
		}
		kept = append(kept, m)
	}
	f.rows = kept
	if f.convs != nil {
		if conv, ok := f.convs.convs[convID]; ok && conv.ActiveLeafID != nil && subtree[*conv.ActiveLeafID] {
			conv.ActiveLeafID = nil
			if target.ParentID != "" {
				parent := target.ParentID
				conv.ActiveLeafID = &parent
			}
			f.convs.convs[convID] = conv
		}
	}
	return nil
}

// CopyPath appends fresh-ID copies into another conversation, remapping the
// parent chain and preserving display fields.
func (f *fakeMsgs) CopyPath(_ context.Context, srcConvID, userID, newConvID string, path []storage.Message) (string, error) {
	if len(path) == 0 {
		return "", nil
	}
	remap := map[string]string{}
	tail := ""
	for _, msg := range path {
		if msg.ConversationID != srcConvID || msg.UserID != userID {
			return "", storage.ErrNotFound
		}
		f.nextID++
		id := fmt.Sprintf("m-%d", f.nextID)
		copy := msg
		copy.ID = id
		copy.ConversationID = newConvID
		copy.ParentID = ""
		if msg.ParentID != "" {
			copy.ParentID = remap[msg.ParentID]
		}
		copy.ReplyToID = ""
		if msg.ReplyToID != "" {
			copy.ReplyToID = remap[msg.ReplyToID]
		}
		f.rows = append(f.rows, copy)
		remap[msg.ID] = id
		tail = id
	}
	return tail, nil
}

func (f *fakeMsgs) count() int { return len(f.rows) }

// fakeFeedback mirrors the (user, message) upsert and conversation-scoped
// read over the fake message rows.
type feedbackMsgReader interface {
	ForConversation(ctx context.Context, convID, userID string) ([]storage.Message, error)
}

type fakeFeedback struct {
	msgs    feedbackMsgReader
	ratings map[string]string
}

func newFakeFeedback(msgs feedbackMsgReader) *fakeFeedback {
	return &fakeFeedback{msgs: msgs, ratings: map[string]string{}}
}

var _ FeedbackStore = (*fakeFeedback)(nil)

func feedbackKey(userID, messageID string) string { return userID + "\x00" + messageID }

func (f *fakeFeedback) Set(_ context.Context, userID, messageID, rating string) error {
	f.ratings[feedbackKey(userID, messageID)] = rating
	return nil
}

func (f *fakeFeedback) Clear(_ context.Context, userID, messageID string) error {
	delete(f.ratings, feedbackKey(userID, messageID))
	return nil
}

func (f *fakeFeedback) ForConversation(ctx context.Context, convID, userID string) (map[string]string, error) {
	rows, err := f.msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, m := range rows {
		if rating, ok := f.ratings[feedbackKey(userID, m.ID)]; ok {
			out[m.ID] = rating
		}
	}
	return out, nil
}

type fakeUsage struct {
	events   []storage.UsageEvent
	searches []storage.SearchQuery
}

type fakeProfiles struct {
	texts    map[string]string
	defaults map[string]storage.GenerationDefaults
}

func newFakeProfiles() *fakeProfiles {
	return &fakeProfiles{texts: map[string]string{}, defaults: map[string]storage.GenerationDefaults{}}
}

var _ ProfileStore = (*fakeProfiles)(nil)

func (f *fakeProfiles) Instructions(_ context.Context, userID string) (string, error) {
	return f.texts[userID], nil
}

func (f *fakeProfiles) SetInstructions(_ context.Context, userID, text string) error {
	f.texts[userID] = text
	return nil
}

func (f *fakeProfiles) GenerationDefaults(_ context.Context, userID string) (storage.GenerationDefaults, error) {
	return f.defaults[userID], nil
}

func (f *fakeProfiles) SetGenerationDefaults(_ context.Context, userID string, d storage.GenerationDefaults) error {
	f.defaults[userID] = d
	return nil
}

type fakeMemories struct {
	items map[string][]storage.Memory
}

func newFakeMemories() *fakeMemories {
	return &fakeMemories{items: make(map[string][]storage.Memory)}
}

var _ MemoryStore = (*fakeMemories)(nil)

func (f *fakeMemories) List(_ context.Context, userID string) ([]storage.Memory, error) {
	return f.items[userID], nil
}

func (f *fakeMemories) Create(_ context.Context, userID, content string) (storage.Memory, error) {
	mem := storage.Memory{
		ID:        fmt.Sprintf("mem-%d", len(f.items[userID])+1),
		UserID:    userID,
		Content:   content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	f.items[userID] = append([]storage.Memory{mem}, f.items[userID]...)
	return mem, nil
}

func (f *fakeMemories) Delete(_ context.Context, userID, memoryID string) error {
	list := f.items[userID]
	for i, m := range list {
		if m.ID == memoryID {
			f.items[userID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

func (f *fakeMemories) DeleteAll(_ context.Context, userID string) error {
	delete(f.items, userID)
	return nil
}

func newFakeUsage() *fakeUsage { return &fakeUsage{} }

func (f *fakeUsage) Add(_ context.Context, e storage.UsageEvent) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeUsage) RecordSearch(_ context.Context, sq storage.SearchQuery) error {
	f.searches = append(f.searches, sq)
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

func TestUserMessage(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{
			err:  &golem.RunError{Stage: golem.StageLoop},
			want: "The model reached the maximum tool loop limit without completing its response. Please try asking a more focused question or rephrasing your request.",
		},
		{
			err:  &golem.RunError{Stage: golem.StageUsage},
			want: "The model run exceeded the configured token or request limit.",
		},
		{
			err:  &golem.RunError{Stage: golem.StageTool},
			want: "A tool execution failed while processing your request.",
		},
		{
			err:  &golem.RunError{Stage: golem.StageModel},
			want: "The AI model encountered an error generating a response.",
		},
		{
			err:  &golem.RunError{Stage: golem.StageDecode},
			want: "The model produced a response format that could not be parsed.",
		},
		{
			err:  &golem.RunError{Stage: golem.StageCanceled},
			want: "The request was canceled.",
		},
		{
			err:  errors.New("other"),
			want: "the model run failed",
		},
	}

	for _, tt := range tests {
		got := userMessage(tt.err)
		if got != tt.want {
			t.Errorf("userMessage(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestDeriveConversationTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "file with custom prompt uses prompt",
			content: "--- File: financial_report_2026.pdf ---\n```pdf\n[binary content]\n```\n\nPlease summarize the key highlights and revenue risks.",
			want:    "Please summarize the key highlights and revenue",
		},
		{
			name:    "file with default prompt uses filename",
			content: "--- File: financial_report_2026.pdf ---\n```pdf\n[binary content]\n```\n\nPlease analyze the attached file(s) above.",
			want:    "financial_report_2026.pdf",
		},
		{
			name:    "multiple files with default prompt",
			content: "--- File: notes.txt ---\n```txt\nhello\n```\n\n--- File: data.csv ---\n```csv\na,b\n```\n\nPlease analyze the attached file(s) above.",
			want:    "notes.txt, data.csv",
		},
		{
			name:    "three or more files with default prompt",
			content: "--- File: a.py ---\n```py\n```\n\n--- File: b.py ---\n```py\n```\n\n--- File: c.py ---\n```py\n```\n\nPlease analyze the attached file(s) above.",
			want:    "a.py + 2 files",
		},
		{
			name:    "file without any prompt text at all",
			content: "--- File: invoice_1042.pdf ---\n```pdf\n[content]\n```",
			want:    "invoice_1042.pdf",
		},
		{
			name:    "regular text message",
			content: "How does the Go memory allocator work under load?",
			want:    "How does the Go memory allocator work under load",
		},
		{
			name:    "regular text with markdown headers",
			content: "### System Architecture Blueprint\n\nDetails here...",
			want:    "System Architecture Blueprint Details here...",
		},
		{
			name:    "empty content",
			content: "   ",
			want:    "New conversation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveConversationTitle(tt.content)
			if got != tt.want {
				t.Errorf("deriveConversationTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
