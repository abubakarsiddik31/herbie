package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/abubakarsiddik31/golem/tool"
)

// BuildTools turns validated user tool configs into golem tools: one
// tool.Tool per config, advertising the config's schema and executing the
// configured HTTP request. Approval-gated tools defer instead of executing
// until golem re-runs them with the approved marker set.
func BuildTools(cfgs []ToolConfig, env ToolEnv) ([]tool.Tool[Deps], error) {
	client := newToolHTTPClient(env)
	tools := make([]tool.Tool[Deps], 0, len(cfgs))
	for _, cfg := range cfgs {
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("tool %q: %w", cfg.Name, err)
		}
		schema, err := cfg.Schema()
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", cfg.Name, err)
		}
		created := cfg
		tools = append(tools, tool.Tool[Deps]{
			Name:        created.Name,
			Description: created.Description,
			Schema:      schema,
			// The tool timeout bounds the approved re-run too; keep headroom
			// over the HTTP client's own per-request budget.
			Timeout:    env.HTTPTimeout + 10*time.Second,
			MaxRetries: tool.RetryLimit(2),
			Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
				if created.RequireApproval && !tool.CallApproved(ctx) {
					return tool.Result{}, &tool.Deferred{Kind: tool.DeferApproval, Reason: "user approval required"}
				}
				out, err := executeHTTPTool(ctx, created, env, client, args)
				return tool.Text(out), err
			},
		})
	}
	return tools, nil
}
