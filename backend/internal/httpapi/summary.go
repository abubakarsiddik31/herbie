package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
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

// pathRows loads the storage rows on the active path. Summary coverage is
// keyed by a message ID so branches and resumes never apply a stale prefix.
func (s *Server) pathRows(ctx context.Context, convID, userID string) ([]storage.Message, error) {
	conv, err := s.deps.Convos.ByID(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	leaf, err := s.resolveActiveLeaf(ctx, conv, rows)
	if err != nil {
		return nil, err
	}
	return s.pathToLeaf(ctx, convID, userID, leaf)
}

func coveredCount(path []storage.Message, throughID *string) int {
	if throughID == nil {
		return 0
	}
	idx := rowOnPath(path, *throughID)
	if idx < 0 {
		return 0
	}
	return idx + 1
}

func writeSummaryError(w http.ResponseWriter, err error, fallback string) {
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", fallback)
}

func (s *Server) handleGetSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	path, err := s.pathRows(r.Context(), convID, userID)
	if err != nil {
		writeSummaryError(w, err, "could not load conversation")
		return
	}
	sum, err := s.deps.Convos.Summary(r.Context(), convID, userID)
	if err != nil {
		writeSummaryError(w, err, "could not load summary")
		return
	}
	covered := coveredCount(path, sum.ThroughID)
	if covered > len(path) {
		covered = len(path)
	}
	threshold, keep := s.summaryLimits()
	resp := summaryResponse{
		Summary: sum.Text, CoveredMessages: covered, TotalMessages: len(path),
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
	text := storage.SanitizeText(req.Summary)
	if strings.ContainsRune(text, 0) {
		writeError(w, http.StatusBadRequest, "bad_request", "summary contains invalid characters")
		return
	}
	if len([]rune(text)) > maxSummaryChars {
		writeError(w, http.StatusBadRequest, "too_long", "summary is too long")
		return
	}
	existing, err := s.deps.Convos.Summary(r.Context(), convID, userID)
	if err != nil {
		writeSummaryError(w, err, "could not load summary")
		return
	}
	if err := s.deps.Convos.SetSummary(r.Context(), convID, userID, text, existing.ThroughID); err != nil {
		writeSummaryError(w, err, "could not save summary")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	if err := s.deps.Convos.ClearSummary(r.Context(), convID, userID); err != nil {
		writeSummaryError(w, err, "could not clear summary")
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
	if !s.allowAuth(w, r, "compact", userID) {
		return
	}
	// Hold the run slot so a send/edit/approval cannot start mid-compaction.
	release, ok := s.beginRun(convID)
	if !ok {
		writeError(w, http.StatusConflict, "run_active", "finish the active run before compacting")
		return
	}
	defer release()
	if s.deps.Pending != nil {
		if pending, err := s.deps.Pending.ForConversation(r.Context(), convID, userID); err == nil && len(pending) > 0 {
			writeError(w, http.StatusConflict, "approval_pending", "resolve pending approvals before compacting")
			return
		}
	}
	path, err := s.pathRows(r.Context(), convID, userID)
	if err != nil {
		writeSummaryError(w, err, "could not load conversation")
		return
	}
	history := historyFrom(path, len(path))
	_, keep := s.summaryLimits()
	if len(history) <= keep {
		writeError(w, http.StatusBadRequest, "not_enough_history", "not enough history to summarize")
		return
	}
	summary, covered, usage, ok := s.deps.Compactor.SummarizeNow(r.Context(), history, keep)
	s.meterCompaction(r.Context(), userID, convID, usage)
	if !ok || summary == "" || covered <= 0 || covered > len(path) {
		writeError(w, http.StatusBadGateway, "summarize_failed", "could not summarize this conversation")
		return
	}
	throughID := path[covered-1].ID
	if err := s.deps.Convos.SetSummary(r.Context(), convID, userID, summary, &throughID); err != nil {
		writeSummaryError(w, err, "could not save summary")
		return
	}
	threshold, _ := s.summaryLimits()
	writeJSON(w, http.StatusOK, summaryResponse{
		Summary: summary, CoveredMessages: covered, TotalMessages: len(history),
		ThresholdTokens: threshold, KeepRecent: keep,
	})
}
