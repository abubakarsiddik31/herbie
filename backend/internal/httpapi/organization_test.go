package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func doAuthed(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := reqJSON(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func listConversationDTOs(t *testing.T, h http.Handler, token, query string) []conversationDTO {
	t.Helper()
	rec := doAuthed(t, h, token, http.MethodGet, "/api/conversations"+query, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %q: %d %s", query, rec.Code, rec.Body.String())
	}
	var out []conversationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func conversationTitles(rows []conversationDTO) []string {
	titles := make([]string, len(rows))
	for i, c := range rows {
		titles[i] = c.Title
	}
	sort.Strings(titles)
	return titles
}

func TestPinAndArchiveEndpoints(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "Pinned chat")
	foreign := convs.mustCreate("u-2", "Someone else")

	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/pin", map[string]bool{"pinned": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("pin: %d %s", rec.Code, rec.Body.String())
	}
	rows := listConversationDTOs(t, h, token, "?pinned=true")
	if len(rows) != 1 || rows[0].PinnedAt == nil {
		t.Fatalf("pinned view = %+v", rows)
	}
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/pin", map[string]bool{"pinned": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("unpin: %d", rec.Code)
	}

	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/archive", map[string]bool{"archived": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body.String())
	}
	if rows := listConversationDTOs(t, h, token, ""); len(rows) != 0 {
		t.Fatalf("default view must exclude archived: %+v", rows)
	}
	archivedRows := listConversationDTOs(t, h, token, "?archived=true")
	if len(archivedRows) != 1 || archivedRows[0].ArchivedAt == nil {
		t.Fatalf("archived view = %+v", archivedRows)
	}
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/archive", map[string]bool{"archived": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("unarchive: %d", rec.Code)
	}

	for _, path := range []string{"/pin", "/archive"} {
		body := map[string]bool{"pinned": true}
		if path == "/archive" {
			body = map[string]bool{"archived": true}
		}
		if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/missing"+path, body); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown %s = %d", path, rec.Code)
		}
		if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+foreign.ID+path, body); rec.Code != http.StatusNotFound {
			t.Fatalf("foreign %s = %d", path, rec.Code)
		}
	}
}

func TestListConversationViews(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())

	convs.mustCreate("u-1", "Plain moon")
	pinned := convs.mustCreate("u-1", "Pinned moon")
	archived := convs.mustCreate("u-1", "Archived moon")
	_ = convs.SetPinned(context.Background(), pinned.ID, "u-1", true)
	_ = convs.SetArchived(context.Background(), archived.ID, "u-1", true)

	if got := conversationTitles(listConversationDTOs(t, h, token, "")); len(got) != 2 || got[0] != "Pinned moon" || got[1] != "Plain moon" {
		t.Fatalf("default view = %v", got)
	}
	if got := conversationTitles(listConversationDTOs(t, h, token, "?pinned=true")); len(got) != 1 || got[0] != "Pinned moon" {
		t.Fatalf("pinned view = %v", got)
	}
	if got := conversationTitles(listConversationDTOs(t, h, token, "?archived=true")); len(got) != 1 || got[0] != "Archived moon" {
		t.Fatalf("archived view = %v", got)
	}
	if got := conversationTitles(listConversationDTOs(t, h, token, "?q=moon&pinned=true")); len(got) != 1 || got[0] != "Pinned moon" {
		t.Fatalf("search+pinned = %v", got)
	}
	if got := conversationTitles(listConversationDTOs(t, h, token, "?q=moon")); len(got) != 2 {
		t.Fatalf("q must exclude archived: %v", got)
	}
	if err := convs.SetPinned(context.Background(), archived.ID, "u-1", true); err != nil {
		t.Fatal(err)
	}
	if got := conversationTitles(listConversationDTOs(t, h, token, "?pinned=true&archived=true")); len(got) != 1 || got[0] != "Archived moon" {
		t.Fatalf("pinned+archived view = %v", got)
	}
	if rec := doAuthed(t, h, token, http.MethodGet, "/api/conversations?archived=maybe", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid archived bool = %d", rec.Code)
	}
}

func TestArchiveDuringActiveRun(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	m := testmodel.StreamFunc(func(ctx context.Context, _ model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return model.Response{}, ctx.Err()
		}
		_ = testmodel.Emit(onDelta, model.Delta{Content: "done"})
		return model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
	})
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, m), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "Running")
	// Already archived so the unarchive-during-run path is exercised too.
	_ = convs.SetArchived(context.Background(), conv.ID, "u-1", true)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- postMessage(t, h, token, conv.ID, "hold") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("run never started")
	}

	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/pin", map[string]bool{"pinned": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("pin during run = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/archive", map[string]bool{"archived": true}); rec.Code != http.StatusConflict || errorCode(t, rec) != "run_active" {
		t.Fatalf("archive during run = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/archive", map[string]bool{"archived": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("unarchive during run = %d %s", rec.Code, rec.Body.String())
	}

	close(release)
	if rec := <-done; rec.Code != http.StatusOK {
		t.Fatalf("run finished %d: %s", rec.Code, rec.Body.String())
	}
}

func TestArchiveWhileParkedApproval(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	pending := &fakePending{}
	h, token := newHandlerServerFull(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage(), newFakeToolStore(), pending)
	conv := convs.mustCreate("u-1", "Parked")
	if err := pending.Add(context.Background(), []storage.PendingToolCall{{
		CallID: "call-1", ConversationID: conv.ID, UserID: "u-1", ToolName: "t",
		Args: json.RawMessage(`{}`), Status: "pending",
	}}); err != nil {
		t.Fatal(err)
	}
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/archive", map[string]bool{"archived": true}); rec.Code != http.StatusConflict || errorCode(t, rec) != "approval_pending" {
		t.Fatalf("archive with parked approval = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/conversations/"+conv.ID+"/archive", map[string]bool{"archived": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("unarchive with parked approval = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSendEditRegenerateUnarchive(t *testing.T) {
	agent := newTestAgent(t, testmodel.New().
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "one"}}).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "two"}}).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "three"}}))
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "Archive me")
	ctx := context.Background()
	_ = convs.SetArchived(ctx, conv.ID, "u-1", true)

	if rec := postMessage(t, h, token, conv.ID, "first"); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := convs.ByID(ctx, conv.ID, "u-1"); got.ArchivedAt != nil {
		t.Fatal("send must not leave the conversation archived")
	}
	_ = convs.SetArchived(ctx, conv.ID, "u-1", true)
	rows := msgs.forConv(conv.ID, "u-1")
	firstUser := rows[0].ID
	if rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+firstUser+"/edit", map[string]string{"content": "edited"}); rec.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := convs.ByID(ctx, conv.ID, "u-1"); got.ArchivedAt != nil {
		t.Fatal("edit must unarchive")
	}
	_ = convs.SetArchived(ctx, conv.ID, "u-1", true)
	if rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/regenerate", map[string]any{}); rec.Code != http.StatusOK {
		t.Fatalf("regenerate: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := convs.ByID(ctx, conv.ID, "u-1"); got.ArchivedAt != nil {
		t.Fatal("regenerate must unarchive")
	}
}

func TestPatchConversationNotFound(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	foreign := convs.mustCreate("u-2", "Theirs")

	for _, tc := range []struct {
		name string
		path string
		body map[string]any
	}{
		{"unknown title", "/api/conversations/missing", map[string]any{"title": "x"}},
		{"unknown settings", "/api/conversations/missing", map[string]any{"ragEnabled": false}},
		{"foreign title", "/api/conversations/" + foreign.ID, map[string]any{"title": "x"}},
		{"foreign settings", "/api/conversations/" + foreign.ID, map[string]any{"ragEnabled": false}},
	} {
		if rec := doAuthed(t, h, token, http.MethodPatch, tc.path, tc.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestTitleSourceHeuristicAndManual(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	ctx := context.Background()

	fresh := convs.mustCreate("u-1", "")
	if rec := postMessage(t, h, token, fresh.ID, "Plan a trip to Mars"); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := convs.ByID(ctx, fresh.ID, "u-1"); got.TitleSource != "heuristic" || got.Title == "" {
		t.Fatalf("fresh chat title = %+v", got)
	}

	renamed := convs.mustCreate("u-1", "")
	if rec := doAuthed(t, h, token, http.MethodPatch, "/api/conversations/"+renamed.ID, map[string]any{"title": "My name"}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postMessage(t, h, token, renamed.ID, "this first message must not rename"); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := convs.ByID(ctx, renamed.ID, "u-1"); got.Title != "My name" || got.TitleSource != "manual" {
		t.Fatalf("manual title was overwritten: %+v", got)
	}
}

func TestBotAndForkTitlesAreManual(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	srv := newBotTestServer(bots, convs, newFakeBotMemories(), 20)
	rec := httptest.NewRecorder()
	srv.handleCreateBot(rec, authedJSONReq(http.MethodPost, "/api/bots", map[string]any{"name": "Scout"}, "u-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bot: %d %s", rec.Code, rec.Body.String())
	}
	bot := decodeBotBody(t, rec)
	rec = httptest.NewRecorder()
	r := authedJSONReq(http.MethodPost, "/api/bots/"+bot.ID+"/conversations", nil, "u-1")
	r.SetPathValue("id", bot.ID)
	srv.handleCreateBotConversation(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bot conversation: %d %s", rec.Code, rec.Body.String())
	}
	var botConv struct {
		Conversation conversationDTO `json:"conversation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &botConv); err != nil {
		t.Fatal(err)
	}
	if botConv.Conversation.TitleSource != "manual" {
		t.Fatalf("bot conversation title source = %q", botConv.Conversation.TitleSource)
	}

	msgs := newFakeMsgs()
	convs = newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "Source")
	if _, err := msgs.Add(context.Background(), storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "q"}); err != nil {
		t.Fatal(err)
	}
	rows := msgs.forConv(conv.ID, "u-1")
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", rows[0].ID); err != nil {
		t.Fatal(err)
	}
	rec = doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/fork", map[string]any{})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("fork: %d %s", rec.Code, rec.Body.String())
	}
	var forked conversationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &forked); err != nil {
		t.Fatal(err)
	}
	if forked.TitleSource != "manual" {
		t.Fatalf("fork title source = %q", forked.TitleSource)
	}
}

// fakePrompts is the in-memory PromptStore.
type fakePrompts struct {
	rows map[string]storage.SavedPrompt
	seq  int
}

func newFakePrompts() *fakePrompts { return &fakePrompts{rows: map[string]storage.SavedPrompt{}} }

func (f *fakePrompts) Create(_ context.Context, userID, title, body string, limit int) (storage.SavedPrompt, error) {
	if limit > 0 {
		n := 0
		for _, p := range f.rows {
			if p.UserID == userID {
				n++
			}
		}
		if n >= limit {
			return storage.SavedPrompt{}, storage.ErrLimit
		}
	}
	f.seq++
	now := time.Now()
	p := storage.SavedPrompt{ID: fmt.Sprintf("p-%d", f.seq), UserID: userID, Title: title, Body: body, CreatedAt: now, UpdatedAt: now}
	f.rows[p.ID] = p
	return p, nil
}

func (f *fakePrompts) List(_ context.Context, userID string) ([]storage.SavedPrompt, error) {
	var out []storage.SavedPrompt
	for _, p := range f.rows {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakePrompts) Update(_ context.Context, id, userID string, title, body *string) (storage.SavedPrompt, error) {
	p, ok := f.rows[id]
	if !ok || p.UserID != userID {
		return storage.SavedPrompt{}, storage.ErrNotFound
	}
	if title != nil {
		p.Title = *title
	}
	if body != nil {
		p.Body = *body
	}
	p.UpdatedAt = time.Now()
	f.rows[id] = p
	return p, nil
}

func (f *fakePrompts) Delete(_ context.Context, id, userID string) error {
	p, ok := f.rows[id]
	if !ok || p.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.rows, id)
	return nil
}

func (f *fakePrompts) Count(_ context.Context, userID string) (int, error) {
	n := 0
	for _, p := range f.rows {
		if p.UserID == userID {
			n++
		}
	}
	return n, nil
}

func newPromptServer(t *testing.T, prompts PromptStore) (http.Handler, string) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(ServerDeps{
		Cfg:     config.Config{},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:    authtest.NewService(testSecret),
		Tokens:  tm,
		Prompts: prompts,
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token
}

type savedPromptsResponse struct {
	Prompts []savedPromptDTO `json:"prompts"`
}

func TestSavedPromptCRUD(t *testing.T) {
	prompts := newFakePrompts()
	h, token := newPromptServer(t, prompts)

	rec := doAuthed(t, h, token, http.MethodPost, "/api/prompts", map[string]string{"title": "Standup", "body": "Summarize yesterday"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created savedPromptDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Title != "Standup" {
		t.Fatalf("created = %+v", created)
	}

	rec = doAuthed(t, h, token, http.MethodGet, "/api/prompts", nil)
	var listed savedPromptsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(listed.Prompts) != 1 {
		t.Fatalf("list = %d %+v", rec.Code, listed)
	}

	if rec := doAuthed(t, h, token, http.MethodPatch, "/api/prompts/"+created.ID, map[string]any{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty prompt patch = %d %s", rec.Code, rec.Body.String())
	}
	rec = doAuthed(t, h, token, http.MethodPatch, "/api/prompts/"+created.ID, map[string]string{"title": "Daily", "body": "Recap"})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	var patched savedPromptDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Title != "Daily" || patched.Body != "Recap" {
		t.Fatalf("patched = %+v", patched)
	}

	if rec := doAuthed(t, h, token, http.MethodDelete, "/api/prompts/"+created.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doAuthed(t, h, token, http.MethodGet, "/api/prompts", nil); rec.Code != http.StatusOK {
		t.Fatalf("list after delete: %d", rec.Code)
	} else {
		listed = savedPromptsResponse{}
		_ = json.Unmarshal(rec.Body.Bytes(), &listed)
		if len(listed.Prompts) != 0 {
			t.Fatalf("prompt survived delete: %+v", listed.Prompts)
		}
	}
}

func TestSavedPromptValidationLimitAndOwnership(t *testing.T) {
	prompts := newFakePrompts()
	h, token := newPromptServer(t, prompts)

	cases := []map[string]string{
		{"title": "", "body": "x"},
		{"title": "x", "body": ""},
		{"title": strings.Repeat("t", 201), "body": "x"},
		{"title": "x", "body": strings.Repeat("b", 8001)},
	}
	for _, body := range cases {
		if rec := doAuthed(t, h, token, http.MethodPost, "/api/prompts", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid prompt %+v = %d", body, rec.Code)
		}
	}

	for i := 0; i < maxSavedPrompts; i++ {
		if _, err := prompts.Create(context.Background(), "u-1", "t", "b", maxSavedPrompts); err != nil {
			t.Fatal(err)
		}
	}
	rec := doAuthed(t, h, token, http.MethodPost, "/api/prompts", map[string]string{"title": "t", "body": "b"})
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "prompt_limit" {
		t.Fatalf("limit = %d %s", rec.Code, rec.Body.String())
	}

	theirs, _ := prompts.Create(context.Background(), "u-2", "theirs", "secret", maxSavedPrompts)
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPatch, "/api/prompts/" + theirs.ID, map[string]string{"title": "hijack"}},
		{http.MethodDelete, "/api/prompts/" + theirs.ID, nil},
		{http.MethodPatch, "/api/prompts/missing", map[string]string{"title": "x"}},
		{http.MethodDelete, "/api/prompts/missing", nil},
	} {
		if rec := doAuthed(t, h, token, tc.method, tc.path, tc.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}
