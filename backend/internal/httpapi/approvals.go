package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type approvalDecision struct {
	CallID   string `json:"callId"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

// handleApprovals resolves the pending approval-gated calls of a paused
// conversation and resumes the run. The response is the same SSE stream as
// send-message: meta/delta frames, a final done, error on failure — and
// possibly another approval_request when the resumed run pauses again.
func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
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
	// The resume runs under the conversation's current model settings.
	spec := s.runSpecFor(ctx, conv)

	var req struct {
		Decisions []approvalDecision `json:"decisions"`
	}
	if err := decodeJSON(r, &req); err != nil || len(req.Decisions) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "decisions are required")
		return
	}

	pending, err := s.deps.Pending.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load pending approvals")
		return
	}
	if len(pending) == 0 {
		writeError(w, http.StatusBadRequest, "no_pending", "nothing is awaiting approval")
		return
	}

	// Every live pending call must be resolved exactly once.
	pendingByID := make(map[string]storage.PendingToolCall, len(pending))
	for _, c := range pending {
		pendingByID[c.CallID] = c
	}
	seen := map[string]bool{}
	resolutions := make(map[string]golem.Approval, len(req.Decisions))
	for _, d := range req.Decisions {
		if _, ok := pendingByID[d.CallID]; !ok || seen[d.CallID] {
			writeError(w, http.StatusBadRequest, "validation_error", "unknown or duplicated callId "+d.CallID)
			return
		}
		seen[d.CallID] = true
		resolutions[d.CallID] = golem.Approval{Approved: d.Approved, Reason: d.Reason}
	}
	if len(seen) != len(pendingByID) {
		writeError(w, http.StatusBadRequest, "validation_error", "every pending call needs a decision")
		return
	}

	status := map[string]string{}
	for _, d := range req.Decisions {
		if d.Approved {
			status[d.CallID] = "approved"
		} else {
			status[d.CallID] = "denied"
		}
	}
	// Rows flip before the run so a crash cannot re-ask a resolved call;
	// a failed resume is reportable, a re-ask is not.
	for callID, st := range status {
		if err := s.deps.Pending.SetStatus(ctx, userID, callID, st); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not record decision")
			return
		}
	}

	msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
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
	var sources []rag.Scored
	history := pausedRunHistory(msgs)
	spec, history, compacted := s.maybeCompact(ctx, userID, convID, spec, history)
	if compacted {
		_ = sink.event("meta", map[string]any{"type": "compacted"})
	}
	outcome, err := s.deps.Agent.RunDeferred(ctx,
		chat.Deps{UserID: userID, ConversationID: convID, Search: s.searchDeps(userID, convID, &sources), SaveMemory: s.saveMemoryFunc(userID)},
		history, golem.DeferredResults{Approvals: resolutions},
		sink, tools, spec)
	if err != nil {
		s.persistFailure(ctx, userID, convID, spec, err, sink)
		return
	}
	// The resume run's Messages replay the paused history; skip rows that
	// already exist (message JSON is stable across round-trips). newFrom 0
	// disables the positional rule — the known-payload map alone is correct
	// here because the paused history ends with an unanswered tool call.
	known := make(map[string]bool, len(msgs))
	for _, row := range msgs {
		known[string(row.Data)] = true
	}
	idMap, err := s.persistRunMessages(ctx, userID, convID, spec.Model, outcome.Messages, outcome.Usage, outcome.Requests, false, known, 0, sourceJSON(sources))
	if err != nil {
		s.deps.Log.Error("persist resumed messages", "err", err)
	}

	if len(outcome.Pending) > 0 {
		// The resumed run paused again: park the new pending calls and ask
		// again. Message rows are already persisted above.
		s.parkPending(ctx, userID, convID, spec, outcome, sink, idMap)
		return
	}
	s.finishResumed(ctx, userID, convID, spec, outcome, sink)
}

// parkPending persists the pending calls of a paused run and announces them.
// Unlike pauseRun it does not touch message rows — the caller persisted them.
func (s *Server) parkPending(ctx context.Context, userID, convID string, spec chat.RunSpec, outcome chat.Outcome, sink *sseSink, idMap map[string]string) {
	if err := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, spec.Model, outcome.Usage, outcome.Requests, s.deps.Rates)); err != nil {
		s.deps.Log.Error("record usage", "err", err)
	}
	calls := make([]storage.PendingToolCall, 0, len(outcome.Pending))
	announced := make([]chat.PendingApproval, 0, len(outcome.Pending))
	for _, p := range outcome.Pending {
		if mapped, ok := idMap[p.CallID]; ok {
			p.CallID = mapped
		}
		calls = append(calls, storage.PendingToolCall{
			CallID: p.CallID, ConversationID: convID, UserID: userID,
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

// finishResumed records usage and emits done without re-persisting messages.
func (s *Server) finishResumed(ctx context.Context, userID, convID string, spec chat.RunSpec, outcome chat.Outcome, sink *sseSink) {
	if err := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, spec.Model, outcome.Usage, outcome.Requests, s.deps.Rates)); err != nil {
		s.deps.Log.Error("record usage", "err", err)
	}
	_ = s.deps.Convos.Touch(ctx, convID)
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
		"costUsd":      math.Round(float64(cost)/10) / 1e5,
		"model":        spec.Model,
	})
}
