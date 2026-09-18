package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abubakarsiddik31/golem/model"
)

const (
	maxRerankCandidates   = 40
	maxRerankPassageChars = 600
)

var rerankSchema = json.RawMessage(`{"type":"object","properties":{"ranking":{"type":"array","items":{"type":"integer"}}},"required":["ranking"]}`)

const rerankSystem = `You rank document passages by relevance to a query. Reply with JSON only, no prose: {"ranking":[indices best-first, 0-based]}.`

// DisplayText is the text the model sees and the answer cites from: the
// expanded window when present, else the core chunk content.
func DisplayText(s Scored) string {
	if s.Context != "" {
		return s.Context
	}
	return s.Chunk.Content
}

// Ranker reorders retrieval candidates with a chat model. Fail-open: any
// model or parse failure returns the input order trimmed to topN.
type Ranker struct {
	model model.Model
	name  string
}

// NewRanker wraps m (resolved from chat.ModelRegistry by the HTTP layer).
func NewRanker(m model.Model, modelName string) *Ranker {
	return &Ranker{model: m, name: modelName}
}

// ModelName reports the chat model used, for usage metering.
func (r *Ranker) ModelName() string { return r.name }

// Rerank reorders in by relevance to query, trimmed to topN (<=0 keeps all).
func (r *Ranker) Rerank(ctx context.Context, query string, in []Scored, topN int) ([]Scored, model.Usage, error) {
	if len(in) == 0 {
		return nil, model.Usage{}, nil
	}
	n := len(in)
	if n > maxRerankCandidates {
		n = maxRerankCandidates
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nPassages:\n", query)
	for i := 0; i < n; i++ {
		p := DisplayText(in[i])
		if len(p) > maxRerankPassageChars {
			p = p[:maxRerankPassageChars] + "…"
		}
		fmt.Fprintf(&sb, "[%d] (%s) %s\n\n", i, in[i].Chunk.DocTitle, p)
	}
	sb.WriteString("Rank all passages best-first. JSON only.")
	resp, err := r.model.Generate(ctx, model.Request{
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: rerankSystem},
			{Role: model.RoleUser, Content: sb.String()},
		},
		OutputSchema: rerankSchema,
	})
	if err != nil {
		return trimScored(in, topN), resp.Usage, nil
	}
	order := parseRanking(resp.Message.Content, n)
	out := make([]Scored, 0, n)
	for _, idx := range order {
		out = append(out, in[idx])
	}
	return trimScored(out, topN), resp.Usage, nil
}

func trimScored(in []Scored, topN int) []Scored {
	if topN > 0 && len(in) > topN {
		return in[:topN]
	}
	return in
}

// parseRanking extracts a deduped, range-checked index order; garbage in
// yields the identity order (fail-open). Unranked indices append in
// original order so no candidate is ever lost.
func parseRanking(content string, n int) []int {
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return identityOrder(n)
	}
	var v struct {
		Ranking []int `json:"ranking"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &v); err != nil {
		return identityOrder(n)
	}
	seen := map[int]bool{}
	var out []int
	for _, idx := range v.Ranking {
		if idx < 0 || idx >= n || seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, idx)
	}
	for i := 0; i < n; i++ {
		if !seen[i] {
			out = append(out, i)
		}
	}
	return out
}

func identityOrder(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
