package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/abubakarsiddik31/golem/model"
	golemanthropic "github.com/abubakarsiddik31/golem/providers/anthropic"
	golemgemini "github.com/abubakarsiddik31/golem/providers/gemini"
	golemopenai "github.com/abubakarsiddik31/golem/providers/openai"
	"github.com/abubakarsiddik31/golem/tokens"
)

// RunSpec is one run's model selection, resolved from the conversation row
// by the HTTP layer: which catalog model to hit, its sampling temperature
// (nil = provider default), and the system prompt (empty = built-in).
type RunSpec struct {
	Model       string
	Temperature *float64
	// TopP nil, MaxOutputTokens 0, and Reasoning "" leave the provider
	// default in place.
	TopP            *float64
	MaxOutputTokens int
	Reasoning       string
	SystemPrompt    string
	// RagEnabled gates the built-in document search for this run; the
	// citation guidance joins the prompt exactly when the tool registers.
	RagEnabled bool
}

// clientFactory builds a streaming client for one (provider, model,
// temperature). The production factory constructs golem provider clients;
// tests inject fakes.
type clientFactory func(provider, modelName string, temperature *float64) (model.StreamingModel, error)

// clientOptions carries the provider-agnostic generation controls a RunSpec
// resolved to. Temperature is provider-clamped before the factory sees it.
type clientOptions struct {
	Temperature     *float64
	TopP            *float64
	MaxOutputTokens int
	Reasoning       string
}

type clientOptionsFactory func(provider, modelName string, opts clientOptions) (model.StreamingModel, error)

// ModelRegistry resolves RunSpecs to streaming clients. Provider clients are
// immutable and concurrency-safe, so identical specs share one cached
// instance; construction is pure validation (microseconds), so an unbounded
// cache keyed by spec is fine — the catalog bounds the distinct keys.
type ModelRegistry struct {
	keys    ProviderKeys
	new     clientFactory
	newOpts clientOptionsFactory
	counter CounterFactory

	mu       sync.Mutex
	cache    map[string]model.StreamingModel
	counters map[string]tokens.Counter
}

// CounterFactory builds a provider token counter for a model. It is a test
// seam; production uses the golem provider counters.
type CounterFactory func(provider, model string) (tokens.Counter, error)

// ErrNoTokenCounter means the provider has no exact counting endpoint for
// this model; callers fall back to an estimate.
var ErrNoTokenCounter = errors.New("input-token counting unavailable for this model")

func NewModelRegistry(keys ProviderKeys) *ModelRegistry {
	return &ModelRegistry{keys: keys, cache: map[string]model.StreamingModel{}}
}

// NewModelRegistryWithFactory swaps the production client constructors for
// an injected factory (offline tests).
func NewModelRegistryWithFactory(keys ProviderKeys, f clientFactory) *ModelRegistry {
	return &ModelRegistry{keys: keys, new: f, cache: map[string]model.StreamingModel{}}
}

// NewModelRegistryWithOptionsFactory is the options-aware test seam.
func NewModelRegistryWithOptionsFactory(keys ProviderKeys, f clientOptionsFactory) *ModelRegistry {
	return &ModelRegistry{keys: keys, newOpts: f, cache: map[string]model.StreamingModel{}}
}

// NewModelRegistryWithCounterFactory swaps the provider counters for an
// injected factory (offline tests).
func NewModelRegistryWithCounterFactory(keys ProviderKeys, f CounterFactory) *ModelRegistry {
	return &ModelRegistry{keys: keys, counter: f, cache: map[string]model.StreamingModel{}}
}

// CountInputTokens prices a would-be request exactly when the provider offers
// a counting endpoint (Gemini/Anthropic); callers fall back to estimates on
// ErrNoTokenCounter or any provider error.
func (r *ModelRegistry) CountInputTokens(ctx context.Context, spec RunSpec, messages []model.Message, toolSpecs []model.ToolSpec) (int, error) {
	if spec.Model == AutoModelID {
		return 0, ErrNoTokenCounter
	}
	target, ok := FindModel(spec.Model)
	if !ok {
		return 0, ErrNoTokenCounter
	}
	key := target.Provider + "|" + target.ID
	r.mu.Lock()
	counter := r.counters[key]
	r.mu.Unlock()
	if counter == nil {
		factory := r.counter
		if factory == nil {
			factory = r.productionCounter
		}
		built, err := factory(target.Provider, target.ID)
		if err != nil {
			return 0, err
		}
		r.mu.Lock()
		if r.counters == nil {
			r.counters = map[string]tokens.Counter{}
		}
		r.counters[key] = built
		r.mu.Unlock()
		counter = built
	}
	return counter.CountTokens(ctx, tokens.CountInput{Messages: messages, Tools: toolSpecs})
}

func (r *ModelRegistry) productionCounter(provider, model string) (tokens.Counter, error) {
	switch provider {
	case "gemini":
		if r.keys.Gemini == "" {
			return nil, ErrNoTokenCounter
		}
		return golemgemini.NewCounter(golemgemini.Config{
			APIKey: r.keys.Gemini, Model: model, BaseURL: r.keys.GeminiBaseURL,
		})
	case "anthropic":
		if r.keys.Anthropic == "" {
			return nil, ErrNoTokenCounter
		}
		return golemanthropic.NewCounter(golemanthropic.Config{
			APIKey: r.keys.Anthropic, Model: model, BaseURL: r.keys.AnthropicBaseURL,
		})
	}
	return nil, ErrNoTokenCounter
}

// Resolve returns the client for spec's (provider, model, temperature),
// building and caching it on first use. Temperatures are normalized to the
// provider's accepted range up front so clamped values share cache entries.
// AutoModelID selects the ordered fallback chain over the configured
// catalog models. It is a UI/alias value, not a catalog row.
const AutoModelID = "auto"

func (r *ModelRegistry) Resolve(spec RunSpec) (model.StreamingModel, error) {
	if spec.Model == AutoModelID {
		return r.resolveAuto(spec)
	}
	target, ok := FindModel(spec.Model)
	if !ok {
		return nil, fmt.Errorf("unknown model %q", spec.Model)
	}
	temperature := spec.Temperature
	if temperature != nil {
		hi, lo := 2.0, 0.0
		if target.Provider == "anthropic" {
			hi = 1.0
		}
		t := clamp(*temperature, lo, hi)
		temperature = &t
	}
	key := target.Provider + "|" + target.ID + "|" + registryTempKey(deref(temperature)) +
		"|" + optionalFloatKey(spec.TopP) + "|" + strconv.Itoa(spec.MaxOutputTokens) + "|" + spec.Reasoning

	r.mu.Lock()
	if c, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	opts := clientOptions{
		Temperature: temperature, TopP: spec.TopP,
		MaxOutputTokens: spec.MaxOutputTokens, Reasoning: spec.Reasoning,
	}
	var client model.StreamingModel
	var err error
	switch {
	case r.newOpts != nil:
		client, err = r.newOpts(target.Provider, target.ID, opts)
	case r.new != nil:
		client, err = r.new(target.Provider, target.ID, temperature)
	default:
		client, err = r.productionFactory(target.Provider, target.ID, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("build %s client for %s: %w", target.Provider, target.ID, err)
	}

	r.mu.Lock()
	r.cache[key] = client
	r.mu.Unlock()
	return client, nil
}

// productionFactory constructs the golem provider client for the spec.
// Temperature is already provider-clamped by Resolve.
// resolveAuto builds a fresh fallback over the available catalog models so
// each run gets its own used-model recorder (the provider clients themselves
// stay cached by their concrete Resolve calls).
func (r *ModelRegistry) resolveAuto(spec RunSpec) (model.StreamingModel, error) {
	available := Available(r.keys)
	if len(available) == 0 {
		return nil, fmt.Errorf("auto model requires a configured provider")
	}
	tracker := &usedModelTracker{}
	members := make([]model.StreamingModel, 0, len(available))
	named := make([]model.Model, 0, len(available))
	for _, candidate := range available {
		concrete := spec
		concrete.Model = candidate.ID
		client, err := r.Resolve(concrete)
		if err != nil {
			return nil, fmt.Errorf("auto fallback build %s: %w", candidate.ID, err)
		}
		client = &namedModel{name: candidate.ID, inner: client, tracker: tracker}
		members = append(members, client)
		named = append(named, client)
	}
	if len(members) == 1 {
		return &autoModel{inner: members[0], tracker: tracker}, nil
	}
	fb, err := model.NewFallback(named[0], named[1:]...)
	if err != nil {
		return nil, err
	}
	return &autoModel{inner: fb, tracker: tracker}, nil
}

// usedModelRecords is implemented by auto-fallback clients so the agent can
// report which member answered.
type usedModelRecords interface {
	UsedModel() string
}

type usedModelTracker struct {
	mu   sync.Mutex
	name string
}

func (t *usedModelTracker) set(name string) {
	t.mu.Lock()
	t.name = name
	t.mu.Unlock()
}

func (t *usedModelTracker) get() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.name
}

type namedModel struct {
	name    string
	inner   model.StreamingModel
	tracker *usedModelTracker
}

func (n *namedModel) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	resp, err := n.inner.Generate(ctx, req)
	if err == nil {
		n.tracker.set(n.name)
	}
	return resp, err
}

func (n *namedModel) GenerateStream(ctx context.Context, req model.Request, onDelta func(model.Delta) error) (model.Response, error) {
	resp, err := n.inner.GenerateStream(ctx, req, onDelta)
	if err == nil {
		n.tracker.set(n.name)
	}
	return resp, err
}

type autoModel struct {
	inner   model.StreamingModel
	tracker *usedModelTracker
}

func (a *autoModel) Generate(ctx context.Context, req model.Request) (model.Response, error) {
	return a.inner.Generate(ctx, req)
}

func (a *autoModel) GenerateStream(ctx context.Context, req model.Request, onDelta func(model.Delta) error) (model.Response, error) {
	return a.inner.GenerateStream(ctx, req, onDelta)
}

func (a *autoModel) UsedModel() string { return a.tracker.get() }

func (r *ModelRegistry) productionFactory(provider, modelName string, opts clientOptions) (model.StreamingModel, error) {
	maxOutputTokens := 16384
	if opts.MaxOutputTokens > 0 {
		maxOutputTokens = opts.MaxOutputTokens
	}
	if envMax := os.Getenv("CHAT_MAX_OUTPUT_TOKENS"); envMax != "" {
		if v, err := strconv.Atoi(envMax); err == nil && v >= 10000 {
			maxOutputTokens = v
		}
	}

	switch provider {
	case "gemini":
		cfg := golemgemini.Config{APIKey: r.keys.Gemini, Model: modelName, BaseURL: r.keys.GeminiBaseURL, MaxTokens: maxOutputTokens}
		if r.keys.GeminiBaseURL == "" {
			cfg.BaseURL = "https://generativelanguage.googleapis.com"
		}
		cfg.Temperature = opts.Temperature
		cfg.TopP = opts.TopP
		if opts.Reasoning != "" {
			cfg.Thinking = &golemgemini.ThinkingConfig{Level: opts.Reasoning}
		}
		return golemgemini.New(cfg)
	case "openai":
		cfg := golemopenai.Config{APIKey: r.keys.OpenAI, Model: modelName, BaseURL: r.keys.OpenAIBaseURL, MaxTokens: maxOutputTokens}
		if r.keys.OpenAIBaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
		cfg.Temperature = opts.Temperature
		cfg.TopP = opts.TopP
		cfg.ReasoningEffort = opts.Reasoning
		return golemopenai.New(cfg)
	case "anthropic":
		anthropicMax := maxOutputTokens
		if anthropicMax > 8192 {
			anthropicMax = 8192
		}
		cfg := golemanthropic.Config{APIKey: r.keys.Anthropic, Model: modelName, MaxTokens: anthropicMax}
		if r.keys.AnthropicBaseURL != "" {
			cfg.BaseURL = r.keys.AnthropicBaseURL
		}
		cfg.Temperature = opts.Temperature
		cfg.TopP = opts.TopP
		switch opts.Reasoning {
		case "low":
			cfg.Thinking = &golemanthropic.ThinkingConfig{BudgetTokens: 1024}
		case "medium":
			cfg.Thinking = &golemanthropic.ThinkingConfig{BudgetTokens: 3072}
		case "high":
			cfg.Thinking = &golemanthropic.ThinkingConfig{BudgetTokens: 6144}
		}
		if cfg.Thinking != nil && cfg.MaxTokens <= cfg.Thinking.BudgetTokens {
			cfg.MaxTokens = cfg.Thinking.BudgetTokens + 2048
			if cfg.MaxTokens > 8192 {
				cfg.MaxTokens = 8192
			}
		}
		return golemanthropic.New(cfg)
	}
	return nil, fmt.Errorf("unsupported provider %q", provider)
}

// registryTempKey renders a temperature for cache keys and test assertions;
// unset shares the empty key with "no temperature" across providers.
func registryTempKey(t float64) string {
	return strconv.FormatFloat(t, 'f', -1, 64)
}

func optionalFloatKey(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

func deref(f *float64) float64 {
	if f == nil {
		return -1 // never a valid temperature; keeps unset out of a set key
	}
	return *f
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
