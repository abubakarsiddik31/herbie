package chat

import (
	"context"
	"errors"
	"strings"

	"github.com/abubakarsiddik31/golem/model"
)

const (
	maxTitleRunes = 60
	titlePrompt   = "Write a short title (3-6 words) for a conversation that starts with the user message below. " +
		"Reply with the title only: no quotes, no trailing punctuation, no explanation.\n\n"
)

// TitleGenerator produces one conversation title. The model is resolved from
// the registry at request time, so a TITLE_MODEL change needs no restart-time
// wiring and a missing provider key fails only that call (the heuristic title
// stays).
type TitleGenerator struct {
	registry  *ModelRegistry
	modelName string
}

func NewTitleGenerator(registry *ModelRegistry, modelName string) *TitleGenerator {
	return &TitleGenerator{registry: registry, modelName: modelName}
}

// ModelName reports the configured title model for usage metering.
func (g *TitleGenerator) ModelName() string {
	if g == nil {
		return ""
	}
	return g.modelName
}

// Generate asks the title model for a short title. Callers pass sanitized
// first-user content (no file blocks, memories, or instructions).
func (g *TitleGenerator) Generate(ctx context.Context, input string) (string, model.Usage, error) {
	if g == nil || g.registry == nil || g.modelName == "" {
		return "", model.Usage{}, errors.New("title model is not configured")
	}
	client, err := g.registry.Resolve(RunSpec{Model: g.modelName})
	if err != nil {
		return "", model.Usage{}, err
	}
	resp, err := client.Generate(ctx, model.Request{
		Messages: []model.Message{{Role: model.RoleUser, Content: titlePrompt + input}},
	})
	if err != nil {
		return "", model.Usage{}, err
	}
	title := CleanTitle(resp.Message.Content)
	if title == "" {
		return "", resp.Usage, errors.New("model returned an empty title")
	}
	return title, resp.Usage, nil
}

// CleanTitle keeps the first line, strips wrapping quotes and trailing
// punctuation, collapses whitespace, and caps the length.
func CleanTitle(raw string) string {
	line := strings.TrimSpace(strings.SplitN(raw, "\n", 2)[0])
	line = strings.Trim(line, "\"'“”‘’` ")
	line = strings.TrimRight(line, ".?!:;, ")
	line = strings.Join(strings.Fields(line), " ")
	if r := []rune(line); len(r) > maxTitleRunes {
		line = strings.TrimSpace(string(r[:maxTitleRunes]))
	}
	return line
}
