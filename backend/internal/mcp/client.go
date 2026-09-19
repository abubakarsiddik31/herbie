package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client executes MCP methods against an external Model Context Protocol server.
type Client struct {
	URL        string
	HTTPClient *http.Client
	Headers    map[string]string
}

func NewClient(endpoint string, httpClient *http.Client, headers map[string]string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		URL:        endpoint,
		HTTPClient: httpClient,
		Headers:    headers,
	}
}

func (c *Client) call(ctx context.Context, method string, params any, resultDst any) error {
	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
		paramsRaw = b
	}

	rpcReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      time.Now().UnixNano(),
		Method:  method,
		Params:  paramsRaw,
	}

	bodyBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return fmt.Errorf("marshal rpc request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("mcp http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mcp server returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode rpc response: %w", err)
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("mcp rpc error (%d): %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	if resultDst != nil && rpcResp.Result != nil {
		resBytes, err := json.Marshal(rpcResp.Result)
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		if err := json.Unmarshal(resBytes, resultDst); err != nil {
			return fmt.Errorf("unmarshal result into destination: %w", err)
		}
	}

	return nil
}

// ListTools queries the MCP server for advertised tools.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var res ListToolsResult
	if err := c.call(ctx, "tools/list", nil, &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// CallTool executes a tool on the external MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var res CallToolResult
	params := CallToolParams{
		Name:      name,
		Arguments: args,
	}
	if err := c.call(ctx, "tools/call", params, &res); err != nil {
		return "", err
	}
	if res.IsError {
		errMsg := "MCP tool execution error"
		if len(res.Content) > 0 && res.Content[0].Text != "" {
			errMsg = res.Content[0].Text
		}
		return "", fmt.Errorf("%s", errMsg)
	}
	if len(res.Content) == 0 {
		return "", nil
	}
	return res.Content[0].Text, nil
}
