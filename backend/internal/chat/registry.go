package chat

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/abubakarsiddik31/golem/model"
	golemanthropic "github.com/abubakarsiddik31/golem/providers/anthropic"
	golemgemini "github.com/abubakarsiddik31/golem/providers/gemini"
	golemopenai "github.com/abubakarsiddik31/golem/providers/openai"
)

// RunSpec is one run's model selection, resolved from the conversation row
// by the HTTP layer: which catalog model to hit, its sampling temperature
// (nil = provider default), and the system prompt (empty = built-in).
type RunSpec struct {
	Model        string
	Temperature  *float64
	SystemPrompt string
	// RagEnabled gates the built-in document search for this run; the
	// citation guidance joins the prompt exactly when the tool registers.
	RagEnabled bool
}

// clientFactory builds a streaming client for one (provider, model,
// temperature). The production factory constructs golem provider clients;
// tests inject fakes.
type clientFactory func(provider, modelName string, temperature *float64) (model.StreamingModel, error)

// ModelRegistry resolves RunSpecs to streaming clients. Provider clients are
// immutable and concurrency-safe, so identical specs share one cached
// instance; construction is pure validation (microseconds), so an unbounded
// cache keyed by spec is fine — the catalog bounds the distinct keys.
type ModelRegistry struct {
	keys ProviderKeys
	new  clientFactory

	mu    sync.Mutex
	cache map[string]model.StreamingModel
}

func NewModelRegistry(keys ProviderKeys) *ModelRegistry {
	return &ModelRegistry{keys: keys, cache: map[string]model.StreamingModel{}}
}

// NewModelRegistryWithFactory swaps the production client constructors for
// an injected factory (offline tests).
func NewModelRegistryWithFactory(keys ProviderKeys, f clientFactory) *ModelRegistry {
	return &ModelRegistry{keys: keys, new: f, cache: map[string]model.StreamingModel{}}
}

// Resolve returns the client for spec's (provider, model, temperature),
// building and caching it on first use. Temperatures are normalized to the
// provider's accepted range up front so clamped values share cache entries.
func (r *ModelRegistry) Resolve(spec RunSpec) (model.StreamingModel, error) {
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
	key := target.Provider + "|" + target.ID + "|" + registryTempKey(deref(temperature))

	r.mu.Lock()
	if c, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	factory := r.new
	if factory == nil {
		factory = r.productionFactory
	}
	client, err := factory(target.Provider, target.ID, temperature)
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
func (r *ModelRegistry) productionFactory(provider, modelName string, temperature *float64) (model.StreamingModel, error) {
	maxOutputTokens := 16384
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
		cfg.Temperature = temperature
		return golemgemini.New(cfg)
	case "openai":
		cfg := golemopenai.Config{APIKey: r.keys.OpenAI, Model: modelName, BaseURL: r.keys.OpenAIBaseURL, MaxTokens: maxOutputTokens}
		if r.keys.OpenAIBaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
		cfg.Temperature = temperature
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
		cfg.Temperature = temperature
		return golemanthropic.New(cfg)
	}
	return nil, fmt.Errorf("unsupported provider %q", provider)
}

// registryTempKey renders a temperature for cache keys and test assertions;
// unset shares the empty key with "no temperature" across providers.
func registryTempKey(t float64) string {
	return strconv.FormatFloat(t, 'f', -1, 64)
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
