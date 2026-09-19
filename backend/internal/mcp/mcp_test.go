package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockToolProvider struct {
	tools []Tool
	calls map[string]json.RawMessage
}

func (m *mockToolProvider) ListMCPTools(_ context.Context) ([]Tool, error) {
	return m.tools, nil
}

func (m *mockToolProvider) CallMCPTool(_ context.Context, name string, args json.RawMessage) (string, error) {
	m.calls[name] = args
	if name == "error_tool" {
		return "", fmt.Errorf("mock error")
	}
	return "result from " + name, nil
}

func TestMCPServer_InitializeAndTools(t *testing.T) {
	provider := &mockToolProvider{
		tools: []Tool{
			{
				Name:        "google_calendar",
				Description: "Calendar tool",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
		calls: make(map[string]json.RawMessage),
	}

	server := NewServer(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// 1. Initialize
	initReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	res := server.Handle(context.Background(), initReq)
	if res.Error != nil {
		t.Fatalf("unexpected init error: %+v", res.Error)
	}
	initRes, ok := res.Result.(InitializeResult)
	if !ok || initRes.ServerInfo.Name != "herbie" {
		t.Fatalf("unexpected init result: %+v", res.Result)
	}

	// 2. Tools List
	listReq := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	res = server.Handle(context.Background(), listReq)
	if res.Error != nil {
		t.Fatalf("unexpected list error: %+v", res.Error)
	}
	listRes, ok := res.Result.(ListToolsResult)
	if !ok || len(listRes.Tools) != 1 || listRes.Tools[0].Name != "google_calendar" {
		t.Fatalf("unexpected list result: %+v", res.Result)
	}

	// 3. Tools Call
	callReq := []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"google_calendar","arguments":{"action":"list_events"}}}`)
	res = server.Handle(context.Background(), callReq)
	if res.Error != nil {
		t.Fatalf("unexpected call error: %+v", res.Error)
	}
	callRes, ok := res.Result.(CallToolResult)
	if !ok || callRes.IsError || len(callRes.Content) == 0 || callRes.Content[0].Text != "result from google_calendar" {
		t.Fatalf("unexpected call result: %+v", res.Result)
	}
}

func TestMCPClient_ListAndCall(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: ListToolsResult{
					Tools: []Tool{
						{
							Name:        "sqlite_query",
							Description: "Execute SQL",
							InputSchema: json.RawMessage(`{"type":"object"}`),
						},
					},
				},
			})
		case "tools/call":
			var p CallToolParams
			_ = json.Unmarshal(req.Params, &p)
			if p.Name == "sqlite_query" {
				_ = json.NewEncoder(w).Encode(JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: CallToolResult{
						Content: []ContentItem{{Type: "text", Text: "row count: 42"}},
					},
				})
				return
			}
			http.Error(w, "unknown tool", http.StatusBadRequest)
		default:
			http.Error(w, "unknown method", http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, ts.Client(), nil)

	// List tools
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "sqlite_query" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	// Call tool
	out, err := client.CallTool(context.Background(), "sqlite_query", json.RawMessage(`{"query":"SELECT 1"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if out != "row count: 42" {
		t.Fatalf("unexpected output: %s", out)
	}
}
