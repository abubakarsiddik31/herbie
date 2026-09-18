package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRememberTool(t *testing.T) {
	var saved string
	deps := Deps{
		SaveMemory: func(_ context.Context, fact string) error {
			saved = fact
			return nil
		},
	}

	tool := RememberTool()
	out, err := tool.Exec(context.Background(), deps, json.RawMessage(`{"fact":"prefers dark mode"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if saved != "prefers dark mode" {
		t.Fatalf("expected saved fact 'prefers dark mode', got %q", saved)
	}
	if !strings.Contains(out.Text, "Saved to long-term memory: prefers dark mode") {
		t.Fatalf("unexpected out: %s", out.Text)
	}
}

func TestRememberToolError(t *testing.T) {
	deps := Deps{
		SaveMemory: func(_ context.Context, _ string) error {
			return errors.New("db down")
		},
	}
	tool := RememberTool()
	_, err := tool.Exec(context.Background(), deps, json.RawMessage(`{"fact":"likes cats"}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRememberToolNilHandler(t *testing.T) {
	tool := RememberTool()
	out, err := tool.Exec(context.Background(), Deps{}, json.RawMessage(`{"fact":"something"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "Memory storage is not available." {
		t.Fatalf("unexpected out: %s", out.Text)
	}
}
