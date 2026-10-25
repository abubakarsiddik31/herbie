package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// fakeBots mirrors storage.Bots semantics in memory: Create fills the
// emoji/color defaults and forces RAG on (the real store cannot express
// RAG-off at creation either), Count ignores archived bots.
type fakeBots struct {
	rows    map[string]storage.Bot
	nextID  int
	updates int
}

func newFakeBots() *fakeBots { return &fakeBots{rows: map[string]storage.Bot{}} }

var _ BotStore = (*fakeBots)(nil)

func (f *fakeBots) Create(_ context.Context, bot storage.Bot) (storage.Bot, error) {
	f.nextID++
	bot.ID = fmt.Sprintf("b-%d", f.nextID)
	if bot.Emoji == "" {
		bot.Emoji = "🤖"
	}
	if bot.Color == "" {
		bot.Color = "sage"
	}
	bot.RagEnabled = true
	bot.CreatedAt = time.Now()
	bot.UpdatedAt = time.Now()
	f.rows[bot.ID] = bot
	return bot, nil
}

func (f *fakeBots) Get(_ context.Context, userID, id string) (storage.Bot, error) {
	bot, ok := f.rows[id]
	if !ok || bot.UserID != userID {
		return storage.Bot{}, storage.ErrNotFound
	}
	return bot, nil
}

func (f *fakeBots) List(_ context.Context, userID string) ([]storage.Bot, error) {
	var out []storage.Bot
	for _, b := range f.rows {
		if b.UserID == userID && !b.IsArchived {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBots) Update(_ context.Context, userID, id string, p storage.BotPatch) (storage.Bot, error) {
	bot, ok := f.rows[id]
	if !ok || bot.UserID != userID {
		return storage.Bot{}, storage.ErrNotFound
	}
	if p.Name != nil {
		bot.Name = *p.Name
	}
	if p.Emoji != nil {
		bot.Emoji = *p.Emoji
	}
	if p.Color != nil {
		bot.Color = *p.Color
	}
	if p.Instructions != nil {
		bot.Instructions = *p.Instructions
	}
	if p.Model != nil {
		bot.Model = *p.Model
	}
	if p.ClearTemperature {
		bot.Temperature = nil
	} else if p.Temperature != nil {
		t := *p.Temperature
		bot.Temperature = &t
	}
	if p.RagEnabled != nil {
		bot.RagEnabled = *p.RagEnabled
	}
	if p.IsArchived != nil {
		bot.IsArchived = *p.IsArchived
	}
	bot.UpdatedAt = time.Now()
	f.updates++
	f.rows[id] = bot
	return bot, nil
}

func (f *fakeBots) Delete(_ context.Context, userID, id string) error {
	bot, ok := f.rows[id]
	if !ok || bot.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.rows, id)
	return nil
}

func (f *fakeBots) Count(_ context.Context, userID string) (int, error) {
	n := 0
	for _, b := range f.rows {
		if b.UserID == userID && !b.IsArchived {
			n++
		}
	}
	return n, nil
}

type fakeBotMemories struct{ rows []storage.BotMemory }

func newFakeBotMemories() *fakeBotMemories { return &fakeBotMemories{} }

var _ BotMemoryStore = (*fakeBotMemories)(nil)

func (f *fakeBotMemories) Add(_ context.Context, m storage.BotMemory) (storage.BotMemory, error) {
	m.ID = fmt.Sprintf("bm-%d", len(f.rows)+1)
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	f.rows = append(f.rows, m)
	return m, nil
}

func (f *fakeBotMemories) ListRecent(_ context.Context, userID, botID string, limit int) ([]storage.BotMemory, error) {
	var out []storage.BotMemory
	for i := len(f.rows) - 1; i >= 0 && len(out) < limit; i-- {
		if f.rows[i].UserID == userID && f.rows[i].BotID == botID {
			out = append(out, f.rows[i])
		}
	}
	return out, nil
}

func (f *fakeBotMemories) Delete(_ context.Context, userID, botID, id string) error {
	for i, m := range f.rows {
		if m.ID == id && m.BotID == botID && m.UserID == userID {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

func newBotTestServer(bots *fakeBots, convs ConvoStore, mems *fakeBotMemories, maxBots int) *Server {
	return &Server{deps: ServerDeps{
		Cfg:         config.Config{MaxBotsPerUser: maxBots},
		Bots:        bots,
		BotMemories: mems,
		Convos:      convs,
		Profiles:    newFakeProfiles(),
		ModelKeys:   chat.ProviderKeys{Gemini: "test"},
	}}
}

func decodeBotBody(t *testing.T, rec *httptest.ResponseRecorder) botDTO {
	t.Helper()
	var out struct {
		Bot botDTO `json:"bot"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out.Bot
}

func TestCreateBotDefaultsAndValidation(t *testing.T) {
	bots := newFakeBots()
	srv := newBotTestServer(bots, nil, nil, 20)

	// Defaults: emoji, color, and ragEnabled=true when the body omits them.
	rec := httptest.NewRecorder()
	srv.handleCreateBot(rec, authedJSONReq(http.MethodPost, "/api/bots",
		map[string]any{"name": "Scout", "instructions": "track prices"}, "u-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	bot := decodeBotBody(t, rec)
	if bot.Emoji != "🤖" || bot.Color != "sage" || !bot.RagEnabled || bot.Name != "Scout" {
		t.Fatalf("defaults not applied: %+v", bot)
	}

	// Explicit ragEnabled=false must survive the create-then-update dance.
	rec = httptest.NewRecorder()
	srv.handleCreateBot(rec, authedJSONReq(http.MethodPost, "/api/bots",
		map[string]any{"name": "Quiet", "ragEnabled": false}, "u-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("rag-off status %d: %s", rec.Code, rec.Body.String())
	}
	bot = decodeBotBody(t, rec)
	if bot.RagEnabled {
		t.Fatalf("ragEnabled=false was not honored: %+v", bot)
	}
	if bots.updates == 0 {
		t.Fatal("expected a follow-up Update to disable RAG at creation")
	}

	for _, tc := range []struct {
		name string
		body map[string]any
		code string
	}{
		{"empty name", map[string]any{"name": ""}, "invalid_name"},
		{"long name", map[string]any{"name": strings.Repeat("x", 65)}, "invalid_name"},
		{"bad color", map[string]any{"name": "Bot", "color": "neon"}, "invalid_color"},
		{"bad emoji", map[string]any{"name": "Bot", "emoji": "123456789"}, "invalid_emoji"},
		{"long instructions", map[string]any{"name": "Bot", "instructions": strings.Repeat("x", 8001)}, "invalid_instructions"},
		{"unknown model", map[string]any{"name": "Bot", "model": "nope"}, "invalid_model"},
		{"bad temperature", map[string]any{"name": "Bot", "temperature": 3.0}, "invalid_temperature"},
	} {
		rec := httptest.NewRecorder()
		srv.handleCreateBot(rec, authedJSONReq(http.MethodPost, "/api/bots", tc.body, "u-1"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d: %s", tc.name, rec.Code, rec.Body.String())
		}
		var e struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if e.Error.Code != tc.code {
			t.Fatalf("%s: want code %s, got %q", tc.name, tc.code, e.Error.Code)
		}
	}
}

func TestCreateBotLimitReached(t *testing.T) {
	bots := newFakeBots()
	if _, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "One"}); err != nil {
		t.Fatal(err)
	}
	srv := newBotTestServer(bots, nil, nil, 1)

	rec := httptest.NewRecorder()
	srv.handleCreateBot(rec, authedJSONReq(http.MethodPost, "/api/bots", map[string]any{"name": "Two"}, "u-1"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var e struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if e.Error.Code != "bot_limit_reached" {
		t.Fatalf("want bot_limit_reached, got %q", e.Error.Code)
	}
}

func TestUpdateBotValidationAndNotFound(t *testing.T) {
	bots := newFakeBots()
	created, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	srv := newBotTestServer(bots, nil, nil, 20)

	// Unknown bot id -> 404.
	rec := httptest.NewRecorder()
	r := authedJSONReq(http.MethodPatch, "/api/bots/missing", map[string]any{"name": "X"}, "u-1")
	r.SetPathValue("id", "missing")
	srv.handleUpdateBot(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing bot: want 404, got %d", rec.Code)
	}

	// Bad values still 400 with their specific codes.
	for _, tc := range []struct {
		name string
		body map[string]any
		code string
	}{
		{"empty name", map[string]any{"name": "  "}, "invalid_name"},
		{"bad color", map[string]any{"color": "neon"}, "invalid_color"},
		{"unknown model", map[string]any{"model": "nope"}, "invalid_model"},
	} {
		rec := httptest.NewRecorder()
		r := authedJSONReq(http.MethodPatch, "/api/bots/"+created.ID, tc.body, "u-1")
		r.SetPathValue("id", created.ID)
		srv.handleUpdateBot(rec, r)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d: %s", tc.name, rec.Code, rec.Body.String())
		}
		var e struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if e.Error.Code != tc.code {
			t.Fatalf("%s: want %s, got %q", tc.name, tc.code, e.Error.Code)
		}
	}

	// A valid patch applies.
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPatch, "/api/bots/"+created.ID,
		map[string]any{"name": "Scout II", "model": "gemini-3.5-pro"}, "u-1")
	r.SetPathValue("id", created.ID)
	srv.handleUpdateBot(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid patch: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if bot := decodeBotBody(t, rec); bot.Name != "Scout II" || bot.Model != "gemini-3.5-pro" {
		t.Fatalf("patch not applied: %+v", bot)
	}
}

func TestBotConversationCarriesBotID(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	srv := newBotTestServer(bots, convs, newFakeBotMemories(), 20)

	rec := httptest.NewRecorder()
	srv.handleCreateBot(rec, authedJSONReq(http.MethodPost, "/api/bots",
		map[string]any{"name": "Scout", "model": "gemini-3.5-pro"}, "u-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bot: %d %s", rec.Code, rec.Body.String())
	}
	bot := decodeBotBody(t, rec)

	// Unknown bot -> 404 before any conversation is created.
	rec = httptest.NewRecorder()
	r := authedJSONReq(http.MethodPost, "/api/bots/missing/conversations", nil, "u-1")
	r.SetPathValue("id", "missing")
	srv.handleCreateBotConversation(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing bot: want 404, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bots/"+bot.ID+"/conversations", nil, "u-1")
	r.SetPathValue("id", bot.ID)
	srv.handleCreateBotConversation(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bot conversation: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Conversation conversationDTO `json:"conversation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Conversation.BotID == nil || *out.Conversation.BotID != bot.ID {
		t.Fatalf("conversation botId = %v, want %s", out.Conversation.BotID, bot.ID)
	}
	if out.Conversation.Title != "Scout" || out.Conversation.Model != "gemini-3.5-pro" {
		t.Fatalf("bot defaults not inherited: %+v", out.Conversation)
	}
	stored, err := convs.ByID(context.Background(), out.Conversation.ID, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.BotID == nil || *stored.BotID != bot.ID {
		t.Fatalf("stored conversation lost bot_id: %+v", stored)
	}
}

func TestRunSpecForBotPrompt(t *testing.T) {
	bots := newFakeBots()
	mems := &fakeBotMemories{}
	srv := newBotTestServer(bots, nil, mems, 20)

	bot, err := bots.Create(context.Background(), storage.Bot{
		UserID: "u-1", Name: "Scout", Instructions: "track prices",
		Model: "gemini-3.5-pro",
	})
	if err != nil {
		t.Fatal(err)
	}
	temp := 0.7
	if _, err := bots.Update(context.Background(), "u-1", bot.ID, storage.BotPatch{
		Temperature: &temp, RagEnabled: &[]bool{false}[0],
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mems.Add(context.Background(), storage.BotMemory{BotID: bot.ID, UserID: "u-1", Content: "BTC hovering near 60k"}); err != nil {
		t.Fatal(err)
	}
	if err := srv.deps.Profiles.SetInstructions(context.Background(), "u-1", "Always be concise."); err != nil {
		t.Fatal(err)
	}

	botID := bot.ID
	conv := storage.Conversation{ID: "c-9", UserID: "u-1", BotID: &botID, Title: "Scout chat", SystemPrompt: "stale conversation prompt"}
	spec := srv.runSpecFor(context.Background(), conv)

	if !strings.Contains(spec.SystemPrompt, "You are Scout") {
		t.Fatalf("bot persona missing: %q", spec.SystemPrompt)
	}
	if !strings.Contains(spec.SystemPrompt, "track prices") {
		t.Fatalf("bot instructions missing: %q", spec.SystemPrompt)
	}
	if !strings.Contains(spec.SystemPrompt, "BTC hovering near 60k") {
		t.Fatalf("bot memory missing: %q", spec.SystemPrompt)
	}
	if strings.Contains(spec.SystemPrompt, "stale conversation prompt") {
		// The bot persona REPLACES the conversation-prompt component only.
		t.Fatalf("conversation prompt was not replaced by the bot persona: %q", spec.SystemPrompt)
	}
	if !strings.HasPrefix(spec.SystemPrompt, "[User preferences]") {
		t.Fatalf("user global prefix must stay first: %q", spec.SystemPrompt)
	}
	if spec.Model != "gemini-3.5-pro" {
		t.Fatalf("bot model not applied: %q", spec.Model)
	}
	if spec.Temperature == nil || *spec.Temperature != 0.7 {
		t.Fatalf("bot temperature not applied: %+v", spec.Temperature)
	}
	if spec.RagEnabled {
		t.Fatal("bot ragEnabled=false not applied")
	}

	// An archived bot stops overriding: the conversation's own settings win.
	archived := true
	if _, err := bots.Update(context.Background(), "u-1", bot.ID, storage.BotPatch{IsArchived: &archived}); err != nil {
		t.Fatal(err)
	}
	spec = srv.runSpecFor(context.Background(), conv)
	if strings.Contains(spec.SystemPrompt, "You are Scout") || !strings.Contains(spec.SystemPrompt, "stale conversation prompt") {
		t.Fatalf("archived bot must not override the spec: %q", spec.SystemPrompt)
	}
}

func TestListBotsCarriesLatestConversationID(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	srv := newBotTestServer(bots, convs, newFakeBotMemories(), 20)

	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	if _, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{BotID: &bot.ID}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	srv.handleListBots(rec, authedJSONReq(http.MethodGet, "/api/bots", nil, "u-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("list bots: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Bots  []botDTO `json:"bots"`
		Count int      `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || len(out.Bots) != 2 {
		t.Fatalf("want 2 bots, got %d/%d", out.Count, len(out.Bots))
	}
	byID := map[string]botDTO{}
	for _, b := range out.Bots {
		byID[b.ID] = b
	}
	if byID[bot.ID].ConversationID == "" {
		t.Fatalf("bot with conversation missing conversationId: %+v", byID[bot.ID])
	}
	for _, b := range out.Bots {
		if b.ID != bot.ID && b.ConversationID != "" {
			t.Fatalf("bot without conversation got conversationId %q", b.ConversationID)
		}
	}
}
