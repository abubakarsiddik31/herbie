package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
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

	release, ok := s.beginRun(convID)
	if !ok {
		s.blockedByActiveRun(w, convID)
		return
	}
	defer release()

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
		if s.deps.Audits != nil {
			action := "approval_granted"
			decisionStatus := "approved"
			if !d.Approved {
				action = "approval_rejected"
				decisionStatus = "rejected"
			}
			call := pendingByID[d.CallID]
			_ = s.deps.Audits.RecordToolAudit(ctx, storage.ToolAuditLog{
				UserID:       userID,
				CallerType:   "chat_agent",
				CallerID:     convID,
				ToolName:     call.ToolName,
				Action:       action,
				InputSummary: string(call.Args),
				Status:       decisionStatus,
				DurationMs:   0,
				CreatedAt:    time.Now(),
			})
		}
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

	history, startParentID, skip, err := s.resumeHistory(ctx, convID, userID, pending)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	tools, err := s.userTools(ctx, userID, spec.RagEnabled, s.botFor(ctx, userID, conv.BotID))
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
	var sourcesMu sync.Mutex
	spec, history, compacted := s.maybeCompact(ctx, userID, convID, spec, history)
	if compacted {
		_ = sink.event("meta", map[string]any{"type": "compacted"})
	}
	// Resume dedupe is payload equality scoped to the paused path: golem
	// merges resolution rows among the batch's already-executed results, so a
	// positional boundary can land mid-batch. Rows replayed verbatim are
	// skipped; resolutions and new answers carry this run's identity and are
	// persisted at the paused tail.
	outcome, err := s.deps.Agent.RunDeferred(ctx,
		s.buildDeps(userID, convID, &sources, &sourcesMu, s.botFor(ctx, userID, conv.BotID)),
		history, golem.DeferredResults{Approvals: resolutions},
		sink, tools, spec)
	if err != nil {
		s.persistFailure(ctx, userID, convID, spec, startParentID, skip, 0, err, sink)
		return
	}
	res, err := s.persistRunMessages(ctx, userID, convID, spec.Model, outcome.Messages, outcome.Usage, outcome.Requests, false, skip, 0, startParentID, sourceJSON(sources))
	if err != nil {
		s.deps.Log.Error("persist resumed messages", "err", err)
	}

	if len(outcome.Pending) > 0 {
		// The resumed run paused again: park the new pending calls and ask
		// again. Message rows are already persisted above.
		s.parkPending(ctx, userID, convID, spec, outcome, sink, res)
		return
	}
	s.finishResumed(ctx, userID, convID, spec, outcome, sink)
}

// resumeHistory rebuilds the paused run's history from the path ending at the
// paused tail (the parked call's message ID). Mixed batches execute some calls
// before pausing, so the tail is the last executed result — anchoring at the
// assistant call row would drop those results and make the resume look like it
// still owed every call. Legacy pending rows carry no message ID and fall back
// to the active-leaf path, which after the backfill is the same chain.
// startParentID is the tail new rows chain under; skip is the payload set of
// the paused path replayed back verbatim.
func (s *Server) resumeHistory(ctx context.Context, convID, userID string, pending []storage.PendingToolCall) ([]model.Message, string, map[string]bool, error) {
	parked := ""
	for _, p := range pending {
		if p.MessageID != "" {
			parked = p.MessageID
		}
	}
	if parked == "" {
		conv, err := s.deps.Convos.ByID(ctx, convID, userID)
		if err != nil {
			return nil, "", nil, err
		}
		rows, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
		if err != nil {
			return nil, "", nil, err
		}
		leaf, err := s.resolveActiveLeaf(ctx, conv, rows)
		if err != nil {
			return nil, "", nil, err
		}
		parked = leaf
	}
	path, err := s.pathToLeaf(ctx, convID, userID, parked)
	if err != nil {
		return nil, "", nil, err
	}
	if len(path) == 0 {
		// Stale pointer: recover through the active leaf instead.
		rows, rerr := s.deps.Msgs.ForConversation(ctx, convID, userID)
		if rerr != nil {
			return nil, "", nil, rerr
		}
		leafID := latestRowID(rows)
		if path, err = s.pathToLeaf(ctx, convID, userID, leafID); err != nil {
			return nil, "", nil, err
		}
	}
	startParentID := ""
	if len(path) > 0 {
		startParentID = path[len(path)-1].ID
	}
	return pausedRunHistory(path), startParentID, pausedKnownRows(path), nil
}

// parkPending persists the pending calls of a paused run and announces them.
// Unlike pauseRun it does not touch message rows — the caller persisted them.
func (s *Server) parkPending(ctx context.Context, userID, convID string, spec chat.RunSpec, outcome chat.Outcome, sink *sseSink, res persistRunResult) {
	if err := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, spec.Model, outcome.Usage, outcome.Requests, s.deps.Rates)); err != nil {
		s.deps.Log.Error("record usage", "err", err)
	}
	calls := make([]storage.PendingToolCall, 0, len(outcome.Pending))
	announced := make([]chat.PendingApproval, 0, len(outcome.Pending))
	for _, p := range outcome.Pending {
		if mapped, ok := res.IDMap[p.CallID]; ok {
			p.CallID = mapped
		}
		calls = append(calls, storage.PendingToolCall{
			CallID: p.CallID, MessageID: res.LastRowID, ConversationID: convID, UserID: userID,
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
	if conv, err := s.deps.Convos.ByID(ctx, convID, userID); err == nil && conv.ActiveLeafID != nil {
		msgID = *conv.ActiveLeafID
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
