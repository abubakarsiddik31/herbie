package workflow

import (
	"encoding/json"
	"time"
)

// Node represents a single step on the workflow canvas.
type Node struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // e.g., manual, webhook, http_request, llm_prompt, condition
	Name     string         `json:"name"`
	Position NodePosition   `json:"position"`
	Data     map[string]any `json:"data"`
}

type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Edge represents a directional data connection between two nodes.
type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"` // e.g. "true", "false", "default"
	TargetHandle string `json:"targetHandle,omitempty"`
}

// WebhookResponse defines a custom HTTP response returned to a webhook caller.
type WebhookResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       any               `json:"body"`
}

// NodeExecutionResult is the recorded execution state of one node.
type NodeExecutionResult struct {
	NodeID     string    `json:"nodeId"`
	NodeName   string    `json:"nodeName"`
	NodeType   string    `json:"nodeType"`
	Status     string    `json:"status"` // success, failed, skipped
	Input      any       `json:"input,omitempty"`
	Output     any       `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	DurationMs int64     `json:"durationMs"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

// RunResult captures the complete execution trace of a workflow run.
type RunResult struct {
	RunID       string                         `json:"runId"`
	WorkflowID  string                         `json:"workflowId"`
	Status      string                         `json:"status"` // success, failed
	Input       any                            `json:"input"`
	Output      any                            `json:"output"`
	NodeResults map[string]NodeExecutionResult `json:"nodeResults"`
	Error       string                         `json:"error,omitempty"`
	DurationMs  int64                          `json:"durationMs"`
	Response    *WebhookResponse               `json:"response,omitempty"`
}

// StepEvent is emitted during workflow execution for live progress streaming.
type StepEvent struct {
	Type   string               `json:"type"` // node_start, node_finish, run_finish
	NodeID string               `json:"nodeId,omitempty"`
	Result *NodeExecutionResult `json:"result,omitempty"`
	Run    *RunResult           `json:"run,omitempty"`
}

// RawWorkflow is a helper for unmarshaling node and edge JSON arrays.
func ParseWorkflowGraph(nodesRaw, edgesRaw json.RawMessage) ([]Node, []Edge, error) {
	var nodes []Node
	if len(nodesRaw) > 0 {
		if err := json.Unmarshal(nodesRaw, &nodes); err != nil {
			return nil, nil, err
		}
	}
	var edges []Edge
	if len(edgesRaw) > 0 {
		if err := json.Unmarshal(edgesRaw, &edges); err != nil {
			return nil, nil, err
		}
	}
	return nodes, edges, nil
}
