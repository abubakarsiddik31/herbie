package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

const (
	minCompareModels = 2
	maxCompareModels = 4
)

type compareResult struct {
	Model        string  `json:"model"`
	MessageID    string  `json:"messageId,omitempty"`
	Content      string  `json:"content,omitempty"`
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
	Error        string  `json:"error,omitempty"`
}

type compareResponse struct {
	PromptMessageID string          `json:"promptMessageId"`
	Results         []compareResult `json:"results"`
}

// discardSink ignores streamed progress: compare runs to completion and
// returns the final answers as JSON.
type discardSink struct{}

func (discardSink) Delta(string) error   { return nil }
func (discardSink) ModelStart()          {}
func (discardSink) ModelEnd(int, int)    {}
func (discardSink) ToolStart(string)     {}
func (discardSink) ToolEnd(string, bool) {}

// handleCompare runs one prompt against 2–4 models, persisting one assistant
// sibling per model under a single user row. The client picks a winner by
// activating that message through the existing branch endpoint.
func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	ctx := r.Context()
	var req struct {
		Content         string       `json:"content"`
		Images          []imageInput `json:"images"`
		ClientMessageID string       `json:"clientMessageId"`
		Models          []string     `json:"models"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	req.Content = storage.SanitizeText(req.Content)
	if strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	if len(req.Models) < minCompareModels || len(req.Models) > maxCompareModels {
		writeError(w, http.StatusBadRequest, "bad_request", "choose 2–4 models to compare")
		return
	}
	seen := map[string]bool{}
	for _, id := range req.Models {
		if id == chat.AutoModelID {
			writeError(w, http.StatusBadRequest, "bad_request", "the auto alias cannot be compared")
			return
		}
		if _, ok := chat.FindModel(id); !ok {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("model %q cannot be compared", id))
			return
		}
		if seen[id] {
			writeError(w, http.StatusBadRequest, "bad_request", "models must be distinct")
			return
		}
		seen[id] = true
	}
	parts, err := decodeImages(req.Images)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	req.ClientMessageID = strings.TrimSpace(storage.SanitizeText(req.ClientMessageID))
	if strings.ContainsRune(req.ClientMessageID, 0) || len([]rune(req.ClientMessageID)) > 100 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid clientMessageId")
		return
	}

	conv, err := s.deps.Convos.ByID(ctx, convID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}
	if blockedByBotConversation(w, conv) {
		return
	}
	if s.blockedByPendingApproval(w, ctx, convID, userID) {
		return
	}
	if req.ClientMessageID != "" {
		if _, derr := s.deps.Msgs.ByClientMessageID(ctx, convID, userID, req.ClientMessageID); derr == nil {
			writeError(w, http.StatusConflict, "duplicate_send", "this message was already sent")
			return
		} else if !errors.Is(derr, storage.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "internal", "could not check message")
			return
		}
	}
	release, ok := s.beginRun(convID)
	if !ok {
		s.blockedByActiveRun(w, convID)
		return
	}
	defer release()

	spec := s.runSpecFor(ctx, conv)
	if n, err := s.deps.Convos.CountMessages(ctx, convID, userID); err == nil && n == 0 {
		_ = s.deps.Convos.SetHeuristicTitle(ctx, convID, userID, deriveConversationTitle(req.Content))
	}
	rows, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	leaf, err := s.resolveActiveLeaf(ctx, conv, rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	path, err := s.pathToLeaf(ctx, convID, userID, leaf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	history := historyFrom(path, len(path))
	spec, history, _ = s.maybeCompact(ctx, userID, convID, spec, history)
	tools, err := s.userTools(ctx, userID, true, s.botFor(ctx, userID, conv.BotID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load tools")
		return
	}
	s.unarchiveForTurn(ctx, conv)
	promptID, err := s.deps.Msgs.Add(ctx, storage.Message{
		ConversationID: convID, UserID: userID, Role: string(model.RoleUser),
		ClientMessageID: req.ClientMessageID,
		Content:         req.Content,
		Data:            mustJSON(model.Message{Role: model.RoleUser, Content: req.Content, Parts: parts}),
		ParentID:        leaf,
	})
	if err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate_send", "this message was already sent")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not store message")
		return
	}
	if err := s.deps.Convos.SetActiveLeaf(ctx, convID, userID, promptID); err != nil {
		s.deps.Log.Error("compare set active leaf", "err", err)
	}
	known := knownRows(path)
	var sources []rag.Scored
	var sourcesMu sync.Mutex
	deps := s.buildDeps(userID, convID, projectScope(conv), &sources, &sourcesMu, nil)
	out := compareResponse{PromptMessageID: promptID}
	for _, modelID := range req.Models {
		runSpec := spec
		runSpec.Model = modelID
		outcome, runErr := s.agentFor(ctx, userID).Run(ctx, deps, history, req.Content, parts, discardSink{}, tools, runSpec)
		if runErr != nil {
			if ctx.Err() != nil {
				writeError(w, http.StatusRequestTimeout, "cancelled", "compare cancelled")
				return
			}
			out.Results = append(out.Results, compareResult{Model: modelID, Error: userMessage(runErr)})
			continue
		}
		if len(outcome.Pending) > 0 {
			out.Results = append(out.Results, compareResult{Model: modelID, Error: "this model needs tool approval; retry it in a normal chat"})
			continue
		}
		res, perr := s.persistRunMessages(ctx, userID, convID, modelID, outcome.Messages, outcome.Usage, outcome.Requests, false, known, replayedPrefixLen(outcome.Messages), promptID, nil)
		if perr != nil {
			out.Results = append(out.Results, compareResult{Model: modelID, Error: "could not save the answer"})
			continue
		}
		_ = s.deps.Usage.Add(ctx, usageEventFor(userID, convID, modelID, outcome.Usage, outcome.Requests, s.deps.Rates))
		cost := s.deps.Rates.ChatCostMicrosFor(modelID, outcome.Usage.InputTokens, outcome.Usage.OutputTokens)
		out.Results = append(out.Results, compareResult{
			Model: modelID, MessageID: res.LastAssistantID, Content: outcome.Output,
			InputTokens: outcome.Usage.InputTokens, OutputTokens: outcome.Usage.OutputTokens,
			CostUSD: float64(cost) / 1_000_000,
		})
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	s.maybeAutotitle(userID, convID)
	writeJSON(w, http.StatusOK, out)
}
