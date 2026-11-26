package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// historyInputCap bounds the summarizer input; older text beyond it is
// dropped oldest-first (it is the least recent context).
const historyInputCap = 100000 // chars ≈ 25k tokens

// Compactor summarizes stale history into an instructions block when the
// decoded run history exceeds a token threshold. Summaries never enter
// history rows, so role alternation and replay are unaffected.
type Compactor struct {
	model         model.Model
	name          string
	threshold     int
	keepRecent    int
	summaryTokens int
}

// NewCompactor builds a compactor; non-positive keepRecent means 10,
// non-positive summaryTokens means 800.
func NewCompactor(m model.Model, modelName string, thresholdTokens, keepRecent, summaryTokens int) *Compactor {
	if keepRecent <= 0 {
		keepRecent = 10
	}
	if summaryTokens <= 0 {
		summaryTokens = 800
	}
	return &Compactor{model: m, name: modelName, threshold: thresholdTokens, keepRecent: keepRecent, summaryTokens: summaryTokens}
}

// ModelName reports the summarizer model, for usage metering.
func (c *Compactor) ModelName() string { return c.name }

// Threshold returns the token threshold at which compaction triggers.
func (c *Compactor) Threshold() int { return c.threshold }

// KeepRecent returns how many recent messages are kept uncompacted.
func (c *Compactor) KeepRecent() int { return c.keepRecent }

// SummaryTokens returns the target maximum tokens for the generated summary.
func (c *Compactor) SummaryTokens() int { return c.summaryTokens }

// EstimateHistoryTokens approximates Σ content tokens at 4 chars/token
// plus 40 tokens of framing per message (roles, tool envelopes).
func EstimateHistoryTokens(msgs []model.Message) int {
	total := 0
	for _, m := range msgs {
		n := len(m.Content)
		for _, p := range m.Parts {
			n += len(p.Data)
		}
		for _, tc := range m.ToolCalls {
			n += len(tc.Name) + len(tc.Args)
		}
		total += n/4 + 40
	}
	return total
}

// EstimateHistoryTextTokens estimates text and tool-call tokens only,
// excluding attachment bytes (counted separately by the context meter).
func EstimateHistoryTextTokens(msgs []model.Message) int {
	total := 0
	for _, m := range msgs {
		n := len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Name) + len(tc.Args)
		}
		total += n/4 + 40
	}
	return total
}

// EstimateTextTokens approximates one text blob at 4 chars/token.
func EstimateTextTokens(text string) int { return (len(text) + 3) / 4 }

// EstimateStorageTokens approximates Σ content tokens for persisted database messages.
func EstimateStorageTokens(msgs []storage.Message) int {
	total := 0
	for _, m := range msgs {
		n := len(m.Content)
		if n == 0 && len(m.Data) > 0 {
			n = len(m.Data)
		}
		total += n/4 + 40
	}
	return total
}

// Compact returns history unchanged under threshold; over threshold it
// summarizes as much of the stale prefix as fits the summarizer input cap,
// keeping at least the last keepRecent messages. Model failure is fail-open
// (history unchanged, compacted=false, nil error).
func (c *Compactor) Compact(ctx context.Context, history []model.Message) (recent []model.Message, summary string, usage model.Usage, compacted bool, err error) {
	if EstimateHistoryTokens(history) <= c.threshold || len(history) == 0 {
		return history, "", model.Usage{}, false, nil
	}
	keep := c.keepRecent
	if keep > len(history) {
		keep = len(history)
	}
	summary, included, usage, ok := c.summarizeOldest(ctx, history[:len(history)-keep])
	if !ok {
		return history, "", usage, false, nil
	}
	return history[included:], summary, usage, true, nil
}

// SummarizeNow summarizes the oldest messages that fit the input cap while
// leaving keepRecent newest messages, regardless of the automatic threshold.
// It returns how many leading messages the summary covers.
func (c *Compactor) SummarizeNow(ctx context.Context, history []model.Message, keepRecent int) (summary string, covered int, usage model.Usage, ok bool) {
	if keepRecent <= 0 {
		keepRecent = c.keepRecent
	}
	if len(history) <= keepRecent {
		return "", 0, model.Usage{}, false
	}
	return c.summarizeOldest(ctx, history[:len(history)-keepRecent])
}

// summarizeOldest summarizes the longest prefix of stale that fits the input
// cap, returning the number of messages actually covered.
func (c *Compactor) summarizeOldest(ctx context.Context, stale []model.Message) (string, int, model.Usage, bool) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Summarize this conversation history in under %d tokens for continuity. Keep: user goals, decisions made, facts retrieved from documents (with document titles), and open questions. Reply with the summary only, no preamble.\n\n", c.summaryTokens))
	used := 0
	included := 0
	for _, m := range stale {
		line := fmt.Sprintf("[%s] %s\n", m.Role, m.Content)
		if used+len(line) > historyInputCap {
			break
		}
		sb.WriteString(line)
		used += len(line)
		included++
	}
	if included == 0 {
		return "", 0, model.Usage{}, false
	}
	resp, gerr := c.model.Generate(ctx, model.Request{
		Messages: []model.Message{{Role: model.RoleUser, Content: sb.String()}},
	})
	if gerr != nil {
		return "", 0, resp.Usage, false
	}
	out := strings.TrimSpace(resp.Message.Content)
	if out == "" {
		return "", 0, resp.Usage, false
	}
	if max := c.summaryTokens * 4; len(out) > max {
		runes := []rune(out)
		if max > 3 && len(runes) > max {
			out = string(runes[:max-3]) + "…"
		} else if len(runes) > max {
			out = string(runes[:max])
		}
	}
	return out, included, resp.Usage, true
}
