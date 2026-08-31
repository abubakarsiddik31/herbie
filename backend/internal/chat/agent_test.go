package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

type recordingSink struct {
	deltas  []string
	ends    int
	in, out int
}

func (s *recordingSink) Delta(text string) error { s.deltas = append(s.deltas, text); return nil }
func (s *recordingSink) ModelStart()             {}
func (s *recordingSink) ModelEnd(in, out int)    { s.ends++; s.in += in; s.out += out }

func respond(content string) model.Response {
	return model.Response{Message: model.Message{Role: model.RoleAssistant, Content: content},
		Usage: model.Usage{InputTokens: 10, OutputTokens: 5}}
}

func TestRunStreamsAndCounts(t *testing.T) {
	m := testmodel.New().Respond(respond("hello "))
	a, err := New(m, golem.UsageLimit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sink := &recordingSink{}
	out, err := a.Run(context.Background(), Deps{UserID: "u", ConversationID: "c"}, nil, "hi", sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Output != "hello " || out.Usage.InputTokens != 10 || out.Usage.OutputTokens != 5 {
		t.Fatalf("outcome: %+v", out)
	}
	if len(sink.deltas) == 0 || sink.ends != 1 || sink.in != 10 {
		t.Fatalf("sink not fed: %+v", sink)
	}
	if out.Requests != 1 {
		t.Fatalf("requests = %d, want 1", out.Requests)
	}
}

func TestUsageLimitSurfacesStage(t *testing.T) {
	m := testmodel.New().Respond(respond("big"))
	a, _ := New(m, golem.UsageLimit{TotalTokens: 3})
	sink := &recordingSink{}
	_, err := a.Run(context.Background(), Deps{}, nil, "hi", sink)
	var runErr *golem.RunError
	if !errors.As(err, &runErr) || runErr.Stage != golem.StageUsage {
		t.Fatalf("expected usage-stage RunError, got %v", err)
	}
}
