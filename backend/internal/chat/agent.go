package chat

import (
	"context"
	"fmt"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
)

// Deps is the per-run identity flowing to every tool (search joins in Phase 2).
type Deps struct {
	UserID         string
	ConversationID string
}

// Sink receives run progress. Implemented by the httpapi SSE writer; tests
// use recording fakes.
type Sink interface {
	Delta(text string) error
	ModelStart()
	ModelEnd(inputTokens, outputTokens int)
}

const systemPrompt = `You are a helpful assistant in a local chat app.
Answer clearly and concisely in markdown.`

// Agent is the shared, concurrency-safe chat agent. One instance per process.
type Agent struct {
	agent *golem.Agent[Deps, string]
}

func New(client model.StreamingModel, usageLimit golem.UsageLimit) (*Agent, error) {
	passthrough := golem.DecodeFunc[string](func(_ context.Context, r model.Response) (string, error) {
		return r.Message.Content, nil
	})
	a, err := golem.New[Deps, string](client, passthrough,
		golem.WithInstructions[Deps, string](systemPrompt),
		golem.WithHistoryProcessor[Deps, string](golem.TrimHistory(40)),
		golem.WithMaxAttempts[Deps, string](2),
		golem.WithUsageLimit[Deps, string](usageLimit),
	)
	if err != nil {
		return nil, fmt.Errorf("build chat agent: %w", err)
	}
	return &Agent{agent: a}, nil
}

type Outcome struct {
	Output   string
	Messages []model.Message
	Usage    model.Usage
	Requests int
}

// Run streams one turn. Progress goes to sink; errors are returned as-is
// (golem.RunError with Partial evidence, or context cancellation) for the
// caller to classify and persist.
func (a *Agent) Run(ctx context.Context, deps Deps, history []model.Message, prompt string, sink Sink) (Outcome, error) {
	var requests int
	res, err := a.agent.RunStreamWithHistory(ctx, golem.RunContext[Deps]{Deps: deps},
		history, prompt,
		func(d model.Delta) error {
			if d.Content == "" {
				return nil
			}
			return sink.Delta(d.Content)
		},
		golem.WithRunObserver(func(e golem.RunEvent) {
			switch e.Kind {
			case golem.EventModelStart:
				sink.ModelStart()
			case golem.EventModelEnd:
				requests++
				sink.ModelEnd(e.Usage.InputTokens, e.Usage.OutputTokens)
			}
		}),
	)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Output: res.Output, Messages: res.Messages, Usage: res.Usage, Requests: requests}, nil
}
