package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// Deps is the per-run identity flowing to every tool. Search is wired by
// the HTTP layer when the RAG stack is enabled (nil = no documents).
type Deps struct {
	UserID         string
	ConversationID string
	Search         SearchFunc
	SaveMemory     func(ctx context.Context, fact string) error
}

// PendingApproval is one deferred tool call waiting on the user's decision.
type PendingApproval struct {
	CallID   string          `json:"callId"`
	ToolName string          `json:"toolName"`
	Args     json.RawMessage `json:"args"`
	Reason   string          `json:"reason"`
}

// Sink receives run progress. Implemented by the httpapi SSE writer; tests
// use recording fakes.
type Sink interface {
	Delta(text string) error
	ModelStart()
	ModelEnd(inputTokens, outputTokens int)
	ToolStart(name string)
	ToolEnd(name string, ok bool)
}

const systemPrompt = `You are a helpful assistant in a local chat app.
Answer clearly and concisely in markdown.`

// DefaultSystemPrompt is the persona promptFor falls back to when a
// conversation sets none. maybeCompact seeds empty specs with it so a
// compacted run never silently drops the persona.
const DefaultSystemPrompt = systemPrompt

// Agent builds a golem agent per run. Golem fixes the model client, the
// system instructions, and the tool set at construction, so a per-request
// build is how a run's conversation-scoped model (RunSpec) and user runtime
// tools join the chat; construction is pure validation and shares all heavy
// state through the model registry's client cache.
type Agent struct {
	registry *ModelRegistry
	limit    golem.UsageLimit
	env      ToolEnv
}

func New(registry *ModelRegistry, usageLimit golem.UsageLimit, env ToolEnv) (*Agent, error) {
	return &Agent{registry: registry, limit: usageLimit, env: env}, nil
}

// build assembles the golem agent for one run (or resume) against the
// conversation's chosen model and system prompt.
func (a *Agent) build(spec RunSpec, tools []tool.Tool[Deps]) (*golem.Agent[Deps, string], error) {
	client, err := a.registry.Resolve(spec)
	if err != nil {
		return nil, err
	}
	passthrough := golem.DecodeFunc[string](func(_ context.Context, r model.Response) (string, error) {
		return r.Message.Content, nil
	})
	maxIter := 25
	if a.limit.Requests > 0 {
		maxIter = a.limit.Requests
	}
	if envIter := os.Getenv("CHAT_MAX_ITERATIONS"); envIter != "" {
		if v, err := strconv.Atoi(envIter); err == nil && v >= 1 {
			maxIter = v
		}
	}
	opts := []golem.Option[Deps, string]{
		golem.WithInstructions[Deps, string](promptFor(spec, tools)),
		golem.WithHistoryProcessor[Deps, string](golem.TrimHistory(40)),
		golem.WithMaxAttempts[Deps, string](2),
		golem.WithUsageLimit[Deps, string](a.limit),
		golem.WithMaxIterations[Deps, string](maxIter),
		golem.WithToolRetries[Deps, string](2),
		golem.WithToolTimeout[Deps, string](a.env.HTTPTimeout + 10*time.Second),
	}
	if len(tools) > 0 {
		opts = append(opts, golem.WithTools[Deps, string](tools...))
	}
	agent, err := golem.New(client, passthrough, opts...)
	if err != nil {
		return nil, fmt.Errorf("build chat agent: %w", err)
	}
	return agent, nil
}

const retrievalGuidance = `

You have a search_documents tool over the user's uploaded files. Use it when queries may relate to the user's uploaded documents. Drive retrieval yourself: start with a focused query; if results look thin or off-topic, call again with a refined query or narrow documentIds (limit to 1-3 searches total). Once relevant evidence is found, immediately synthesize your answer. Never attribute claims to uploaded documents if they were not in the search results.
Citation discipline (hard rules): cite EVERY claim that comes from documents with its bracket number, e.g. [1]; cite ONLY bracket numbers shown in tool results — numbers are cumulative across calls ([1], [2], [3]...); never invent numbers not present in results. If the question specifically asks about the user's uploaded documents and the evidence does not support an answer, say what is missing instead of guessing; for general knowledge questions or external tools, answer normally using that information.`

const webSearchGuidance = `

You have a web_search tool to search the live web. Call it whenever the user asks about current events, breaking news, live data, or facts not present in your knowledge. Formulate clean, concise search keywords (do not include mention tags like "@web" in your query).

Search budget & loop discipline:
- Be concise and selective with searches: 1 to 3 targeted queries are usually plenty to answer even broad topics.
- Do NOT run recursive or open-ended search loops. Once you have gathered sufficient key information or initial relevant results, STOP searching immediately and synthesize your final answer.
- Prioritize delivering a clear, well-structured answer with what you found rather than continuously searching for further sub-details.

When answering based on web search results:
- Provide comprehensive, explanatory, and detailed answers in markdown (use clear topic headings, structured bullet points, and explanatory paragraphs).
- Incorporate specific details, key developments, framework updates, real-world examples, and context found in the results rather than giving a brief 2-3 sentence summary.
- Cite web sources with their titles and URLs e.g. [Title](URL) or citation numbers [N] directly in your answer.`

// promptFor resolves the run's system prompt: the conversation's prompt,
// else the built-in one, plus citation rules, retrieval guidance, or web search
// guidance when those tools are registered.
func promptFor(spec RunSpec, tools []tool.Tool[Deps]) string {
	prompt := spec.SystemPrompt
	if prompt == "" {
		prompt = systemPrompt
	}
	hasDocSearch := false
	hasWebSearch := false
	for _, t := range tools {
		if t.Name == SearchToolName {
			hasDocSearch = true
		}
		if t.Name == WebSearchToolName {
			hasWebSearch = true
		}
	}
	if hasDocSearch {
		prompt += citationRules + retrievalGuidance
	}
	if hasWebSearch {
		prompt += webSearchGuidance
	}
	return prompt
}

type Outcome struct {
	Output   string
	Messages []model.Message
	Usage    model.Usage
	Requests int
	// Pending is non-empty when the run paused on approval-gated tool calls.
	// Output is empty then; resume through RunDeferred.
	Pending []PendingApproval
}

// Run streams one turn. Progress goes to sink; errors are returned as-is
// (golem.RunError with Partial evidence, or context cancellation) for the
// caller to classify and persist. Parts are the prompt's image attachments
// (nil for a text-only turn).
func (a *Agent) Run(ctx context.Context, deps Deps, history []model.Message, prompt string, parts []model.Part, sink Sink, tools []tool.Tool[Deps], spec RunSpec) (Outcome, error) {
	agent, err := a.build(spec, tools)
	if err != nil {
		return Outcome{}, err
	}
	var requests int
	runOpts := []golem.RunOption{golem.WithRunObserver(observe(sink, &requests))}
	if len(parts) > 0 {
		runOpts = append(runOpts, golem.WithPromptParts(parts...))
	}
	res, err := agent.RunStreamWithHistory(ctx, golem.RunContext[Deps]{Deps: deps},
		history, prompt,
		onDelta(sink),
		runOpts...)
	if err != nil {
		return Outcome{}, err
	}
	return outcomeFrom(res, requests), nil
}

// RunDeferred resumes a run paused on approvals (or external results).
// golem's deferred resume is non-streaming, so the answer reaches the sink as
// a single delta after the run completes.
func (a *Agent) RunDeferred(ctx context.Context, deps Deps, history []model.Message, results golem.DeferredResults, sink Sink, tools []tool.Tool[Deps], spec RunSpec) (Outcome, error) {
	agent, err := a.build(spec, tools)
	if err != nil {
		return Outcome{}, err
	}
	var requests int
	res, err := agent.RunWithDeferredResults(ctx, golem.RunContext[Deps]{Deps: deps},
		history, results, "",
		golem.WithRunObserver(observe(sink, &requests)),
	)
	if err != nil {
		return Outcome{}, err
	}
	outcome := outcomeFrom(res, requests)
	if outcome.Output != "" {
		if err := sink.Delta(outcome.Output); err != nil {
			return Outcome{}, err
		}
	}
	return outcome, nil
}

func onDelta(sink Sink) func(model.Delta) error {
	return func(d model.Delta) error {
		if d.Content == "" {
			return nil
		}
		return sink.Delta(d.Content)
	}
}

func observe(sink Sink, requests *int) func(golem.RunEvent) {
	return func(e golem.RunEvent) {
		switch e.Kind {
		case golem.EventModelStart:
			sink.ModelStart()
		case golem.EventModelEnd:
			*requests++
			sink.ModelEnd(e.Usage.InputTokens, e.Usage.OutputTokens)
		case golem.EventToolStart:
			sink.ToolStart(e.ToolName)
		case golem.EventToolEnd:
			sink.ToolEnd(e.ToolName, e.Err == nil)
		}
	}
}

func outcomeFrom(res golem.Result[string], requests int) Outcome {
	outcome := Outcome{Output: res.Output, Messages: res.Messages, Usage: res.Usage, Requests: requests}
	if res.Pending != nil {
		for _, c := range res.Pending.Approvals {
			outcome.Pending = append(outcome.Pending, PendingApproval{
				CallID: c.CallID, ToolName: c.ToolName, Args: c.Args, Reason: c.Reason,
			})
		}
		outcome.Output = ""
	}
	return outcome
}
