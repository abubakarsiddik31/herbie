package chat

import (
	"context"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
)

// fakeClient is a pointer-typed streaming client so cached-vs-fresh
// resolution can be compared with == (a func-typed fake would panic).
type fakeClient struct{}

func (fakeClient) GenerateStream(_ context.Context, _ model.Request, _ func(model.Delta) error) (model.Response, error) {
	return model.Response{}, nil
}

func (fakeClient) Generate(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{}, nil
}

// fakeFactory records every construction and hands back a fresh scripted
// client, standing in for the provider HTTP clients.
func fakeFactory(calls *[]string) clientFactory {
	return func(provider, modelName string, temperature *float64) (model.StreamingModel, error) {
		call := provider + "|" + modelName + "|"
		if temperature != nil {
			call += formatTemp(*temperature)
		}
		*calls = append(*calls, call)
		return fakeClient{}, nil
	}
}

func formatTemp(t float64) string {
	// Mirror the registry's cache-key formatting for assertions.
	return registryTempKey(t)
}

func TestResolveBuildsAndCachesPerSpec(t *testing.T) {
	var calls []string
	reg := NewModelRegistryWithFactory(ProviderKeys{Gemini: "g", OpenAI: "o"}, fakeFactory(&calls))

	c1, err := reg.Resolve(RunSpec{Model: "gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("resolve gemini: %v", err)
	}
	c2, err := reg.Resolve(RunSpec{Model: "gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("resolve gemini again: %v", err)
	}
	if c1 != c2 {
		t.Error("same spec must return the cached client")
	}
	if len(calls) != 1 || calls[0] != "gemini|gemini-2.5-flash|" {
		t.Fatalf("calls = %v, want exactly one gemini construction", calls)
	}

	if _, err := reg.Resolve(RunSpec{Model: "gpt-5", Temperature: ptrFloat(0.7)}); err != nil {
		t.Fatalf("resolve openai: %v", err)
	}
	if len(calls) != 2 || calls[1] != "openai|gpt-5|0.7" {
		t.Fatalf("calls = %v, want openai construction with 0.7 temperature", calls)
	}

	// Different temperature = different client.
	if _, err := reg.Resolve(RunSpec{Model: "gpt-5", Temperature: ptrFloat(1.2)}); err != nil {
		t.Fatalf("resolve openai hot: %v", err)
	}
	if len(calls) != 3 || calls[2] != "openai|gpt-5|1.2" {
		t.Fatalf("calls = %v, want separate construction for 1.2", calls)
	}
}

func TestResolveClampsAnthropicTemperature(t *testing.T) {
	var calls []string
	reg := NewModelRegistryWithFactory(ProviderKeys{Anthropic: "a"}, fakeFactory(&calls))
	if _, err := reg.Resolve(RunSpec{Model: "claude-sonnet-4-5", Temperature: ptrFloat(1.7)}); err != nil {
		t.Fatalf("resolve anthropic: %v", err)
	}
	if len(calls) != 1 || calls[0] != "anthropic|claude-sonnet-4-5|1" {
		t.Fatalf("calls = %v, want temperature clamped to 1", calls)
	}
}

func TestResolveRejectsUnknownAndEmptyModel(t *testing.T) {
	reg := NewModelRegistryWithFactory(ProviderKeys{Gemini: "g"}, fakeFactory(&[]string{}))
	if _, err := reg.Resolve(RunSpec{Model: "nope"}); err == nil {
		t.Error("unknown model must error")
	}
	if _, err := reg.Resolve(RunSpec{}); err == nil {
		t.Error("empty model must error")
	}
}

func ptrFloat(f float64) *float64 { return &f }

func TestResolvePassesGenerationOptions(t *testing.T) {
	var got clientOptions
	reg := NewModelRegistryWithOptionsFactory(ProviderKeys{Gemini: "g"}, func(provider, modelName string, opts clientOptions) (model.StreamingModel, error) {
		got = opts
		return fakeClient{}, nil
	})
	temp := 0.4
	top := 0.7
	if _, err := reg.Resolve(RunSpec{
		Model: "gemini-2.5-flash", Temperature: &temp,
		TopP: &top, MaxOutputTokens: 1234, Reasoning: "high",
	}); err != nil {
		t.Fatal(err)
	}
	if got.Temperature == nil || *got.Temperature != 0.4 || got.TopP == nil || *got.TopP != 0.7 ||
		got.MaxOutputTokens != 1234 || got.Reasoning != "high" {
		t.Fatalf("factory options = %+v", got)
	}
}

func TestAutoFallbackReportsUsedModel(t *testing.T) {
	reg := NewModelRegistryWithOptionsFactory(ProviderKeys{Gemini: "g"}, func(provider, modelName string, opts clientOptions) (model.StreamingModel, error) {
		if modelName == "gemini-3.5-flash" {
			return failingClient{}, nil
		}
		return fakeClient{}, nil
	})
	client, err := reg.Resolve(RunSpec{Model: AutoModelID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GenerateStream(context.Background(), model.Request{}, func(model.Delta) error { return nil }); err != nil {
		t.Fatalf("fallback should succeed: %v", err)
	}
	used, ok := client.(interface{ UsedModel() string })
	if !ok || used.UsedModel() != "gemini-3.5-pro" {
		t.Fatalf("used model = %v, want gemini-3.5-pro", client)
	}
}

type failingClient struct{}

type retryableFailure struct{}

func (retryableFailure) Error() string   { return "nope" }
func (retryableFailure) Retryable() bool { return true }

var errRetryable = retryableFailure{}

func (failingClient) Generate(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, errRetryable
}

func (failingClient) GenerateStream(context.Context, model.Request, func(model.Delta) error) (model.Response, error) {
	return model.Response{}, errRetryable
}
