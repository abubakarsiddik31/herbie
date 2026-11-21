package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

type fakeTitleGen struct {
	mu     sync.Mutex
	calls  []string
	title  string
	usage  model.Usage
	err    error
	called chan string
}

func newFakeTitleGen(title string) *fakeTitleGen {
	return &fakeTitleGen{title: title, called: make(chan string, 10)}
}

func (f *fakeTitleGen) ModelName() string { return "title-model" }

func (f *fakeTitleGen) Generate(_ context.Context, input string) (string, model.Usage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, input)
	f.mu.Unlock()
	select {
	case f.called <- input:
	default:
	}
	return f.title, f.usage, f.err
}

func (f *fakeTitleGen) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeTitleGen) lastInput() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

func eventsFor(events []storage.UsageEvent, kind string) []storage.UsageEvent {
	var out []storage.UsageEvent
	for _, e := range events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func titleServer(convs ConvoStore, msgs MsgStore, usage UsageStore, titles TitleGenerator) *Server {
	return &Server{deps: ServerDeps{
		Cfg:    config.Config{},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Convos: convs,
		Msgs:   msgs,
		Usage:  usage,
		Titles: titles,
	}}
}

func newTitleHandler(t *testing.T, agent *chat.Agent, convs ConvoStore, msgs MsgStore, usage UsageStore, titles TitleGenerator) (http.Handler, string) {
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
		Feedback:  newFakeFeedback(msgs.(*fakeMsgs)),
		Usage:     usage,
		Agent:     agent,
		Titles:    titles,
		ModelKeys: chat.ProviderKeys{Gemini: "test"},
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token
}

func waitForTitle(t *testing.T, convs *fakeConvos, convID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, ok := convs.convs[convID]; ok && c.Title == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("title never became %q: %+v", want, convs.convs[convID])
}

func TestAutotitleAppliesOnceAndRecordsUsage(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	titles := newFakeTitleGen("Deploy Notes")
	titles.usage = model.Usage{InputTokens: 20, OutputTokens: 5}
	s := titleServer(convs, msgs, usage, titles)

	conv := convs.mustCreate("u-1", "hello")
	userID, _ := msgs.Add(context.Background(), storage.Message{
		ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "--- File: notes.md ---\n```md\nsecret\n```\n\n deploy the app",
	})
	_ = convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", userID)

	s.autotitle("u-1", conv.ID)

	if got := convs.convs[conv.ID].Title; got != "Deploy Notes" {
		t.Fatalf("title = %q", got)
	}
	if got := convs.convs[conv.ID].TitleSource; got != "auto" {
		t.Fatalf("title source = %q", got)
	}
	if got := titles.lastInput(); got != "deploy the app" {
		t.Fatalf("title input = %q, want file block stripped", got)
	}
	titleEvents := eventsFor(usage.events, "title")
	if len(titleEvents) != 1 || titleEvents[0].Model != "title-model" || titleEvents[0].InputTokens != 20 {
		t.Fatalf("title usage = %+v", titleEvents)
	}

	// A second attempt after success (manual source is the only way to retry)
	// does not overwrite the auto title.
	s.autotitle("u-1", conv.ID)
	if titles.callCount() != 1 {
		t.Fatalf("auto title regenerated: calls=%d", titles.callCount())
	}
}

func TestAutotitleSkipsManualBotAndLaterTurns(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	titles := newFakeTitleGen("Should not apply")
	s := titleServer(convs, msgs, newFakeUsage(), titles)
	ctx := context.Background()

	manual := convs.mustCreate("u-1", "manual")
	_ = convs.RenameTitle(ctx, manual.ID, "u-1", "My Title")
	userID, _ := msgs.Add(ctx, storage.Message{ConversationID: manual.ID, UserID: "u-1", Role: "user", Content: "hello"})
	_ = convs.SetActiveLeaf(ctx, manual.ID, "u-1", userID)
	s.autotitle("u-1", manual.ID)
	if titles.callCount() != 0 || convs.convs[manual.ID].Title != "My Title" {
		t.Fatalf("manual title was touched: %+v calls=%d", convs.convs[manual.ID], titles.callCount())
	}

	botID := "b-1"
	manualSource := "manual"
	botConv, _ := convs.Create(ctx, "u-1", "Bot chat", storage.ConversationPatch{BotID: &botID, TitleSource: &manualSource})
	botUser, _ := msgs.Add(ctx, storage.Message{ConversationID: botConv.ID, UserID: "u-1", Role: "user", Content: "hello"})
	_ = convs.SetActiveLeaf(ctx, botConv.ID, "u-1", botUser)
	s.autotitle("u-1", botConv.ID)
	if titles.callCount() != 0 {
		t.Fatalf("bot conversation was titled")
	}

	multi := convs.mustCreate("u-1", "multi")
	first, _ := msgs.Add(ctx, storage.Message{ConversationID: multi.ID, UserID: "u-1", Role: "user", Content: "one"})
	second, _ := msgs.Add(ctx, storage.Message{ConversationID: multi.ID, UserID: "u-1", Role: "user", Content: "two", ParentID: first})
	_ = convs.SetActiveLeaf(ctx, multi.ID, "u-1", second)
	s.autotitle("u-1", multi.ID)
	if titles.callCount() != 0 {
		t.Fatalf("later turn was titled")
	}
}

func TestAutotitleFailureKeepsHeuristic(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	titles := newFakeTitleGen("")
	titles.err = fmt.Errorf("model down")
	s := titleServer(convs, msgs, newFakeUsage(), titles)

	conv := convs.mustCreate("u-1", "heuristic title")
	userID, _ := msgs.Add(context.Background(), storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "hello"})
	_ = convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", userID)
	s.autotitle("u-1", conv.ID)

	if convs.convs[conv.ID].Title != "heuristic title" || convs.convs[conv.ID].TitleSource != "heuristic" {
		t.Fatalf("fallback changed title: %+v", convs.convs[conv.ID])
	}
}

func TestFirstTurnFinishSchedulesTitle(t *testing.T) {
	agent := newTestAgent(t, scriptedPongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	titles := newFakeTitleGen("Auto Title")
	h, token := newTitleHandler(t, agent, convs, msgs, usage, titles)

	conv := convs.mustCreate("u-1", "")
	if rec := postMessage(t, h, token, conv.ID, "hello"); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}

	select {
	case input := <-titles.called:
		if input != "hello" {
			t.Fatalf("title input = %q", input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("title generation was not scheduled")
	}
	waitForTitle(t, convs, conv.ID, "Auto Title")
	if got := convs.convs[conv.ID].TitleSource; got != "auto" {
		t.Fatalf("title source = %q", got)
	}
}

func TestFinishResumedSchedulesTitle(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	titles := newFakeTitleGen("Resumed Title")
	s := titleServer(convs, msgs, newFakeUsage(), titles)
	ctx := context.Background()

	conv := convs.mustCreate("u-1", "paused prompt")
	userID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "paused prompt"})
	answerID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "assistant", ParentID: userID, Content: "answer"})
	_ = convs.SetActiveLeaf(ctx, conv.ID, "u-1", answerID)

	sink, ok := newSSESink(httptest.NewRecorder())
	if !ok {
		t.Fatal("recorder must support flushing")
	}
	s.finishResumed(ctx, "u-1", conv.ID, chat.RunSpec{Model: "gemini-2.5-flash"},
		chat.Outcome{Usage: model.Usage{InputTokens: 3, OutputTokens: 2}, Requests: 1}, sink)

	select {
	case <-titles.called:
	case <-time.After(2 * time.Second):
		t.Fatal("resume did not schedule title generation")
	}
	waitForTitle(t, convs, conv.ID, "Resumed Title")
}
