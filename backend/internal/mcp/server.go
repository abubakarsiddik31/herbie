package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// ToolProvider provides tools for the MCP server to advertise and execute.
type ToolProvider interface {
	ListMCPTools(ctx context.Context) ([]Tool, error)
	CallMCPTool(ctx context.Context, name string, args json.RawMessage) (string, error)
}

// Server handles Model Context Protocol JSON-RPC requests.
type Server struct {
	provider ToolProvider
	log      *slog.Logger
}

func NewServer(provider ToolProvider, log *slog.Logger) *Server {
	return &Server{
		provider: provider,
		log:      log,
	}
}

// Handle processes an incoming JSON-RPC 2.0 MCP request body.
func (s *Server) Handle(ctx context.Context, reqBytes []byte) JSONRPCResponse {
	var req JSONRPCRequest
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &JSONRPCError{
				Code:    ErrCodeParse,
				Message: "Parse error: invalid JSON",
			},
		}
	}

	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		res.Result = InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{ListChanged: false},
			},
			ServerInfo: Implementation{
				Name:    "herbie",
				Version: "0.1.0",
			},
		}

	case "notifications/initialized":
		res.Result = true

	case "ping":
		res.Result = map[string]any{}

	case "tools/list":
		if s.provider == nil {
			res.Result = ListToolsResult{Tools: []Tool{}}
			break
		}
		tools, err := s.provider.ListMCPTools(ctx)
		if err != nil {
			res.Error = &JSONRPCError{
				Code:    ErrCodeInternal,
				Message: fmt.Sprintf("Failed to list tools: %v", err),
			}
			break
		}
		res.Result = ListToolsResult{Tools: tools}

	case "tools/call":
		if s.provider == nil {
			res.Error = &JSONRPCError{
				Code:    ErrCodeInternal,
				Message: "No tool provider configured",
			}
			break
		}
		var p CallToolParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			res.Error = &JSONRPCError{
				Code:    ErrCodeInvalidParams,
				Message: "Invalid params: 'name' is required",
			}
			break
		}
		out, err := s.provider.CallMCPTool(ctx, p.Name, p.Arguments)
		if err != nil {
			res.Result = CallToolResult{
				Content: []ContentItem{{Type: "text", Text: err.Error()}},
				IsError: true,
			}
			break
		}
		res.Result = CallToolResult{
			Content: []ContentItem{{Type: "text", Text: out}},
			IsError: false,
		}

	default:
		res.Error = &JSONRPCError{
			Code:    ErrCodeMethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
	}

	return res
}
