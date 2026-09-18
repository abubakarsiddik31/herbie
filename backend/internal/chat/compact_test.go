package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
)

// stubModel is a minimal model stub for compactor tests. (The rerank
// stub of the same shape lives in package rag and is not importable
// here, so this package defines its own.)
type stubModel struct {
	out   string
	usage model.Usage
	err   error
}

func (s stubModel) Generate(_ context.Context, _ model.Request) (model.Response, error) {
	if s.err != nil {
		return model.Response{}, s.err
	}
	return model.Response{Message: model.Message{Content: s.out}, Usage: s.usage}, nil
}

func historyFixture(n int) []model.Message {
	msgs := make([]model.Message, n)
	for i := range msgs {
		role := model.RoleUser
		if i%2 == 1 {
			role = model.RoleAssistant
		}
		msgs[i] = model.Message{Role: role, Content: strings.Repeat(fmt.Sprintf("message %d. ", i), 200)}
	}
	return msgs
}

func TestEstimateHistoryTokens(t *testing.T) {
	msgs := []model.Message{{Role: model.RoleUser, Content: "abcd"}}
	if got := EstimateHistoryTokens(msgs); got != 1+40 {
		t.Fatalf("estimate: %d", got)
	}
}

func TestCompactPassthroughUnderThreshold(t *testing.T) {
	c := NewCompactor(stubModel{out: "x"}, "m", 100000, 10, 800)
	h := historyFixture(4)
	recent, summary, _, compacted, err := c.Compact(context.Background(), h)
	if err != nil || compacted || summary != "" || len(recent) != len(h) {
		t.Fatalf("passthrough: %q %v %v", summary, compacted, err)
	}
}

func TestCompactSummarizesOldSpan(t *testing.T) {
	c := NewCompactor(stubModel{
		out:   "User discussed cats; decided to adopt.",
		usage: model.Usage{InputTokens: 9000, OutputTokens: 20},
	}, "m", 100, 4, 800)
	h := historyFixture(10)
	recent, summary, usage, compacted, err := c.Compact(context.Background(), h)
	if err != nil || !compacted {
		t.Fatal(err)
	}
	if len(recent) != 4 || recent[0].Content != h[6].Content {
		t.Fatalf("recent window: %d", len(recent))
	}
	if summary != "User discussed cats; decided to adopt." {
		t.Fatalf("summary: %q", summary)
	}
	if usage.InputTokens != 9000 || usage.OutputTokens != 20 {
		t.Fatalf("usage: %+v", usage)
	}
}

func TestCompactModelFailureKeepsHistory(t *testing.T) {
	c := NewCompactor(stubModel{err: errors.New("boom")}, "m", 100, 4, 800)
	h := historyFixture(10)
	recent, summary, _, compacted, err := c.Compact(context.Background(), h)
	if err != nil || compacted || summary != "" || len(recent) != len(h) {
		t.Fatalf("fail-open: %v %v", compacted, err)
	}
}

func TestCompactCapsSummary(t *testing.T) {
	c := NewCompactor(stubModel{out: strings.Repeat("x", 5000)}, "m", 100, 4, 10)
	_, summary, _, compacted, _ := c.Compact(context.Background(), historyFixture(10))
	if !compacted || len(summary) > 10*4 {
		t.Fatalf("cap: %d", len(summary))
	}
}
