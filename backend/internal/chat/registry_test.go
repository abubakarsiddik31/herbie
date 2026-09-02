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
