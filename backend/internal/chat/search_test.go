package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

func TestSearchToolSchema(t *testing.T) {
	tl := SearchTool()
	var schema map[string]any
	if err := json.Unmarshal(tl.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema["additionalProperties"]; ok {
		t.Fatal("Gemini rejects additionalProperties in tool schemas")
	}
	props := schema["properties"].(map[string]any)
	if props["query"] == nil || props["k"] == nil {
		t.Fatal("query/k properties missing")
	}
	if tl.Name != SearchToolName {
		t.Fatalf("name %q", tl.Name)
	}
}

func searchDeps(captured *[]string) Deps {
	return Deps{Search: func(_ context.Context, query string, k int) ([]rag.Scored, error) {
		*captured = append(*captured, query+"/"+strconv.Itoa(k))
		return []rag.Scored{
			{Chunk: rag.Chunk{DocTitle: "a.txt", Index: 0, Content: "alpha text"}, Score: 0.9},
			{Chunk: rag.Chunk{DocTitle: "a.txt", Index: 1, Content: strings.Repeat("long ", 100)}, Score: 0.5},
		}, nil
	}}
}

func TestSearchToolExec(t *testing.T) {
	var captured []string
	out, err := SearchTool().Exec(context.Background(), searchDeps(&captured), json.RawMessage(`{"query":"q","k":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.Text, "[1] (a.txt) alpha text\n\n[2] (a.txt) long ") {
		t.Fatalf("formatted results: %q", out.Text)
	}
	if captured[0] != "q/3" {
		t.Fatalf("args not passed: %q", captured[0])
	}
}

func TestSearchToolNoResults(t *testing.T) {
	deps := Deps{Search: func(_ context.Context, _ string, _ int) ([]rag.Scored, error) { return nil, nil }}
	out, err := SearchTool().Exec(context.Background(), deps, json.RawMessage(`{"query":"q"}`))
	if err != nil || out.Text != "No matching documents." {
		t.Fatalf("out=%q err=%v", out.Text, err)
	}
}

func TestSearchToolDisabled(t *testing.T) {
	out, err := SearchTool().Exec(context.Background(), Deps{}, json.RawMessage(`{"query":"q"}`))
	if err != nil || out.Text != "No documents are available." {
		t.Fatalf("out=%q err=%v", out.Text, err)
	}
}

func TestSearchToolClampsK(t *testing.T) {
	for args, want := range map[string]int{`{"query":"q","k":99}`: 20, `{"query":"q","k":0}`: 5, `{"query":"q","k":-3}`: 5} {
		var got int
		deps := Deps{Search: func(_ context.Context, _ string, k int) ([]rag.Scored, error) {
			got = k
			return nil, nil
		}}
		if _, err := SearchTool().Exec(context.Background(), deps, json.RawMessage(args)); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s: k=%d want %d", args, got, want)
		}
	}
}

func TestSearchToolEmptyQueryRetries(t *testing.T) {
	_, err := SearchTool().Exec(context.Background(), searchDeps(&[]string{}), json.RawMessage(`{"query":"  "}`))
	var retry *model.ModelRetry
	if !errors.As(err, &retry) {
		t.Fatalf("want ModelRetry, got %v", err)
	}
}

func TestSearchToolErrorPropagates(t *testing.T) {
	deps := Deps{Search: func(_ context.Context, _ string, _ int) ([]rag.Scored, error) {
		return nil, errors.New("index down")
	}}
	_, err := SearchTool().Exec(context.Background(), deps, json.RawMessage(`{"query":"q"}`))
	if err == nil || !strings.Contains(err.Error(), "index down") {
		t.Fatalf("want wrapped error, got %v", err)
	}
}

func TestPromptForCitations(t *testing.T) {
	spec := RunSpec{SystemPrompt: "custom"}
	if got := promptFor(spec, nil); got != "custom" {
		t.Fatalf("plain prompt: %q", got)
	}
	withSearch := promptFor(RunSpec{}, []tool.Tool[Deps]{SearchTool()})
	if !strings.HasPrefix(withSearch, systemPrompt) || !strings.Contains(withSearch, "cite sources inline") {
		t.Fatalf("citation rules missing: %q", withSearch)
	}
	custom := promptFor(spec, []tool.Tool[Deps]{SearchTool()})
	if !strings.HasPrefix(custom, "custom") || !strings.Contains(custom, "cite sources inline") {
		t.Fatalf("custom prompt lost citation rules: %q", custom)
	}
}
