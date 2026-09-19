package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/vault"
	"github.com/google/uuid"
)

// Engine executes a workflow graph given an input payload and environment.
type Engine struct {
	executors map[string]NodeExecutor
}

func NewEngine(customExecutors map[string]NodeExecutor) *Engine {
	merged := make(map[string]NodeExecutor, len(DefaultExecutors)+len(customExecutors))
	for k, v := range DefaultExecutors {
		merged[k] = v
	}
	for k, v := range customExecutors {
		merged[k] = v
	}
	return &Engine{executors: merged}
}

type RunOptions struct {
	WorkflowID    string
	RunID         string
	UserID        string
	TriggerSource string
	InputData     any
	Credentials   map[string]map[string]any
	Env           *ExecutionEnvironment
	Events        chan<- StepEvent
}

// Execute runs the workflow graph to completion, optionally streaming step events.
func (e *Engine) Execute(ctx context.Context, nodes []Node, edges []Edge, opts RunOptions) (*RunResult, error) {
	startTime := time.Now()
	if opts.RunID == "" {
		opts.RunID = uuid.NewString()
	}

	result := &RunResult{
		RunID:       opts.RunID,
		WorkflowID:  opts.WorkflowID,
		Status:      "running",
		Input:       opts.InputData,
		NodeResults: make(map[string]NodeExecutionResult),
	}

	if len(nodes) == 0 {
		result.Status = "success"
		result.Output = opts.InputData
		result.DurationMs = time.Since(startTime).Milliseconds()
		return result, nil
	}

	nodeMap := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	outgoing := make(map[string][]Edge)
	incoming := make(map[string][]Edge)
	for _, edge := range edges {
		outgoing[edge.Source] = append(outgoing[edge.Source], edge)
		incoming[edge.Target] = append(incoming[edge.Target], edge)
	}

	// Find starting nodes: triggers or nodes with no incoming edges
	var startNodes []Node
	for _, n := range nodes {
		if isTriggerType(n.Type) {
			startNodes = append(startNodes, n)
		}
	}
	if len(startNodes) == 0 {
		for _, n := range nodes {
			if len(incoming[n.ID]) == 0 {
				startNodes = append(startNodes, n)
			}
		}
	}
	if len(startNodes) == 0 && len(nodes) > 0 {
		startNodes = append(startNodes, nodes[0])
	}

	nodeOutputs := make(map[string]any)
	visited := make(map[string]bool)

	// Queue items: { nodeID, inputData }
	type queueItem struct {
		nodeID string
		input  any
	}
	var queue []queueItem
	for _, start := range startNodes {
		queue = append(queue, queueItem{nodeID: start.ID, input: opts.InputData})
	}

	var lastOutput any = opts.InputData
	credSecrets := CollectCredentialSecrets(opts.Credentials)

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			result.Status = "failed"
			result.Error = "execution cancelled: " + err.Error()
			break
		}

		curr := queue[0]
		queue = queue[1:]

		node, exists := nodeMap[curr.nodeID]
		if !exists || visited[node.ID] {
			continue
		}
		visited[node.ID] = true

		nodeStartTime := time.Now()

		nodeRes := NodeExecutionResult{
			NodeID:    node.ID,
			NodeName:  node.Name,
			NodeType:  node.Type,
			Status:    "running",
			Input:     curr.input,
			StartedAt: nodeStartTime,
		}

		if opts.Events != nil {
			select {
			case opts.Events <- StepEvent{Type: "node_start", NodeID: node.ID, Result: &nodeRes}:
			default:
			}
		}

		evalCtx := &EvalContext{
			JSON:        curr.input,
			Nodes:       nodeOutputs,
			Credentials: opts.Credentials,
		}

		executor, ok := e.executors[node.Type]
		if !ok {
			nodeRes.Status = "failed"
			nodeRes.Error = fmt.Sprintf("unsupported node type %q", node.Type)
			nodeRes.FinishedAt = time.Now()
			nodeRes.DurationMs = time.Since(nodeStartTime).Milliseconds()
			result.NodeResults[node.ID] = nodeRes
			result.Status = "failed"
			result.Error = nodeRes.Error
			if opts.Events != nil {
				select {
				case opts.Events <- StepEvent{Type: "node_finish", NodeID: node.ID, Result: &nodeRes}:
				default:
				}
			}
			break
		}

		output, nextHandle, execErr := executor(ctx, node, evalCtx, opts.Env)
		nodeRes.FinishedAt = time.Now()
		nodeRes.DurationMs = time.Since(nodeStartTime).Milliseconds()

		if execErr != nil {
			nodeRes.Status = "failed"
			cleanErr := vault.RedactSecrets(execErr.Error(), credSecrets)
			nodeRes.Error = cleanErr
			nodeRes.Output = vault.SanitizeValue(output, credSecrets)
			result.NodeResults[node.ID] = nodeRes
			result.Status = "failed"
			result.Error = cleanErr
			if opts.Events != nil {
				select {
				case opts.Events <- StepEvent{Type: "node_finish", NodeID: node.ID, Result: &nodeRes}:
				default:
				}
			}
			break
		}

		cleanOutput := vault.SanitizeValue(output, credSecrets)
		nodeRes.Status = "success"
		nodeRes.Output = cleanOutput
		result.NodeResults[node.ID] = nodeRes
		lastOutput = cleanOutput

		// Store output indexed by ID and by Name
		nodeOutputs[node.ID] = output
		if node.Name != "" {
			nodeOutputs[node.Name] = output
		}

		// Capture webhook_response if present
		if node.Type == "webhook_response" {
			if respMap, ok := output.(map[string]any); ok {
				status, _ := respMap["statusCode"].(int)
				headers, _ := respMap["headers"].(map[string]string)
				result.Response = &WebhookResponse{
					StatusCode: status,
					Headers:    headers,
					Body:       respMap["body"],
				}
			}
		}

		if opts.Events != nil {
			select {
			case opts.Events <- StepEvent{Type: "node_finish", NodeID: node.ID, Result: &nodeRes}:
			default:
			}
		}

		// Enqueue downstream nodes based on branching
		outEdges := outgoing[node.ID]
		for _, edge := range outEdges {
			// If branching handle is set (e.g. condition node returning "true" or "false")
			if nextHandle != "" && edge.SourceHandle != "" && edge.SourceHandle != nextHandle {
				continue
			}
			queue = append(queue, queueItem{nodeID: edge.Target, input: output})
		}
	}

	result.DurationMs = time.Since(startTime).Milliseconds()
	if result.Status == "running" {
		result.Status = "success"
	}
	result.Output = lastOutput

	if opts.Events != nil {
		select {
		case opts.Events <- StepEvent{Type: "run_finish", Run: result}:
		default:
		}
	}

	return result, nil
}

func isTriggerType(nodeType string) bool {
	switch nodeType {
	case "manual", "trigger_manual", "webhook", "trigger_webhook", "chat_agent", "trigger_chat":
		return true
	default:
		return false
	}
}
