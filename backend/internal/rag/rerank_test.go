package rag

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
)

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

type spyModel struct {
	seen *model.Request
	out  string
}

func (s spyModel) Generate(_ context.Context, req model.Request) (model.Response, error) {
	*s.seen = req
	return model.Response{Message: model.Message{Content: s.out}}, nil
}

func scoredFixture() []Scored {
	return []Scored{
		{Chunk: Chunk{DocumentID: "d", DocTitle: "t", Content: "first passage"}, Score: 0.9},
		{Chunk: Chunk{DocumentID: "d", DocTitle: "t", Content: "second passage"}, Score: 0.8},
		{Chunk: Chunk{DocumentID: "d", DocTitle: "t", Content: "third passage"}, Score: 0.7},
	}
}

func TestRerankReorders(t *testing.T) {
	r := NewRanker(stubModel{out: `{"ranking":[2,0,1]}`, usage: model.Usage{InputTokens: 100, OutputTokens: 10}}, "m")
	got, usage, err := r.Rerank(context.Background(), "q", scoredFixture(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Chunk.Content != "third passage" || got[1].Chunk.Content != "first passage" || got[2].Chunk.Content != "second passage" {
		t.Fatalf("order: %+v", got)
	}
	if usage.InputTokens != 100 || usage.OutputTokens != 10 {
		t.Fatalf("usage: %+v", usage)
	}
}

func TestRerankTrimsTopN(t *testing.T) {
	r := NewRanker(stubModel{out: `{"ranking":[2,0,1]}`}, "m")
	got, _, err := r.Rerank(context.Background(), "q", scoredFixture(), 2)
	if err != nil || len(got) != 2 || got[0].Chunk.Content != "third passage" {
		t.Fatalf("trim: %+v %v", got, err)
	}
}

func TestRerankRanksExpandedContext(t *testing.T) {
	var seen model.Request
	spy := spyModel{&seen, `{"ranking":[1,0]}`}
	in := []Scored{
		{Chunk: Chunk{DocTitle: "t", Content: "core one"}, Context: "neighbor core one neighbor"},
		{Chunk: Chunk{DocTitle: "t", Content: "core two"}, Context: "neighbor core two neighbor"},
	}
	r := NewRanker(spy, "m")
	got, _, err := r.Rerank(context.Background(), "q", in, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seen.Messages[1].Content, "neighbor core one neighbor") {
		t.Fatalf("prompt lacks expanded context: %s", seen.Messages[1].Content)
	}
	if got[0].Chunk.Content != "core two" {
		t.Fatalf("order: %+v", got)
	}
}

func TestRerankGarbageFallsBack(t *testing.T) {
	r := NewRanker(stubModel{out: `not json at all`}, "m")
	got, _, err := r.Rerank(context.Background(), "q", scoredFixture(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Chunk.Content != "first passage" {
		t.Fatalf("fallback: %+v", got)
	}
}

func TestRerankModelErrorFallsBack(t *testing.T) {
	r := NewRanker(stubModel{err: fmt.Errorf("boom")}, "m")
	got, _, err := r.Rerank(context.Background(), "q", scoredFixture(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Chunk.Content != "first passage" {
		t.Fatalf("fallback: %+v", got)
	}
}

func TestRerankCapsCandidates(t *testing.T) {
	var seen model.Request
	spy := spyModel{&seen, `{"ranking":[]}`}
	in := make([]Scored, 45)
	for i := range in {
		in[i] = Scored{Chunk: Chunk{Content: fmt.Sprintf("passage %d", i)}}
	}
	r := NewRanker(spy, "m")
	if _, _, err := r.Rerank(context.Background(), "q", in, 45); err != nil {
		t.Fatal(err)
	}
	n := strings.Count(seen.Messages[1].Content, "passage ")
	if n != 40 {
		t.Fatalf("prompt carries %d passages, want 40", n)
	}
}

func TestRerankEmptyPassthrough(t *testing.T) {
	r := NewRanker(stubModel{out: `{"ranking":[0]}`}, "m")
	got, _, err := r.Rerank(context.Background(), "q", nil, 5)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty: %+v %v", got, err)
	}
}
