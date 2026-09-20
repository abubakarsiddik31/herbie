package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/websearch"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// Deps is the per-run identity flowing to every tool. Search is wired by
// the HTTP layer when the RAG stack is enabled (nil = no documents).
type Deps struct {
	UserID         string
	ConversationID string
	Search         SearchFunc
	ListDocs       ListDocsFunc
	ReadDoc        ReadDocFunc
	SaveMemory     func(ctx context.Context, fact string) error
	RecordSearch   func(ctx context.Context, query, kind, provider string, resultsCount int, durationMs int64)
	AddWebSources  func(results []websearch.Result)
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

You have retrieval tools over the user's uploaded files:
- list_documents: lists all uploaded files with their document IDs, filenames, chunk counts, sizes, and statuses. Use this when you need an overview of available documents or want to find specific document IDs.
- read_document: reads bounded sequential chunks of a specific document (parameters: documentId, offset, limit). Use this for document summarization, chapter reading, or inspecting consecutive text.
- search_documents: searches for passages relevant to a query across all files or filtered by documentIds.

Workflows for document requests:
1. Summarization workflow:
   - When asked to summarize, outline, or explain an uploaded document (e.g. "summarize <file>"):
   - Step 1: Look up the document ID using list_documents if not already known.
   - Step 2: Call read_document(documentId="<id>", offset=0, limit=5) to read the beginning (title, authors, abstract, executive summary, and introduction).
   - Step 3: If you need the conclusion or key findings to complete the summary, either call read_document with the next offset or call search_documents with targeted keywords like "conclusion results discussion" filtered to that document ID.
   - Step 4: Synthesize a well-structured markdown summary with citations [1], [2] referencing the read chunks.
   - NEVER call web_search when asked to summarize or query an uploaded document.

2. Multi-hop, comparative, and facet queries:
   - DECISION RULE: Determine whether your sub-queries are independent or dependent before retrieving:
     a) Independent Facets (Parallel Search): If you need to compare two known concepts, sections, or documents simultaneously (e.g. "compare methodology in Doc A vs benchmarks in Doc B"):
        -> Use parallel retrieval by passing "queries": ["query 1", "query 2"] to search_documents in a single call.
     b) Dependent Multi-Hop (Sequential Hops): If Step 2 strictly depends on an unknown entity, citation, or finding from Step 1 (e.g. "find who authored Theorem 1 in Doc A, then find what other papers they published"):
        -> Execute Step 1 first with a single query to retrieve the premise.
        -> Inspect the retrieved passages to identify the specific entity.
        -> Execute Step 2 as a targeted follow-up query using the discovered entity.
        -> Do NOT run open-ended chains; cap at 2-3 hops total, then synthesize your final answer with bracket citations.

Retrieval budget & stop discipline:
- Limit retrieval to 1 or at most 2-3 focused tool calls total (call again with a refined query or narrow documentIds only if initial results are completely off-topic).
- Once you obtain initial relevant passages or the core document sections, STOP calling search/read tools immediately and synthesize your final answer.
- Never attribute claims to uploaded documents if they were not in the search/read results.

Citation discipline (hard rules): cite EVERY claim that comes from documents with its bracket number, e.g. [1]; cite ONLY bracket numbers shown in tool results — numbers are cumulative across calls ([1], [2], [3]...); never invent numbers not present in results. If the question specifically asks about the user's uploaded documents and the evidence does not support an answer, say what is missing instead of guessing; for general knowledge questions or external tools, answer normally using that information.`

const webSearchGuidance = `

You have a web_search tool to search the live web. Call it whenever the user asks about current events, breaking news, live data, or general external facts not present in your knowledge. Formulate clean, concise search keywords (do not include mention tags like "@web" in your query).

PARALLEL VS. SEQUENTIAL MULTI-HOP DECISION RULE:
- Independent Facets (Parallel Search):
  When researching multiple known sub-topics or entities simultaneously (e.g. "latest news on OpenAI, Google, Anthropic"):
  Pass the "queries" array: {"query": "AI agents news", "queries": ["OpenAI Operator agent", "Google Jarvis agent", "Anthropic Claude computer use"]}.
  All queries execute concurrently in parallel in a single fast round. Always prefer parallel search for independent sub-topics!
- Dependent Multi-Hop (Sequential Hops):
  When Step 2 depends on an unknown fact from Step 1 (e.g. "find who won the 2025 AI prize, then search what institution they work at"):
  Execute Step 1 first with a single query, inspect the winner's identity from the results, then execute Step 2 with that specific name.
  Limit sequential chains to at most 2-3 hops total.

STRICT EXCLUSION FOR UPLOADED DOCUMENTS:
- Do NOT use web_search if the user's request is asking to summarize, explain, or query an uploaded document, file, or workspace attachment. Use read_document and search_documents exclusively for files.

Search budget & loop discipline:
- HARD BUDGET: Limit searching to 1 or at most 2 tool calls total per turn.
- Do NOT run recursive or open-ended sequential search loops across minor sub-details.
- Once you obtain initial search results, STOP searching immediately and synthesize your final answer using the retrieved facts.
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
		if t.Name == SearchToolName || t.Name == ListDocumentsToolName || t.Name == ReadDocumentToolName {
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
