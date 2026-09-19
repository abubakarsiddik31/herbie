package httpapi

import (
	"math"
	"net/http"
	"strconv"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

func microsToUSD(m int64) float64 { return math.Round(float64(m)/10) / 1e5 }

func (s *Server) handleUsageSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	days := 30
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 && d <= 365 {
		days = d
	}
	sum, err := s.deps.Usage.Summary(r.Context(), userID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load usage")
		return
	}
	type row struct {
		Kind         string  `json:"kind"`
		Model        string  `json:"model"`
		InputTokens  int     `json:"inputTokens"`
		OutputTokens int     `json:"outputTokens"`
		Requests     int     `json:"requests"`
		CostUsd      float64 `json:"costUsd"`
	}
	totals := make([]row, 0, len(sum.Totals))
	for _, t := range sum.Totals {
		totals = append(totals, row{t.Kind, t.Model, t.InputTokens, t.OutputTokens, t.Requests, microsToUSD(t.CostMicros)})
	}
	type daily struct {
		Day          string  `json:"day"`
		InputTokens  int     `json:"inputTokens"`
		OutputTokens int     `json:"outputTokens"`
		CostUsd      float64 `json:"costUsd"`
	}
	dailies := make([]daily, 0, len(sum.Daily))
	for _, d := range sum.Daily {
		dailies = append(dailies, daily{d.Day.Format("2006-01-02"), d.InputTokens, d.OutputTokens, microsToUSD(d.CostMicros)})
	}
	type docRow struct {
		DocumentID  string  `json:"documentId"`
		Filename    string  `json:"filename"`
		InputTokens int     `json:"inputTokens"`
		CostUsd     float64 `json:"costUsd"`
	}
	docs := make([]docRow, 0, len(sum.Documents))
	for _, d := range sum.Documents {
		docs = append(docs, docRow{d.DocumentID, d.Filename, d.InputTokens, microsToUSD(d.CostMicros)})
	}

	type searchItem struct {
		ID             string `json:"id"`
		Query          string `json:"query"`
		Kind           string `json:"kind"`
		Provider       string `json:"provider"`
		ResultsCount   int    `json:"resultsCount"`
		DurationMs     int64  `json:"durationMs"`
		CreatedAt      string `json:"createdAt"`
		ConversationID string `json:"conversationId,omitempty"`
	}
	recent := make([]searchItem, 0, len(sum.Searches.Recent))
	for _, qi := range sum.Searches.Recent {
		recent = append(recent, searchItem{
			ID:             qi.ID,
			Query:          qi.Query,
			Kind:           qi.Kind,
			Provider:       qi.Provider,
			ResultsCount:   qi.ResultsCount,
			DurationMs:     qi.DurationMs,
			CreatedAt:      qi.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			ConversationID: qi.ConversationID,
		})
	}
	byProvider := sum.Searches.ByProvider
	if byProvider == nil {
		byProvider = []storage.SearchProviderCount{}
	}
	dailySearches := sum.Searches.Daily
	if dailySearches == nil {
		dailySearches = []storage.SearchDailyCount{}
	}
	topQueries := sum.Searches.TopQueries
	if topQueries == nil {
		topQueries = []storage.SearchQueryCount{}
	}
	searches := map[string]any{
		"totalQueries": sum.Searches.TotalQueries,
		"webQueries":   sum.Searches.WebQueries,
		"docQueries":   sum.Searches.DocQueries,
		"byProvider":   byProvider,
		"daily":        dailySearches,
		"recent":       recent,
		"topQueries":   topQueries,
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"totals":    totals,
		"daily":     dailies,
		"documents": docs,
		"searches":  searches,
	})
}
