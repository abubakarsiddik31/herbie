package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
	"github.com/abubakarsiddik31/golem/tokens"
)

func TestCountInputTokensUsesProviderCounter(t *testing.T) {
	reg := NewModelRegistryWithCounterFactory(ProviderKeys{}, func(provider, modelName string) (tokens.Counter, error) {
		if provider != "gemini" || modelName != "gemini-3.5-flash" {
			t.Fatalf("counter asked for %q/%q", provider, modelName)
		}
		return testmodel.CountFunc(func(_ context.Context, in tokens.CountInput) (int, error) {
			return len(in.Messages) * 10, nil
		}), nil
	})
	n, err := reg.CountInputTokens(context.Background(), RunSpec{Model: "gemini-3.5-flash"},
		[]model.Message{{Role: model.RoleUser, Content: "hi"}}, nil)
	if err != nil || n != 10 {
		t.Fatalf("count = %d err=%v", n, err)
	}
}

func TestCountInputTokensUnavailable(t *testing.T) {
	reg := NewModelRegistry(ProviderKeys{})
	if _, err := reg.CountInputTokens(context.Background(), RunSpec{Model: "gemini-3.5-flash"}, nil, nil); !errors.Is(err, ErrNoTokenCounter) {
		t.Fatalf("missing key = %v", err)
	}
	if _, err := reg.CountInputTokens(context.Background(), RunSpec{Model: AutoModelID}, nil, nil); !errors.Is(err, ErrNoTokenCounter) {
		t.Fatalf("auto = %v", err)
	}
}
