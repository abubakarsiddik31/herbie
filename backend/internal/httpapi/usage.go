package httpapi

import (
	"math"
	"net/http"
	"strconv"
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
	writeJSON(w, http.StatusOK, map[string]any{"totals": totals, "daily": dailies})
}
