package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// The handler depends on store interfaces, not concrete *storage.* types,
// so its tests run offline with in-memory fakes; *storage.Conversations,
// *storage.Messages, and *storage.Usage satisfy them implicitly.
// ConvoStore is the full union the chat and CRUD routes need; the
// var _ ConvoStore = (*storage.Conversations)(nil) below pins the
// production wiring at compile time.
type ConvoStore interface {
	Create(ctx context.Context, userID, title string) (storage.Conversation, error)
	List(ctx context.Context, userID string) ([]storage.Conversation, error)
	ByID(ctx context.Context, id, userID string) (storage.Conversation, error)
	SetTitle(ctx context.Context, id, userID, title string) error
	Touch(ctx context.Context, id string) error
	Delete(ctx context.Context, id, userID string) error
	CountMessages(ctx context.Context, id, userID string) (int, error)
}

var _ ConvoStore = (*storage.Conversations)(nil)

type MsgStore interface {
	Add(ctx context.Context, msg storage.Message) error
	ForConversation(ctx context.Context, convID, userID string) ([]storage.Message, error)
}

type UsageStore interface {
	Add(ctx context.Context, e storage.UsageEvent) error
	Summary(ctx context.Context, userID string, days int) (storage.Summary, error)
}

const maxPromptChars = 8000

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Content == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	if len([]rune(req.Content)) > maxPromptChars {
		writeError(w, http.StatusBadRequest, "bad_request", "message too long")
		return
	}

	ctx := r.Context()
	if _, err := s.deps.Convos.ByID(ctx, convID, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}

	// Auto-title from the first user message.
	if n, err := s.deps.Convos.CountMessages(ctx, convID, userID); err == nil && n == 0 {
		_ = s.deps.Convos.SetTitle(ctx, convID, userID, truncateRunes(req.Content, 48))
	}

	// Persist the user message first, then build history from prior turns.
	if err := s.deps.Msgs.Add(ctx, storage.Message{
		ConversationID: convID, UserID: userID, Role: string(model.RoleUser),
		Content: req.Content,
		Data:    mustJSON(model.Message{Role: model.RoleUser, Content: req.Content}),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store message")
		return
	}
	msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	history := messagesToHistory(msgs)

	// SSE headers go out only after validation; from here the response is a stream.
	sink, ok := newSSESink(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}

	outcome, err := s.deps.Agent.Run(ctx, chat.Deps{UserID: userID, ConversationID: convID}, history, req.Content, sink)
	if err != nil {
		s.persistFailure(ctx, userID, convID, err, sink)
		return
	}
	s.finishRun(ctx, userID, convID, outcome, sink)
}

// persistFailure uses golem v0.7.1 partial evidence: whatever completed is
// kept, marked truncated.
func (s *Server) persistFailure(ctx context.Context, userID, convID string, runErr error, sink *sseSink) {
	clientGone := errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)
	var runError *golem.RunError
	_ = errors.As(runErr, &runError)

	var partial *golem.PartialResult
	if runError != nil {
		partial = runError.Partial
	}
	if partial != nil && len(partial.Messages) > 0 {
		content := partialTailText(partial.Messages)
		if content != "" {
			if err := s.deps.Msgs.Add(ctx, storage.Message{
				ConversationID: convID, UserID: userID, Role: string(model.RoleAssistant),
				Content: content, Data: mustJSON(partial.Messages[len(partial.Messages)-1]),
				InputTokens: partial.Usage.InputTokens, OutputTokens: partial.Usage.OutputTokens,
				Requests: partial.Requests, Truncated: true,
			}); err != nil {
				s.deps.Log.Error("persist partial", "err", err)
			}
			_ = s.deps.Usage.Add(ctx, usageEventFor(userID, convID, s.deps.Cfg.GeminiModel, partial.Usage, partial.Requests, s.deps.Rates))
		}
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

func (s *Server) finishRun(ctx context.Context, userID, convID string, outcome chat.Outcome, sink *sseSink) {
	cost := s.deps.Rates.ChatCostMicros(outcome.Usage.InputTokens, outcome.Usage.OutputTokens)
	var data []byte
	if len(outcome.Messages) > 0 {
		data = mustJSON(outcome.Messages[len(outcome.Messages)-1])
	}
	if err := s.deps.Msgs.Add(ctx, storage.Message{
		ConversationID: convID, UserID: userID, Role: string(model.RoleAssistant),
		Content: outcome.Output, Data: data,
		InputTokens: outcome.Usage.InputTokens, OutputTokens: outcome.Usage.OutputTokens,
		Requests: outcome.Requests, CostMicros: cost,
	}); err != nil {
		s.deps.Log.Error("persist assistant message", "err", err)
	}
	if err := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, s.deps.Cfg.GeminiModel, outcome.Usage, outcome.Requests, s.deps.Rates)); err != nil {
		s.deps.Log.Error("record usage", "err", err)
	}
	_ = s.deps.Convos.Touch(ctx, convID)

	// The stored message id is needed for `done`; fetch the latest assistant row.
	msgID := ""
	if msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID); err == nil && len(msgs) > 0 {
		msgID = msgs[len(msgs)-1].ID
	}
	_ = sink.event("done", map[string]any{
		"messageId":    msgID,
		"inputTokens":  outcome.Usage.InputTokens,
		"outputTokens": outcome.Usage.OutputTokens,
		"requests":     outcome.Requests,
		"costUsd":      math.Round(float64(cost)/10) / 1e5, // 5 decimal places
	})
}

func usageEventFor(userID, convID, mdl string, usage model.Usage, requests int, rates cost.Rates) storage.UsageEvent {
	cid := convID
	return storage.UsageEvent{
		UserID: userID, Kind: "chat", Model: mdl, ConversationID: &cid,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		Requests: requests, CostMicros: rates.ChatCostMicros(usage.InputTokens, usage.OutputTokens),
	}
}

// messagesToHistory decodes every stored message except the last (the fresh
// user prompt, which golem appends itself).
func messagesToHistory(msgs []storage.Message) []model.Message {
	if len(msgs) <= 1 {
		return nil
	}
	var history []model.Message
	for _, m := range msgs[:len(msgs)-1] {
		var mm model.Message
		if err := json.Unmarshal(m.Data, &mm); err != nil {
			continue // damaged row: skip rather than fail the run
		}
		history = append(history, mm)
	}
	return history
}

func partialTailText(msgs []model.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == model.RoleAssistant {
			return msgs[i].Content
		}
	}
	return ""
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
