package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/mcp"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem-chatbot/internal/workflow"
)

type createMCPServerRequest struct {
	Name      string            `json:"name"`
	URL       string            `json:"url"`
	Transport string            `json:"transport"`
	AppID     *string           `json:"appId,omitempty"`
	Headers   map[string]string `json:"headers"`
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPServers == nil {
		writeJSON(w, http.StatusOK, map[string]any{"servers": []any{}})
		return
	}
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}

	servers, err := s.deps.MCPServers.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list mcp servers")
		return
	}
	if servers == nil {
		servers = []storage.MCPServer{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (s *Server) handleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPServers == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "mcp servers not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}

	var req createMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	name := strings.TrimSpace(req.Name)
	rawURL := strings.TrimSpace(req.URL)
	if name == "" || rawURL == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name and url are required")
		return
	}

	if _, err := workflow.ValidatePublicURL(r.Context(), rawURL, s.deps.Cfg.ToolAllowPrivateHosts); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}

	var appIDPtr *string
	if req.AppID != nil {
		cleanAppID := strings.ToLower(strings.TrimSpace(*req.AppID))
		if cleanAppID != "" && cleanAppID != "none" && cleanAppID != "custom" {
			appIDPtr = &cleanAppID
			_ = s.deps.MCPServers.UnlinkApp(r.Context(), cleanAppID, userID)
		}
	}

	headersBytes, _ := json.Marshal(req.Headers)
	server, err := s.deps.MCPServers.Create(r.Context(), storage.MCPServer{
		UserID:    userID,
		Name:      name,
		URL:       rawURL,
		Transport: "http",
		AppID:     appIDPtr,
		Enabled:   true,
		Headers:   headersBytes,
	})
	if err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			writeError(w, http.StatusConflict, "conflict", "an mcp server with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not save mcp server")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"server": server})
}

func (s *Server) handleUpdateMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPServers == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "mcp servers not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	id := r.PathValue("id")
	existing, err := s.deps.MCPServers.ByID(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "mcp server not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "lookup error")
		return
	}

	var req createMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		existing.Name = name
	}
	if rawURL := strings.TrimSpace(req.URL); rawURL != "" {
		if _, err := workflow.ValidatePublicURL(r.Context(), rawURL, s.deps.Cfg.ToolAllowPrivateHosts); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}
		existing.URL = rawURL
	}
	if req.Transport != "" {
		existing.Transport = req.Transport
	}
	if req.Headers != nil {
		headersBytes, _ := json.Marshal(req.Headers)
		existing.Headers = headersBytes
	}
	if req.AppID != nil {
		cleanAppID := strings.ToLower(strings.TrimSpace(*req.AppID))
		if cleanAppID == "" || cleanAppID == "none" || cleanAppID == "custom" {
			existing.AppID = nil
		} else {
			existing.AppID = &cleanAppID
			_ = s.deps.MCPServers.UnlinkApp(r.Context(), cleanAppID, userID)
		}
	}

	updated, err := s.deps.MCPServers.Update(r.Context(), existing)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update mcp server")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"server": updated})
}

func (s *Server) handleUnlinkAppMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPServers == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "mcp servers not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	id := r.PathValue("id")
	srv, err := s.deps.MCPServers.ByID(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "mcp server not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "lookup error")
		return
	}

	srv.AppID = nil
	updated, err := s.deps.MCPServers.Update(r.Context(), srv)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not unlink app")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"server": updated})
}

func (s *Server) handleToggleMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPServers == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "mcp servers not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")

	existing, err := s.deps.MCPServers.ByID(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "mcp server not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "lookup error")
		return
	}

	existing.Enabled = !existing.Enabled
	updated, err := s.deps.MCPServers.Update(r.Context(), existing)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not toggle mcp server")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"server": updated})
}

func (s *Server) handleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPServers == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "mcp servers not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")

	if err := s.deps.MCPServers.Delete(r.Context(), id, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "mcp server not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "delete error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	var req createMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "url is required")
		return
	}

	if _, err := workflow.ValidatePublicURL(r.Context(), rawURL, s.deps.Cfg.ToolAllowPrivateHosts); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}

	safeClient := workflow.NewSafeHTTPClient(s.deps.Cfg.ToolAllowPrivateHosts, 10*time.Second)
	client := mcp.NewClient(rawURL, safeClient, req.Headers)

	tools, err := client.ListTools(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "mcp_error", fmt.Sprintf("Failed to reach MCP server: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"count": len(tools),
		"tools": tools,
	})
}

// mcpHerbieToolProvider adapts Herbie's apps to the MCP ToolProvider interface.
type mcpHerbieToolProvider struct {
	server *Server
	userID string
}

func (p *mcpHerbieToolProvider) ListMCPTools(ctx context.Context) ([]mcp.Tool, error) {
	tools := []mcp.Tool{
		{
			Name:        "google_calendar",
			Description: "Access Google Calendar to check agenda, view events, and create calendar meetings.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"action": {"type": "string", "enum": ["list_events", "create_event", "delete_event"]},
					"summary": {"type": "string"},
					"start_time": {"type": "string"},
					"end_time": {"type": "string"}
				},
				"required": ["action"]
			}`),
		},
		{
			Name:        "web_search",
			Description: "Search the public web for current events, live information, and external facts.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string"}
				},
				"required": ["query"]
			}`),
		},
	}

	if p.server.deps.Workflows != nil && p.userID != "" {
		activeFlows, err := p.server.deps.Workflows.ListActiveTools(ctx, p.userID)
		if err == nil {
			for _, flow := range activeFlows {
				name := flow.ToolName
				if name == "" {
					name = "workflow_" + strings.ToLower(strings.ReplaceAll(flow.Name, " ", "_"))
				}
				tools = append(tools, mcp.Tool{
					Name:        sanitizeToolName(name),
					Description: flow.Description,
					InputSchema: json.RawMessage(`{
						"type": "object",
						"properties": {
							"input": {"type": "string"}
						}
					}`),
				})
			}
		}
	}

	return tools, nil
}

func (p *mcpHerbieToolProvider) CallMCPTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	switch name {
	case "google_calendar":
		calClient := p.server.calendarClient(ctx, p.userID)
		tl := chat.GoogleCalendarTool(calClient)
		res, err := tl.Exec(ctx, chat.Deps{UserID: p.userID}, args)
		if err != nil {
			return "", err
		}
		return res.Text, nil

	case "web_search":
		if p.server.deps.WebSearch == nil {
			return "Web search not available", nil
		}
		var a struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(args, &a)
		results, err := p.server.deps.WebSearch.Search(ctx, a.Query)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		for i, r := range results {
			fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i+1, r.Title, r.Snippet)
		}
		return strings.TrimSpace(sb.String()), nil

	default:
		return "", fmt.Errorf("tool %q not supported via MCP", name)
	}
}

func (s *Server) handleMCPEndpoint(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot read body")
		return
	}

	provider := &mcpHerbieToolProvider{server: s, userID: userID}
	mcpServer := mcp.NewServer(provider, s.deps.Log)
	res := mcpServer.Handle(r.Context(), body)

	writeJSON(w, http.StatusOK, res)
}
