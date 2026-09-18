package httpapi

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// The handler depends on store interfaces, not concrete *storage.* types,
// so its tests run offline with in-memory fakes; *storage.Conversations,
// *storage.Messages, and *storage.Usage satisfy them implicitly.
// ConvoStore is the full union the chat and CRUD routes need; the
// var _ ConvoStore = (*storage.Conversations)(nil) below pins the
// production wiring at compile time.
type ConvoStore interface {
	Create(ctx context.Context, userID, title string, patch storage.ConversationPatch) (storage.Conversation, error)
	List(ctx context.Context, userID, q string) ([]storage.Conversation, error)
	ByID(ctx context.Context, id, userID string) (storage.Conversation, error)
	SetTitle(ctx context.Context, id, userID, title string) error
	SetSettings(ctx context.Context, id, userID string, patch storage.ConversationPatch) error
	Touch(ctx context.Context, id string) error
	Delete(ctx context.Context, id, userID string) error
	CountMessages(ctx context.Context, id, userID string) (int, error)
}

var _ ConvoStore = (*storage.Conversations)(nil)

type MsgStore interface {
	Add(ctx context.Context, msg storage.Message) error
	ForConversation(ctx context.Context, convID, userID string) ([]storage.Message, error)
	ByID(ctx context.Context, msgID, userID string) (storage.Message, error)
	UpdateContent(ctx context.Context, msgID, userID, content string, data []byte) error
	DeleteAfter(ctx context.Context, convID, userID string, after time.Time, afterID string) (int64, error)
	DeleteMessage(ctx context.Context, convID, msgID, userID string) error
}

type UsageStore interface {
	Add(ctx context.Context, e storage.UsageEvent) error
	Summary(ctx context.Context, userID string, days int) (storage.Summary, error)
}

// PendingStore bridges paused runs (approval-gated tools) to the later
// resume request.
type PendingStore interface {
	Add(ctx context.Context, calls []storage.PendingToolCall) error
	ForConversation(ctx context.Context, convID, userID string) ([]storage.PendingToolCall, error)
	SetStatus(ctx context.Context, userID, callID, status string) error
}

var _ PendingStore = (*storage.PendingCalls)(nil)

// ShareStore bridges public conversation links to storage.
type ShareStore interface {
	Share(ctx context.Context, convID, userID, tokenHash string) (string, bool, error)
	Unshare(ctx context.Context, convID, userID string) error
	SharedHash(ctx context.Context, convID, userID string) (string, error)
	Resolve(ctx context.Context, tokenHash string) (string, string, error)
}

var _ ShareStore = (*storage.Shares)(nil)

// ProfileStore bridges user-level preferences (global instructions) to
// the profile endpoints and the run builder.
type ProfileStore interface {
	Instructions(ctx context.Context, userID string) (string, error)
	SetInstructions(ctx context.Context, userID, instructions string) error
}

var _ ProfileStore = (*storage.Users)(nil)

const maxPromptChars = 8000

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	var req struct {
		Content string       `json:"content"`
		Images  []imageInput `json:"images"`
	}
	if err := decodeJSON(r, &req); err != nil || (req.Content == "" && len(req.Images) == 0) {
		writeError(w, http.StatusBadRequest, "bad_request", "content or images is required")
		return
	}
	if len([]rune(req.Content)) > maxPromptChars {
		writeError(w, http.StatusBadRequest, "bad_request", "message too long")
		return
	}
	parts, err := decodeImages(req.Images)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	ctx := r.Context()
	conv, err := s.deps.Convos.ByID(ctx, convID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}
	spec := s.runSpecFor(ctx, conv)

	// A paused conversation must be resolved before new turns.
	if s.blockedByPendingApproval(w, ctx, convID, userID) {
		return
	}

	// Auto-title from the first user message.
	if n, err := s.deps.Convos.CountMessages(ctx, convID, userID); err == nil && n == 0 {
		_ = s.deps.Convos.SetTitle(ctx, convID, userID, truncateRunes(req.Content, 48))
	}

	// Persist the user message first, then build history from prior turns.
	// Image parts ride in the payload, so history replay resends them.
	if err := s.deps.Msgs.Add(ctx, storage.Message{
		ConversationID: convID, UserID: userID, Role: string(model.RoleUser),
		Content: req.Content,
		Data:    mustJSON(model.Message{Role: model.RoleUser, Content: req.Content, Parts: parts}),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store message")
		return
	}
	msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	// Rows already stored before this run must not be re-persisted when the
	// run's Messages replay them as history.
	known := make(map[string]bool, len(msgs))
	for _, row := range msgs {
		known[string(row.Data)] = true
	}
	s.runTurn(ctx, userID, convID, spec, historyFrom(msgs, len(msgs)-1), req.Content, parts, known, w)
}

// blockedByPendingApproval writes the response and reports true when the
// conversation has unresolved approvals; a paused conversation must be
// resolved before any new run.
func (s *Server) blockedByPendingApproval(w http.ResponseWriter, ctx context.Context, convID, userID string) bool {
	if s.deps.Pending == nil {
		return false
	}
	pending, err := s.deps.Pending.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not check pending approvals")
		return true
	}
	if len(pending) > 0 {
		writeError(w, http.StatusConflict, "approval_pending", "resolve the pending approval first")
		return true
	}
	return false
}

// knownRows builds the skip set of already-stored message payloads.
func knownRows(msgs []storage.Message) map[string]bool {
	known := make(map[string]bool, len(msgs))
	for _, row := range msgs {
		known[string(row.Data)] = true
	}
	return known
}

// runTurn streams one model turn to w: loads the caller's tools, opens the
// SSE sink, runs the agent, and settles the outcome (persist + done/error).
// Callers validate, prepare history/spec/parts, and persist the prompt;
// known holds the payloads of rows already in the store so history replay
// rows are not duplicated. The sink opens only after those steps so
// failures stay plain HTTP errors.
func (s *Server) runTurn(ctx context.Context, userID, convID string, spec chat.RunSpec, history []model.Message, prompt string, parts []model.Part, known map[string]bool, w http.ResponseWriter) {
	spec, history, compacted := s.maybeCompact(ctx, userID, convID, spec, history)
	// Tools load before the SSE sink goes out so failures can still be
	// plain HTTP errors. One broken config skips that tool only (logged by
	// DecodeConfigs's returned error).
	tools, err := s.userTools(ctx, userID, spec.RagEnabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load tools")
		return
	}
	sink, ok := newSSESink(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}
	if compacted {
		_ = sink.event("meta", map[string]any{"type": "compacted"})
	}
	var sources []rag.Scored
	outcome, err := s.deps.Agent.Run(ctx, chat.Deps{
		UserID:         userID,
		ConversationID: convID,
		Search:         s.searchDeps(userID, convID, &sources),
		SaveMemory:     s.saveMemoryFunc(userID),
	}, history, prompt, parts, sink, tools, spec)
	if err != nil {
		s.persistFailure(ctx, userID, convID, spec, err, sink)
		return
	}
	emitSources(sink, sources)
	s.settleRun(ctx, userID, convID, spec, outcome, sink, known, sourceJSON(sources))
}

// maybeCompact trims a hot history into its recent window and folds the
// older span's summary into the run instructions (ephemeral: history rows
// and role alternation are untouched). The summarizer spend is metered as
// a `compaction` usage event priced from the rate table. Fail-open: a nil
// compactor, a cool history, or a model failure returns inputs unchanged.
// The third return tells callers whether a summary landed, so they can
// announce it on the stream.
func (s *Server) maybeCompact(ctx context.Context, userID, convID string, spec chat.RunSpec, history []model.Message) (chat.RunSpec, []model.Message, bool) {
	if s.deps.Compactor == nil {
		return spec, history, false
	}
	recent, summary, cusage, did, _ := s.deps.Compactor.Compact(ctx, history)
	if !did {
		return spec, history, false
	}
	if spec.SystemPrompt == "" {
		spec.SystemPrompt = chat.DefaultSystemPrompt
	}
	spec.SystemPrompt += "\n\n[Compacted earlier context — summarized, not verbatim]\n" + summary
	if cusage.InputTokens+cusage.OutputTokens > 0 {
		cm := s.deps.Compactor.ModelName()
		ccost := s.deps.Rates.RatesFor(cm).ChatCostMicros(cusage.InputTokens, cusage.OutputTokens)
		_ = s.deps.Usage.Add(ctx, storage.UsageEvent{
			UserID: userID, Kind: "compaction", Model: cm, ConversationID: &convID,
			InputTokens: cusage.InputTokens, OutputTokens: cusage.OutputTokens,
			Estimated: false, CostMicros: ccost,
		})
	}
	return spec, recent, true
}

// emitSources tells the client which document chunks the run retrieved, so
// the answer can render its source cards. Nothing emits when the run never
// searched.
func emitSources(sink *sseSink, sources []rag.Scored) {
	if len(sources) == 0 {
		return
	}
	_ = sink.event("sources", map[string]any{"sources": sourceRows(sources)})
}

// sourceJSON renders retrieved chunks as the persisted per-message copy
// (nil when the run never searched — the store defaults to []).
func sourceJSON(sources []rag.Scored) json.RawMessage {
	if len(sources) == 0 {
		return nil
	}
	raw, err := json.Marshal(sourceRows(sources))
	if err != nil {
		return nil
	}
	return raw
}

// sourceRows renders retrieved chunks as client source rows: the SSE payload
// and the persisted per-message copy share this shape exactly.
func sourceRows(sources []rag.Scored) []map[string]any {
	rows := make([]map[string]any, len(sources))
	for i, sc := range sources {
		snippet := rag.DisplayText(sc)
		if len(snippet) > 160 {
			snippet = strings.TrimSpace(snippet[:160]) + "…"
		}
		rows[i] = map[string]any{
			"documentId": sc.Chunk.DocumentID,
			"title":      sc.Chunk.DocTitle,
			"heading":    sc.Chunk.Heading,
			"page":       sc.Chunk.Page,
			"snippet":    snippet,
			"score":      sc.Score,
		}
	}
	return rows
}

// runSpecFor resolves a conversation's model settings into a RunSpec: an
// unset model falls back to the server default (first catalog entry with a
// configured key), an empty system prompt to the built-in one (applied at
// agent build), and nil temperature to the provider default. Global user
// instructions prepend the prompt; the per-conversation prompt follows so
// it wins ties. Fail-open: instruction lookup errors behave as unset.
func (s *Server) runSpecFor(ctx context.Context, conv storage.Conversation) chat.RunSpec {
	spec := chat.RunSpec{Model: conv.Model, Temperature: conv.Temperature, SystemPrompt: conv.SystemPrompt, RagEnabled: conv.RagEnabled}
	if spec.Model == "" {
		spec.Model = chat.DefaultModel(s.deps.ModelKeys).ID
	}
	var prefix strings.Builder
	if s.deps.Memories != nil {
		if mems, err := s.deps.Memories.List(ctx, conv.UserID); err == nil && len(mems) > 0 {
			prefix.WriteString("[User memory]\n")
			for _, m := range mems {
				prefix.WriteString("- ")
				prefix.WriteString(m.Content)
				prefix.WriteString("\n")
			}
			prefix.WriteString("\n")
		}
	}
	if s.deps.Profiles != nil {
		if global, err := s.deps.Profiles.Instructions(ctx, conv.UserID); err == nil && global != "" {
			prefix.WriteString("[User preferences]\n")
			prefix.WriteString(global)
			prefix.WriteString("\n\n")
		}
	}
	if prefix.Len() > 0 {
		spec.SystemPrompt = prefix.String() + spec.SystemPrompt
	}
	return spec
}

func (s *Server) saveMemoryFunc(userID string) func(ctx context.Context, fact string) error {
	if s.deps.Memories == nil {
		return nil
	}
	return func(ctx context.Context, fact string) error {
		_, err := s.deps.Memories.Create(ctx, userID, fact)
		return err
	}
}

// userTools builds the caller's enabled golem tools plus the built-in
// document search when RAG is enabled globally and for this conversation.
// A nil Tools store (tests without the tools feature) means user tools
// only. Every run path (send, edit, regenerate, approvals resume) loads
// tools through here.
func (s *Server) userTools(ctx context.Context, userID string, ragEnabled bool) ([]tool.Tool[chat.Deps], error) {
	var tools []tool.Tool[chat.Deps]
	if s.deps.Tools != nil {
		rows, err := s.deps.Tools.ListEnabled(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list enabled tools: %w", err)
		}
		cfgs, derr := chat.DecodeConfigs(rows)
		if derr != nil {
			s.deps.Log.Error("skip broken tool config", "err", derr)
		}
		tools, err = chat.BuildTools(cfgs, s.toolEnv())
		if err != nil {
			return nil, fmt.Errorf("build tools: %w", err)
		}
	}
	if s.deps.RagSearch != nil && ragEnabled {
		tools = append(tools, chat.SearchTool())
	}
	if s.deps.WebSearch != nil {
		tools = append(tools, chat.WebSearchTool(s.deps.WebSearch, s.deps.Cfg.WebSearchRequireApproval))
	}
	if s.deps.Memories != nil {
		tools = append(tools, chat.RememberTool())
	}
	return tools, nil
}

// searchDeps wraps the raw RagSearch with embedding metering (every query
// embed is an exact-token `embedding` usage event priced from the rate
// table), optional rerank generation metering, and per-run source collection
// for the sources SSE event. Nil when RAG is disabled.
func (s *Server) searchDeps(userID, convID string, sources *[]rag.Scored) chat.SearchFunc {
	if s.deps.RagSearch == nil {
		return nil
	}
	return func(ctx context.Context, query string, k int, docIDs []string) ([]rag.Scored, int, error) {
		scored, rep, err := s.deps.RagSearch(ctx, userID, query, k, docIDs)
		if err != nil {
			return nil, 0, err
		}
		offset := len(*sources)
		*sources = append(*sources, scored...)
		if rep.Embed.InputTokens > 0 {
			model := s.deps.Cfg.RAG.EmbeddingModel
			_ = s.deps.Usage.Add(ctx, storage.UsageEvent{
				UserID: userID, Kind: "embedding", Model: model,
				ConversationID: &convID,
				InputTokens:    rep.Embed.InputTokens,
				Estimated:      rep.Embed.Estimated,
				CostMicros:     s.deps.Rates.RatesFor(model).ChatCostMicros(rep.Embed.InputTokens, 0),
			})
		}
		if rep.Reranked && rep.RerankIn+rep.RerankOut > 0 {
			cost := s.deps.Rates.RatesFor(rep.RerankModel).ChatCostMicros(rep.RerankIn, rep.RerankOut)
			_ = s.deps.Usage.Add(ctx, storage.UsageEvent{
				UserID: userID, Kind: "rerank", Model: rep.RerankModel, ConversationID: &convID,
				InputTokens: rep.RerankIn, OutputTokens: rep.RerankOut, Estimated: false, CostMicros: cost,
			})
		}
		return scored, offset, nil
	}
}

// toolEnv derives the execution bounds from config, filling the unset
// (zero-valued test config) fields with the package defaults.
func (s *Server) toolEnv() chat.ToolEnv {
	env := chat.ToolEnv{
		HTTPTimeout:       s.deps.Cfg.ToolHTTPTimeout,
		HTTPMaxBytes:      s.deps.Cfg.ToolHTTPMaxBytes,
		ResultMaxBytes:    s.deps.Cfg.ToolResultMaxBytes,
		AllowPrivateHosts: s.deps.Cfg.ToolAllowPrivateHosts,
	}
	if env.HTTPTimeout <= 0 {
		def := chat.DefaultToolEnv()
		env.HTTPTimeout = def.HTTPTimeout
		env.HTTPMaxBytes = def.HTTPMaxBytes
		env.ResultMaxBytes = def.ResultMaxBytes
	}
	return env
}

// settleRun dispatches a finished run: approval pauses park the conversation,
// everything else finishes normally. sources carries the run's retrieved
// chunks for the concluding answer row (nil when it never searched).
func (s *Server) settleRun(ctx context.Context, userID, convID string, spec chat.RunSpec, outcome chat.Outcome, sink *sseSink, known map[string]bool, sources json.RawMessage) {
	newFrom := replayedPrefixLen(outcome.Messages)
	if len(outcome.Pending) > 0 {
		s.pauseRun(ctx, userID, convID, spec, outcome, sink, known, newFrom)
		return
	}
	s.finishRun(ctx, userID, convID, spec, outcome, sink, known, newFrom, sources)
}

// replayedPrefixLen reports how many leading messages of a streamed run's
// Messages are replay (history plus the fresh prompt): everything up to and
// including the last user message. Golem returns the full conversation, so
// those rows are already in the store and only genuinely new rows (after
// that point) may add rows. 0 means "no positional constraint".
func replayedPrefixLen(msgs []model.Message) int {
	last := -1
	for i, m := range msgs {
		if m.Role == model.RoleUser {
			last = i
		}
	}
	return last + 1
}

// pauseRun persists the paused conversation (history through the unanswered
// tool call, usage, pending calls) and tells the client what needs a
// decision. No done event follows — the stream ends waiting for approvals.
func (s *Server) pauseRun(ctx context.Context, userID, convID string, spec chat.RunSpec, outcome chat.Outcome, sink *sseSink, known map[string]bool, newFrom int) {
	idMap, err := s.persistRunMessages(ctx, userID, convID, spec.Model, outcome.Messages, outcome.Usage, outcome.Requests, false, known, newFrom, nil)
	if err != nil {
		s.deps.Log.Error("persist paused messages", "err", err)
	}
	if err := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, spec.Model, outcome.Usage, outcome.Requests, s.deps.Rates)); err != nil {
		s.deps.Log.Error("record usage", "err", err)
	}
	calls := make([]storage.PendingToolCall, 0, len(outcome.Pending))
	announced := make([]chat.PendingApproval, 0, len(outcome.Pending))
	for _, p := range outcome.Pending {
		// Reference the persisted (rewritten, conversation-unique) call ID.
		callID := p.CallID
		if mapped, ok := idMap[p.CallID]; ok {
			callID = mapped
			p.CallID = mapped
		}
		calls = append(calls, storage.PendingToolCall{
			CallID: callID, ConversationID: convID, UserID: userID,
			ToolName: p.ToolName, Args: p.Args, Reason: p.Reason, Status: "pending",
		})
		announced = append(announced, p)
	}
	if err := s.deps.Pending.Add(ctx, calls); err != nil {
		s.deps.Log.Error("persist pending calls", "err", err)
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	_ = sink.event("approval_request", map[string]any{"calls": announced})
}

// persistFailure uses golem v0.7.1 partial evidence: whatever completed is
// kept, marked truncated. Every partial message persists so the next turn's
// history — including any completed tool exchanges — replays faithfully.
func (s *Server) persistFailure(ctx context.Context, userID, convID string, spec chat.RunSpec, runErr error, sink *sseSink) {
	clientGone := errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)
	var runError *golem.RunError
	_ = errors.As(runErr, &runError)

	var partial *golem.PartialResult
	if runError != nil {
		partial = runError.Partial
	}
	if partial != nil && len(partial.Messages) > 0 {
		if _, err := s.persistRunMessages(ctx, userID, convID, spec.Model, partial.Messages, partial.Usage, partial.Requests, true, nil, replayedPrefixLen(partial.Messages), nil); err != nil {
			s.deps.Log.Error("persist partial", "err", err)
		}
		_ = s.deps.Usage.Add(ctx, usageEventFor(userID, convID, spec.Model, partial.Usage, partial.Requests, s.deps.Rates))
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	if clientGone {
		return // client is gone; quiet stop for user-initiated cancels
	}
	stage := "model"
	if runError != nil {
		stage = string(runError.Stage)
	}
	_ = sink.event("error", map[string]string{"stage": stage, "message": userMessage(runErr)})
	s.deps.Log.Error("chat run failed", "err", runErr)
}

// persistRunMessages stores every run-produced message as its own row. The
// system message and the fresh user prompt stay out — the prompt row was
// persisted by the handler, and replaying either would duplicate them.
// Tool-call and tool-result rows ARE persisted: the next turn's history and
// any deferred resume need them. Usage and cost land on the last assistant
// row only.
//
// Two skip rules keep stored rows from duplicating. Rows in the replayed
// prefix (index < newFrom — history plus the fresh prompt, per golem's
// "Messages is the full conversation" contract) are positional and always
// skipped; newFrom <= 0 disables that rule. Beyond the prefix, a row whose
// exact payload matches a stored row (skip map) is a resumed run's replay
// and is skipped too — the approvals resume path relies on that, since a
// resumed run's Messages start with the paused history verbatim.
//
// Provider-synthesized call IDs (Gemini's call-1, call-2, …) restart every
// request, so they collide across runs of one conversation — golem keys
// answered-ness by call ID alone. Each persisted call therefore gets a
// conversation-unique ID; the returned map links the run's raw IDs to the
// persisted ones (the approval pause needs it to key pending rows). A tool
// result is matched to the most recent unanswered raw ID, mirroring the
// emission order golem guarantees.
func (s *Server) persistRunMessages(ctx context.Context, userID, convID, modelStr string, msgs []model.Message, usage model.Usage, requests int, truncated bool, skip map[string]bool, newFrom int, sources json.RawMessage) (map[string]string, error) {
	lastAssistant := -1
	for i, m := range msgs {
		if m.Role == model.RoleAssistant {
			lastAssistant = i
		}
	}
	idMap := map[string]string{}
	unanswered := map[string]string{} // raw call ID → persisted call ID
	for i, m := range msgs {
		if m.Role != model.RoleAssistant && m.Role != model.RoleTool {
			continue
		}
		// Replay detection must run BEFORE rewriting: a resumed run's
		// Messages start with the paused history verbatim, and those rows
		// already exist. Rewriting first would mint fresh IDs for them.
		if newFrom > 0 {
			// Streamed run: everything before the fresh prompt is replay.
			if i < newFrom {
				continue
			}
		} else if skip[string(mustJSON(m))] {
			// Deferred resume: the paused history replays verbatim from
			// index 0, so payload equality is the replay test.
			continue
		}
		if m.Role == model.RoleAssistant && len(m.ToolCalls) > 0 {
			m = cloneMessage(m)
			for j, call := range m.ToolCalls {
				prefix := convID
				if len(prefix) > 8 {
					prefix = prefix[:8]
				}
				newID := prefix + "-call-" + strings.ToLower(randomHex())
				if call.ID != "" {
					unanswered[call.ID] = newID
					idMap[call.ID] = newID
				}
				m.ToolCalls[j].ID = newID
			}
		}
		if m.Role == model.RoleTool && m.ToolCallID != "" {
			if newID, ok := unanswered[m.ToolCallID]; ok {
				m = cloneMessage(m)
				m.ToolCallID = newID
				delete(unanswered, rawOf(idMap, newID))
			}
		}
		row := storage.Message{
			ConversationID: convID, UserID: userID,
			Role: string(m.Role), Content: m.Content, Data: mustJSON(m),
			Truncated: truncated,
		}
		if i == lastAssistant {
			row.InputTokens = usage.InputTokens
			row.OutputTokens = usage.OutputTokens
			row.Requests = requests
			row.CostMicros = s.deps.Rates.ChatCostMicrosFor(modelStr, usage.InputTokens, usage.OutputTokens)
			row.Model = modelStr
			// The run's citations belong to its concluding answer: the same
			// rows the sources event carried, so history reloads render the
			// same cards the live session showed.
			row.Sources = sources
		}
		if err := s.deps.Msgs.Add(ctx, row); err != nil {
			return idMap, err
		}
	}
	return idMap, nil
}

func cloneMessage(m model.Message) model.Message {
	out := m
	out.ToolCalls = append([]model.ToolCall(nil), m.ToolCalls...)
	return out
}

// randomHex returns 16 random hex chars for a persisted call-ID suffix.
func randomHex() string {
	var b [8]byte
	_, _ = cryptorand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// rawOf finds the raw ID mapped to a persisted ID (used to clear the
// answered marker once its result is stored).
func rawOf(idMap map[string]string, persisted string) string {
	for raw, mapped := range idMap {
		if mapped == persisted {
			return raw
		}
	}
	return ""
}

func (s *Server) finishRun(ctx context.Context, userID, convID string, spec chat.RunSpec, outcome chat.Outcome, sink *sseSink, known map[string]bool, newFrom int, sources json.RawMessage) {
	if _, err := s.persistRunMessages(ctx, userID, convID, spec.Model, outcome.Messages, outcome.Usage, outcome.Requests, false, known, newFrom, sources); err != nil {
		s.deps.Log.Error("persist assistant messages", "err", err)
	}
	if err := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, spec.Model, outcome.Usage, outcome.Requests, s.deps.Rates)); err != nil {
		s.deps.Log.Error("record usage", "err", err)
	}
	_ = s.deps.Convos.Touch(ctx, convID)

	// The stored message id is needed for `done`; fetch the latest assistant row.
	msgID := ""
	if msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID); err == nil && len(msgs) > 0 {
		msgID = msgs[len(msgs)-1].ID
	}
	cost := s.deps.Rates.ChatCostMicrosFor(spec.Model, outcome.Usage.InputTokens, outcome.Usage.OutputTokens)
	_ = sink.event("done", map[string]any{
		"messageId":    msgID,
		"inputTokens":  outcome.Usage.InputTokens,
		"outputTokens": outcome.Usage.OutputTokens,
		"requests":     outcome.Requests,
		"costUsd":      math.Round(float64(cost)/10) / 1e5, // 5 decimal places
		"model":        spec.Model,
	})
}

// fullHistory decodes every stored row.
func fullHistory(msgs []storage.Message) []model.Message {
	var history []model.Message
	for _, m := range msgs {
		var mm model.Message
		if err := json.Unmarshal(m.Data, &mm); err != nil {
			continue // damaged row: skip rather than fail the run
		}
		history = append(history, mm)
	}
	return history
}

// pausedRunHistory decodes the rows of the paused run: everything from the
// last user message on. That is golem's deferred-resume contract ("history
// is the paused run's Result.Messages") — and a hard requirement with
// providers that synthesize call IDs per request (Gemini's call-1, call-2,
// … repeat across runs, so a full-conversation history would let the resume
// see the new call as already answered).
func pausedRunHistory(msgs []storage.Message) []model.Message {
	decoded := fullHistory(msgs)
	start := 0
	for i, m := range decoded {
		if m.Role == model.RoleUser {
			start = i
		}
	}
	return decoded[start:]
}

func usageEventFor(userID, convID, mdl string, usage model.Usage, requests int, rates cost.Table) storage.UsageEvent {
	cid := convID
	return storage.UsageEvent{
		UserID: userID, Kind: "chat", Model: mdl, ConversationID: &cid,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		Requests: requests, CostMicros: rates.ChatCostMicrosFor(mdl, usage.InputTokens, usage.OutputTokens),
	}
}

// historyFrom decodes the first upto stored rows as replay history.
// Damaged rows are skipped rather than failing the run.
func historyFrom(msgs []storage.Message, upto int) []model.Message {
	if upto <= 0 {
		return nil
	}
	var history []model.Message
	for _, m := range msgs[:upto] {
		var mm model.Message
		if err := json.Unmarshal(m.Data, &mm); err != nil {
			continue
		}
		history = append(history, mm)
	}
	return history
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func userMessage(err error) string {
	var runErr *golem.RunError
	if errors.As(err, &runErr) {
		return "the model run failed at the " + string(runErr.Stage) + " stage"
	}
	return "the model run failed"
}
