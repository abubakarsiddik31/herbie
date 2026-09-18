package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// RememberToolName is the identifier for the long-term memory tool.
const RememberToolName = "remember"

var rememberSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "fact": {
      "type": "string",
      "description": "The key personal fact, preference, or detail to remember about the user across conversations."
    }
  },
  "required": ["fact"]
}`)

// RememberTool allows the assistant to save facts or preferences into the user's long-term cross-chat memory.
func RememberTool() tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        RememberToolName,
		Description: "Save a key personal fact, preference, or detail about the user to long-term memory. Use when the user explicitly asks you to remember something or shares an enduring detail about themselves.",
		Schema:      rememberSchema,
		Timeout:     10 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			var a struct {
				Fact string `json:"fact"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Fact) == "" {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a non-empty "fact" string`)}
			}
			fact := strings.TrimSpace(a.Fact)
			if deps.SaveMemory == nil {
				return tool.Text("Memory storage is not available."), nil
			}
			if err := deps.SaveMemory(ctx, fact); err != nil {
				return tool.Result{}, fmt.Errorf("save memory: %w", err)
			}
			return tool.Text("Saved to long-term memory: " + fact), nil
		},
	}
}
