package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// BotStore is the bots CRUD surface the handlers need; *storage.Bots
// satisfies it (pin below), tests run against in-memory fakes.
type BotStore interface {
	Create(ctx context.Context, bot storage.Bot) (storage.Bot, error)
	Get(ctx context.Context, userID, id string) (storage.Bot, error)
	List(ctx context.Context, userID string) ([]storage.Bot, error)
	Update(ctx context.Context, userID, id string, p storage.BotPatch) (storage.Bot, error)
	Delete(ctx context.Context, userID, id string) error
	Count(ctx context.Context, userID string) (int, error)
}

// BotMemoryStore is the per-bot compounding-memory surface; *storage.
// BotMemories satisfies it.
type BotMemoryStore interface {
	Add(ctx context.Context, m storage.BotMemory) (storage.BotMemory, error)
	ListRecent(ctx context.Context, userID, botID string, limit int) ([]storage.BotMemory, error)
	Delete(ctx context.Context, userID, botID, id string) error
}

var (
	_ BotStore       = (*storage.Bots)(nil)
	_ BotMemoryStore = (*storage.BotMemories)(nil)
)

// BotFileStore is the per-bot file workspace surface the file tools and
// read endpoints need; *storage.BotFiles satisfies it (pin below), tests
// run against in-memory fakes.
type BotFileStore interface {
	Upsert(ctx context.Context, file storage.BotFile) (storage.BotFile, error)
	Get(ctx context.Context, userID, botID, path string) (storage.BotFile, error)
	List(ctx context.Context, userID, botID string) ([]storage.BotFile, error)
	Delete(ctx context.Context, userID, botID, path string) error
}

var _ BotFileStore = (*storage.BotFiles)(nil)

// botFSAdapter adapts the storage workspace (methods take userID first)
// to chat.BotFileStore (which scopes by botID per call): the adapter closes
// over the run's user so the tool executors only carry the bot ID.
type botFSAdapter struct {
	fs     BotFileStore
	userID string
}

var _ chat.BotFileStore = botFSAdapter{}

func (a botFSAdapter) Read(ctx context.Context, botID, path string) (string, error) {
	file, err := a.fs.Get(ctx, a.userID, botID, path)
	if err != nil {
		return "", err
	}
	return file.Content, nil
}

func (a botFSAdapter) Write(ctx context.Context, botID, path, content string) error {
	_, err := a.fs.Upsert(ctx, storage.BotFile{BotID: botID, UserID: a.userID, Path: path, Content: content})
	return err
}

func (a botFSAdapter) List(ctx context.Context, botID string) ([]chat.BotFileInfo, error) {
	files, err := a.fs.List(ctx, a.userID, botID)
	if err != nil {
		return nil, err
	}
	out := make([]chat.BotFileInfo, len(files))
	for i, f := range files {
		out[i] = chat.BotFileInfo{Path: f.Path, Size: f.SizeBytes, UpdatedAt: f.UpdatedAt}
	}
	return out, nil
}

func (a botFSAdapter) Delete(ctx context.Context, botID, path string) error {
	return a.fs.Delete(ctx, a.userID, botID, path)
}

// botFileMaxBytesDefault mirrors config's BOT_FILE_MAX_BYTES default so a
// zero-valued test config keeps the tool usable.
const botFileMaxBytesDefault = 262144

// botFileMaxBytes resolves the write_file content cap, filling the unset
// (zero-valued test config) field with the package default.
func (s *Server) botFileMaxBytes() int {
	if s.deps.Cfg.BotFileMaxBytes > 0 {
		return s.deps.Cfg.BotFileMaxBytes
	}
	return botFileMaxBytesDefault
}

// botFor resolves the live bot for a bot-scoped conversation (nil for plain
// conversations or when the store is missing or the bot is gone). Run paths
// use it to thread the bot into userTools and buildDeps; fail-open keeps a
// broken bot row from blocking an otherwise valid conversation.
func (s *Server) botFor(ctx context.Context, userID string, botID *string) *storage.Bot {
	if botID == nil || s.deps.Bots == nil {
		return nil
	}
	bot, err := s.deps.Bots.Get(ctx, userID, *botID)
	if err != nil {
		return nil
	}
	return &bot
}

const botMemoriesListLimit = 50

const (
	maxBotNameChars         = 64
	maxBotEmojiRunes        = 8
	maxBotInstructionsChars = 8000
	botMemoriesPerFetch     = 20
)

// botColors mirrors the frontend bot avatar palette; unknown colors would
// render no styling, so they are rejected at the door.
var botColors = map[string]bool{
	"sage": true, "sky": true, "amber": true, "rose": true, "violet": true, "slate": true,
}

type botDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Emoji          string   `json:"emoji"`
	Color          string   `json:"color"`
	Instructions   string   `json:"instructions"`
	Model          string   `json:"model"`
	Temperature    *float64 `json:"temperature"`
	RagEnabled     bool     `json:"ragEnabled"`
	IsArchived     bool     `json:"isArchived"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
	ConversationID string   `json:"conversationId"` // latest bot-scoped conversation, "" when none
}

func toBotDTO(b storage.Bot, conversationID string) botDTO {
	return botDTO{
		ID: b.ID, Name: b.Name, Emoji: b.Emoji, Color: b.Color,
		Instructions: b.Instructions, Model: b.Model, Temperature: b.Temperature,
		RagEnabled: b.RagEnabled, IsArchived: b.IsArchived,
		CreatedAt: b.CreatedAt.UTC().Format(timeRFC3339), UpdatedAt: b.UpdatedAt.UTC().Format(timeRFC3339),
		ConversationID: conversationID,
	}
}

type botMemoryDTO struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

func toBotMemoryDTO(m storage.BotMemory) botMemoryDTO {
	return botMemoryDTO{ID: m.ID, Content: m.Content, CreatedAt: m.CreatedAt.UTC().Format(timeRFC3339)}
}

// validateBotModel reuses the conversation-settings model rule: a non-empty
// model must exist in the catalog and its provider must be configured.
func (s *Server) validateBotModel(id string) error {
	if id == "" {
		return nil
	}
	spec, ok := chat.FindModel(id)
	if !ok {
		return fmt.Errorf("unknown model %s", id)
	}
	if !chat.HasProvider(s.deps.ModelKeys, spec.Provider) {
		return fmt.Errorf("model %s is not configured on this server", id)
	}
	return nil
}

// validateBotName trims and checks the shared name rule (used by create,
// which passes the raw string, and update, which passes *req.Name).
func validateBotName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxBotNameChars {
		return "", errors.New("name must be 1-64 characters")
	}
	return name, nil
}

func (s *Server) handleCreateBot(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Name         string   `json:"name"`
		Emoji        string   `json:"emoji"`
		Color        string   `json:"color"`
		Instructions string   `json:"instructions"`
		Model        string   `json:"model"`
		Temperature  *float64 `json:"temperature"`
		RagEnabled   *bool    `json:"ragEnabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	name, err := validateBotName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_name", err.Error())
		return
	}
	emoji := strings.TrimSpace(req.Emoji)
	if emoji == "" {
		emoji = "🤖"
	}
	if utf8.RuneCountInString(emoji) > maxBotEmojiRunes {
		writeError(w, http.StatusBadRequest, "invalid_emoji", "emoji must be at most 8 characters")
		return
	}
	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "sage"
	}
	if !botColors[color] {
		writeError(w, http.StatusBadRequest, "invalid_color", "color must be one of sage,sky,amber,rose,violet,slate")
		return
	}
	if utf8.RuneCountInString(req.Instructions) > maxBotInstructionsChars {
		writeError(w, http.StatusBadRequest, "invalid_instructions", "instructions must be at most 8000 characters")
		return
	}
	if err := s.validateBotModel(req.Model); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_model", err.Error())
		return
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		writeError(w, http.StatusBadRequest, "invalid_temperature", "temperature must be between 0 and 2")
		return
	}
	limit := s.deps.Cfg.MaxBotsPerUser
	if limit <= 0 {
		limit = 20 // config default; guards zero-valued test configs
	}
	if n, err := s.deps.Bots.Count(r.Context(), userID); err == nil && n >= limit {
		writeError(w, http.StatusBadRequest, "bot_limit_reached", "bot limit reached")
		return
	}
	rag := true
	if req.RagEnabled != nil {
		rag = *req.RagEnabled
	}
	bot, err := s.deps.Bots.Create(r.Context(), storage.Bot{
		UserID: userID, Name: name, Emoji: emoji, Color: color,
		Instructions: req.Instructions, Model: req.Model, Temperature: req.Temperature, RagEnabled: rag,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to create bot")
		return
	}
	// storage.Bots.Create cannot express RAG-off at creation (a plain bool
	// cannot say "unspecified", so it always forces true) — an explicit
	// ragEnabled=false is applied right after so the user's choice wins.
	if !rag {
		off := false
		if bot, err = s.deps.Bots.Update(r.Context(), userID, bot.ID, storage.BotPatch{RagEnabled: &off}); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "failed to create bot")
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"bot": toBotDTO(bot, "")})
}

// latestConvIDsByBot loads the user's bot→latest-conversation map, failing
// open to an empty map so listing still works if the lookup errors.
func (s *Server) latestConvIDsByBot(ctx context.Context, userID string) map[string]string {
	ids := map[string]string{}
	if s.deps.Convos == nil {
		return ids
	}
	if got, err := s.deps.Convos.LatestIDsByUser(ctx, userID); err == nil {
		ids = got
	}
	return ids
}

func (s *Server) handleListBots(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	bots, err := s.deps.Bots.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list bots")
		return
	}
	latest := s.latestConvIDsByBot(r.Context(), userID)
	out := make([]botDTO, len(bots))
	for i, b := range bots {
		out[i] = toBotDTO(b, latest[b.ID])
	}
	writeJSON(w, http.StatusOK, map[string]any{"bots": out, "count": len(out)})
}

func (s *Server) handleGetBot(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	bot, err := s.deps.Bots.Get(r.Context(), userID, id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "bot not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load bot")
		return
	}
	var mems []botMemoryDTO
	if s.deps.BotMemories != nil {
		rows, err := s.deps.BotMemories.ListRecent(r.Context(), userID, bot.ID, botMemoriesPerFetch)
		if err == nil {
			mems = make([]botMemoryDTO, len(rows))
			for i, m := range rows {
				mems[i] = toBotMemoryDTO(m)
			}
		}
	}
	latest := s.latestConvIDsByBot(r.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]any{
		"bot":      toBotDTO(bot, latest[bot.ID]),
		"memories": mems,
	})
}

func (s *Server) handleUpdateBot(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	var req struct {
		Name             *string  `json:"name"`
		Emoji            *string  `json:"emoji"`
		Color            *string  `json:"color"`
		Instructions     *string  `json:"instructions"`
		Model            *string  `json:"model"`
		Temperature      *float64 `json:"temperature"`
		ClearTemperature bool     `json:"clearTemperature"`
		RagEnabled       *bool    `json:"ragEnabled"`
		IsArchived       *bool    `json:"isArchived"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	patch := storage.BotPatch{
		Temperature:      req.Temperature,
		ClearTemperature: req.ClearTemperature,
		RagEnabled:       req.RagEnabled,
		IsArchived:       req.IsArchived,
	}
	if req.Name != nil {
		name, err := validateBotName(*req.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_name", err.Error())
			return
		}
		patch.Name = &name
	}
	if req.Emoji != nil {
		emoji := strings.TrimSpace(*req.Emoji)
		if emoji == "" || utf8.RuneCountInString(emoji) > maxBotEmojiRunes {
			writeError(w, http.StatusBadRequest, "invalid_emoji", "emoji must be 1-8 characters")
			return
		}
		patch.Emoji = &emoji
	}
	if req.Color != nil {
		color := strings.TrimSpace(*req.Color)
		if !botColors[color] {
			writeError(w, http.StatusBadRequest, "invalid_color", "color must be one of sage,sky,amber,rose,violet,slate")
			return
		}
		patch.Color = &color
	}
	if req.Instructions != nil {
		if utf8.RuneCountInString(*req.Instructions) > maxBotInstructionsChars {
			writeError(w, http.StatusBadRequest, "invalid_instructions", "instructions must be at most 8000 characters")
			return
		}
		patch.Instructions = req.Instructions
	}
	if req.Model != nil && *req.Model != "" {
		if err := s.validateBotModel(*req.Model); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_model", err.Error())
			return
		}
	}
	patch.Model = req.Model
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		writeError(w, http.StatusBadRequest, "invalid_temperature", "temperature must be between 0 and 2")
		return
	}
	bot, err := s.deps.Bots.Update(r.Context(), userID, id, patch)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "bot not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update bot")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bot": toBotDTO(bot, "")})
}

func (s *Server) handleDeleteBot(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Bots.Delete(r.Context(), userID, r.PathValue("id")); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "bot not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete bot")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCreateBotConversation opens a fresh conversation scoped to the bot:
// it inherits the bot's model and RAG setting and stores bot_id, so later
// runs rebuild the spec from the live bot (persona + memories) rather than
// a frozen prompt.
func (s *Server) handleCreateBotConversation(w http.ResponseWriter, r *http.Request) {
	if s.deps.Convos == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "conversations not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	botID := r.PathValue("id")
	bot, err := s.deps.Bots.Get(r.Context(), userID, botID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "bot not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load bot")
		return
	}
	model := bot.Model
	rag := bot.RagEnabled
	conv, err := s.deps.Convos.Create(r.Context(), userID, bot.Name, storage.ConversationPatch{
		Model:        &model,
		SystemPrompt: nil, // "" stays; the run spec builds the bot persona at send time
		RagEnabled:   &rag,
		BotID:        &botID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"conversation": toConversationDTO(conv)})
}

// handleListBotFiles returns the bot's workspace listing (paths, sizes,
// updated timestamps — no content). The canonical file browser source.
func (s *Server) handleListBotFiles(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	bot, ok := s.botOr404(w, r, userID)
	if !ok {
		return
	}
	type botFileDTO struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"sizeBytes"`
		UpdatedAt string `json:"updatedAt"`
	}
	files := []botFileDTO{}
	if s.deps.BotFiles != nil {
		rows, err := s.deps.BotFiles.List(r.Context(), userID, bot.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not list files")
			return
		}
		files = make([]botFileDTO, len(rows))
		for i, f := range rows {
			files[i] = botFileDTO{Path: f.Path, SizeBytes: f.SizeBytes, UpdatedAt: f.UpdatedAt.UTC().Format(timeRFC3339)}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// handleListBotMemories returns the bot's compounding memories (newest
// first, capped at 50) — the canonical list endpoint.
func (s *Server) handleListBotMemories(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	bot, ok := s.botOr404(w, r, userID)
	if !ok {
		return
	}
	mems := []botMemoryDTO{}
	if s.deps.BotMemories != nil {
		rows, err := s.deps.BotMemories.ListRecent(r.Context(), userID, bot.ID, botMemoriesListLimit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not list memories")
			return
		}
		mems = make([]botMemoryDTO, len(rows))
		for i, m := range rows {
			mems[i] = toBotMemoryDTO(m)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": mems})
}

// handleDeleteBotMemory removes one bot memory; 404 on unknown bot or memory.
func (s *Server) handleDeleteBotMemory(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	bot, ok := s.botOr404(w, r, userID)
	if !ok {
		return
	}
	if s.deps.BotMemories == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "memories not configured")
		return
	}
	if err := s.deps.BotMemories.Delete(r.Context(), userID, bot.ID, r.PathValue("memoryId")); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "memory not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete memory")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// botOr404 loads the path bot for the caller, writing the 404/500 itself;
// ok=false means the response is already written.
func (s *Server) botOr404(w http.ResponseWriter, r *http.Request, userID string) (storage.Bot, bool) {
	bot, err := s.deps.Bots.Get(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "bot not found")
		return storage.Bot{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load bot")
		return storage.Bot{}, false
	}
	return bot, true
}
