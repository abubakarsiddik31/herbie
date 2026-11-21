package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "Deploy Notes", "Deploy Notes"},
		{"quotes and stop", `"Deploy Notes."`, "Deploy Notes"},
		{"curly quotes", "“Weekly Planning”", "Weekly Planning"},
		{"first line only", "Deploy notes\nIgnore this", "Deploy notes"},
		{"collapses whitespace", "  Deploy   the\tapp  ", "Deploy the app"},
		{"trailing punctuation", "Is it ready?!", "Is it ready"},
		{"empty", "  \"\"  ", ""},
		{"caps length", strings.Repeat("a", maxTitleRunes+20), strings.Repeat("a", maxTitleRunes)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanTitle(tt.in); got != tt.want {
				t.Fatalf("CleanTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTitleGenerator(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: `"Deploy Notes."`},
		Usage:   model.Usage{InputTokens: 12, OutputTokens: 4},
	})
	registry := NewModelRegistryWithFactory(ProviderKeys{Gemini: "test"},
		func(string, string, *float64) (model.StreamingModel, error) { return m, nil })
	g := NewTitleGenerator(registry, "gemini-2.5-flash")

	title, usage, err := g.Generate(context.Background(), "deploy the app to production")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Deploy Notes" || usage.InputTokens != 12 || usage.OutputTokens != 4 {
		t.Fatalf("title=%q usage=%+v", title, usage)
	}
	requests := m.Requests()
	if len(requests) != 1 || len(requests[0].Messages) != 1 {
		t.Fatalf("requests = %+v", requests)
	}
	if !strings.Contains(requests[0].Messages[0].Content, "deploy the app to production") {
		t.Fatalf("title prompt missing user input: %q", requests[0].Messages[0].Content)
	}
}

func TestTitleGeneratorUnconfigured(t *testing.T) {
	if _, _, err := NewTitleGenerator(nil, "").Generate(context.Background(), "x"); err == nil {
		t.Fatal("unconfigured generator must fail")
	}
}
