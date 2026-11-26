package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

const maxSummaryChars = 8000

type summaryResponse struct {
	Summary         string `json:"summary"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
	CoveredMessages int    `json:"coveredMessages"`
	TotalMessages   int    `json:"totalMessages"`
	ThresholdTokens int    `json:"thresholdTokens"`
	KeepRecent      int    `json:"keepRecent"`
}

func (s *Server) summaryLimits() (threshold, keepRecent int) {
	threshold, keepRecent = 40000, 10
	if s.deps.Compactor != nil {
		threshold = s.deps.Compactor.Threshold()
		keepRecent = s.deps.Compactor.KeepRecent()
	} else if s.deps.Cfg.RAG.CompactionThreshold > 0 {
		threshold = s.deps.Cfg.RAG.CompactionThreshold
		keepRecent = s.deps.Cfg.RAG.CompactionKeepRecent
	}
	return threshold, keepRecent
}

// loadHistory loads the active path as model messages for summary operations.
func (s *Server) loadHistory(ctx context.Context, convID, userID string) (storage.Conversation, []model.Message, error) {
	conv, err := s.deps.Convos.ByID(ctx, convID, userID)
	if err != nil {
		return storage.Conversation{}, nil, err
	}
	rows, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		return storage.Conversation{}, nil, err
	}
	leaf, err := s.resolveActiveLeaf(ctx, conv, rows)
	if err != nil {
		return storage.Conversation{}, nil, err
	}
	path, err := s.pathToLeaf(ctx, convID, userID, leaf)
	if err != nil {
		return storage.Conversation{}, nil, err
	}
	return conv, historyFrom(path, len(path)), nil
}

func (s *Server) handleGetSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	_, history, err := s.loadHistory(r.Context(), convID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	sum, err := s.deps.Convos.Summary(r.Context(), convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load summary")
		return
	}
	threshold, keep := s.summaryLimits()
	resp := summaryResponse{
		Summary: sum.Text, CoveredMessages: sum.Covers, TotalMessages: len(history),
		ThresholdTokens: threshold, KeepRecent: keep,
	}
	if sum.UpdatedAt != nil {
		resp.UpdatedAt = sum.UpdatedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePutSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	var req struct {
		Summary string `json:"summary"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if len([]rune(req.Summary)) > maxSummaryChars {
		writeError(w, http.StatusBadRequest, "too_long", "summary is too long")
		return
	}
	existing, err := s.deps.Convos.Summary(r.Context(), convID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load summary")
		return
	}
	covers := existing.Covers
	if req.Summary == "" {
		covers = 0
	}
	if err := s.deps.Convos.SetSummary(r.Context(), convID, userID, req.Summary, covers); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not save summary")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	if err := s.deps.Convos.ClearSummary(r.Context(), convID, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not clear summary")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCompactNow(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	if s.deps.Compactor == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "compaction not configured")
		return
	}
	if s.runActive(convID) {
		writeError(w, http.StatusConflict, "run_active", "finish the active run before compacting")
		return
	}
	if s.deps.Pending != nil {
		if pending, err := s.deps.Pending.ForConversation(r.Context(), convID, userID); err == nil && len(pending) > 0 {
			writeError(w, http.StatusConflict, "approval_pending", "resolve pending approvals before compacting")
			return
		}
	}
	_, history, err := s.loadHistory(r.Context(), convID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	_, keep := s.summaryLimits()
	if len(history) <= keep {
		writeError(w, http.StatusBadRequest, "not_enough_history", "not enough history to summarize")
		return
	}
	summary, covered, usage, ok := s.deps.Compactor.SummarizeNow(r.Context(), history, keep)
	if !ok {
		writeError(w, http.StatusBadGateway, "summarize_failed", "could not summarize this conversation")
		return
	}
	if err := s.deps.Convos.SetSummary(r.Context(), convID, userID, summary, covered); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not save summary")
		return
	}
	s.meterCompaction(r.Context(), userID, convID, usage)
	threshold, _ := s.summaryLimits()
	writeJSON(w, http.StatusOK, summaryResponse{
		Summary: summary, CoveredMessages: covered, TotalMessages: len(history),
		ThresholdTokens: threshold, KeepRecent: keep,
	})
}
