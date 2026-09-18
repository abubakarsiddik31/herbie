package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem-chatbot/internal/workflow"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/google/uuid"
)

type WorkflowStore interface {
	Create(ctx context.Context, w storage.Workflow) (storage.Workflow, error)
	Get(ctx context.Context, id, userID string) (storage.Workflow, error)
	GetByWebhookSlug(ctx context.Context, slug string) (storage.Workflow, error)
	List(ctx context.Context, userID string) ([]storage.Workflow, error)
	ListActiveTools(ctx context.Context, userID string) ([]storage.Workflow, error)
	Update(ctx context.Context, id, userID string, patch storage.WorkflowPatch) (storage.Workflow, error)
	Delete(ctx context.Context, id, userID string) error

	CreateRun(ctx context.Context, r storage.WorkflowRun) (storage.WorkflowRun, error)
	GetRun(ctx context.Context, id, userID string) (storage.WorkflowRun, error)
	ListRuns(ctx context.Context, workflowID, userID string, limit int) ([]storage.WorkflowRun, error)
	UpdateRun(ctx context.Context, r storage.WorkflowRun) error

	CreateCredential(ctx context.Context, c storage.WorkflowCredential) (storage.WorkflowCredential, error)
	GetCredential(ctx context.Context, id, userID string) (storage.WorkflowCredential, error)
	ListCredentials(ctx context.Context, userID string) ([]storage.WorkflowCredential, error)
	UpdateCredential(ctx context.Context, c storage.WorkflowCredential) (storage.WorkflowCredential, error)
	DeleteCredential(ctx context.Context, id, userID string) error
}

var _ WorkflowStore = (*storage.Workflows)(nil)

type workflowDTO struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	TriggerType     string          `json:"triggerType"`
	WebhookSlug     *string         `json:"webhookSlug"`
	WebhookSecret   string          `json:"webhookSecret,omitempty"`
	Nodes           json.RawMessage `json:"nodes"`
	Edges           json.RawMessage `json:"edges"`
	ExposeAsTool    bool            `json:"exposeAsTool"`
	ToolName        string          `json:"toolName"`
	ToolDescription string          `json:"toolDescription"`
	IsActive        bool            `json:"isActive"`
	CreatedAt       string          `json:"createdAt"`
	UpdatedAt       string          `json:"updatedAt"`
}

func toWorkflowDTO(w storage.Workflow) workflowDTO {
	return workflowDTO{
		ID:              w.ID,
		Name:            w.Name,
		Description:     w.Description,
		TriggerType:     w.TriggerType,
		WebhookSlug:     w.WebhookSlug,
		WebhookSecret:   w.WebhookSecret,
		Nodes:           w.Nodes,
		Edges:           w.Edges,
		ExposeAsTool:    w.ExposeAsTool,
		ToolName:        w.ToolName,
		ToolDescription: w.ToolDescription,
		IsActive:        w.IsActive,
		CreatedAt:       w.CreatedAt.UTC().Format(timeRFC3339),
		UpdatedAt:       w.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

type workflowRunDTO struct {
	ID            string          `json:"id"`
	WorkflowID    string          `json:"workflowId"`
	Status        string          `json:"status"`
	TriggerSource string          `json:"triggerSource"`
	InputData     json.RawMessage `json:"inputData"`
	OutputData    json.RawMessage `json:"outputData"`
	NodeResults   json.RawMessage `json:"nodeResults"`
	Error         *string         `json:"error"`
	DurationMs    int64           `json:"durationMs"`
	CreatedAt     string          `json:"createdAt"`
	FinishedAt    *string         `json:"finishedAt"`
}

func toWorkflowRunDTO(r storage.WorkflowRun) workflowRunDTO {
	var finishedStr *string
	if r.FinishedAt != nil {
		s := r.FinishedAt.UTC().Format(timeRFC3339)
		finishedStr = &s
	}
	return workflowRunDTO{
		ID:            r.ID,
		WorkflowID:    r.WorkflowID,
		Status:        r.Status,
		TriggerSource: r.TriggerSource,
		InputData:     r.InputData,
		OutputData:    r.OutputData,
		NodeResults:   r.NodeResults,
		Error:         r.Error,
		DurationMs:    r.DurationMs,
		CreatedAt:     r.CreatedAt.UTC().Format(timeRFC3339),
		FinishedAt:    finishedStr,
	}
}

type credentialDTO struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Data      map[string]string `json:"data"` // masked
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

func toCredentialDTO(c storage.WorkflowCredential) credentialDTO {
	data := map[string]string{}
	_ = json.Unmarshal(c.Data, &data)
	masked := make(map[string]string, len(data))
	for k := range data {
		masked[k] = headerMask
	}
	return credentialDTO{
		ID:        c.ID,
		Name:      c.Name,
		Type:      c.Type,
		Data:      masked,
		CreatedAt: c.CreatedAt.UTC().Format(timeRFC3339),
		UpdatedAt: c.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

// Handler implementations

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeJSON(w, http.StatusOK, map[string]any{"workflows": []workflowDTO{}})
		return
	}
	userID, _ := userIDFrom(r.Context())
	list, err := s.deps.Workflows.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list workflows")
		return
	}
	dtos := make([]workflowDTO, len(list))
	for i, item := range list {
		dtos[i] = toWorkflowDTO(item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": dtos})
}

func (s *Server) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "workflows not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Name            string          `json:"name"`
		Description     string          `json:"description"`
		TriggerType     string          `json:"triggerType"`
		WebhookSlug     *string         `json:"webhookSlug"`
		WebhookSecret   string          `json:"webhookSecret"`
		Nodes           json.RawMessage `json:"nodes"`
		Edges           json.RawMessage `json:"edges"`
		ExposeAsTool    bool            `json:"exposeAsTool"`
		ToolName        string          `json:"toolName"`
		ToolDescription string          `json:"toolDescription"`
		IsActive        bool            `json:"isActive"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "malformed request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "workflow name is required")
		return
	}

	created, err := s.deps.Workflows.Create(r.Context(), storage.Workflow{
		UserID:          userID,
		Name:            req.Name,
		Description:     req.Description,
		TriggerType:     req.TriggerType,
		WebhookSlug:     req.WebhookSlug,
		WebhookSecret:   req.WebhookSecret,
		Nodes:           req.Nodes,
		Edges:           req.Edges,
		ExposeAsTool:    req.ExposeAsTool,
		ToolName:        req.ToolName,
		ToolDescription: req.ToolDescription,
		IsActive:        req.IsActive,
	})
	if err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate_webhook_slug", "webhook slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not create workflow")
		return
	}
	writeJSON(w, http.StatusCreated, toWorkflowDTO(created))
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "workflow not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	wf, err := s.deps.Workflows.Get(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not get workflow")
		return
	}
	writeJSON(w, http.StatusOK, toWorkflowDTO(wf))
}

func (s *Server) handlePatchWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "workflow not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")

	var req struct {
		Name            *string          `json:"name"`
		Description     *string          `json:"description"`
		TriggerType     *string          `json:"triggerType"`
		WebhookSlug     *string          `json:"webhookSlug"`
		WebhookSecret   *string          `json:"webhookSecret"`
		Nodes           *json.RawMessage `json:"nodes"`
		Edges           *json.RawMessage `json:"edges"`
		ExposeAsTool    *bool            `json:"exposeAsTool"`
		ToolName        *string          `json:"toolName"`
		ToolDescription *string          `json:"toolDescription"`
		IsActive        *bool            `json:"isActive"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "malformed request body")
		return
	}

	updated, err := s.deps.Workflows.Update(r.Context(), id, userID, storage.WorkflowPatch{
		Name:            req.Name,
		Description:     req.Description,
		TriggerType:     req.TriggerType,
		WebhookSlug:     req.WebhookSlug,
		WebhookSecret:   req.WebhookSecret,
		Nodes:           req.Nodes,
		Edges:           req.Edges,
		ExposeAsTool:    req.ExposeAsTool,
		ToolName:        req.ToolName,
		ToolDescription: req.ToolDescription,
		IsActive:        req.IsActive,
	})
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "workflow not found")
			return
		}
		if errors.Is(err, storage.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate_webhook_slug", "webhook slug already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update workflow")
		return
	}
	writeJSON(w, http.StatusOK, toWorkflowDTO(updated))
}

func (s *Server) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "workflow not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	if err := s.deps.Workflows.Delete(r.Context(), id, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete workflow")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "workflows not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")

	wf, err := s.deps.Workflows.Get(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not get workflow")
		return
	}

	var req struct {
		Input  any  `json:"input"`
		Stream bool `json:"stream"`
	}
	_ = decodeJSON(r, &req)
	if req.Input == nil {
		req.Input = map[string]any{"manual": true}
	}

	nodes, edges, err := workflow.ParseWorkflowGraph(wf.Nodes, wf.Edges)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_graph", "could not parse nodes/edges")
		return
	}

	// Fetch user credentials
	credList, _ := s.deps.Workflows.ListCredentials(r.Context(), userID)
	credentialsMap := make(map[string]map[string]any, len(credList))
	for _, c := range credList {
		var d map[string]any
		_ = json.Unmarshal(c.Data, &d)
		credentialsMap[c.Name] = d
	}

	// Build execution environment
	env := &workflow.ExecutionEnvironment{
		HTTPClient:        http.DefaultClient,
		AllowPrivateHosts: s.deps.Cfg.ToolAllowPrivateHosts,
		UserID:            userID,
		ModelResolver: func(modelName string) (model.StreamingModel, error) {
			reg := chat.NewModelRegistry(s.deps.ModelKeys)
			return reg.Resolve(chat.RunSpec{Model: modelName})
		},
	}

	engine := workflow.NewEngine(nil)

	// Create initial run record
	inputBytes, _ := json.Marshal(req.Input)
	runRecord, err := s.deps.Workflows.CreateRun(r.Context(), storage.WorkflowRun{
		WorkflowID:    wf.ID,
		UserID:        userID,
		Status:        "running",
		TriggerSource: "manual",
		InputData:     inputBytes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to record run start")
		return
	}

	runOpts := workflow.RunOptions{
		WorkflowID:    wf.ID,
		RunID:         runRecord.ID,
		UserID:        userID,
		TriggerSource: "manual",
		InputData:     req.Input,
		Credentials:   credentialsMap,
		Env:           env,
	}

	// Check if SSE streaming is requested
	if req.Stream || strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming_unsupported", "streaming not supported")
			return
		}

		eventsCh := make(chan workflow.StepEvent, 100)
		runOpts.Events = eventsCh

		go func() {
			defer close(eventsCh)
			res, _ := engine.Execute(r.Context(), nodes, edges, runOpts)
			// Persist final run record
			outBytes, _ := json.Marshal(res.Output)
			nodeResBytes, _ := json.Marshal(res.NodeResults)
			var errStr *string
			if res.Error != "" {
				errStr = &res.Error
			}
			_ = s.deps.Workflows.UpdateRun(r.Context(), storage.WorkflowRun{
				ID:          runRecord.ID,
				UserID:      userID,
				Status:      res.Status,
				OutputData:  outBytes,
				NodeResults: nodeResBytes,
				Error:       errStr,
				DurationMs:  res.DurationMs,
			})
		}()

		for ev := range eventsCh {
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: step\ndata: %s\n\n", b)
			flusher.Flush()
		}
		return
	}

	// Synchronous execution
	res, err := engine.Execute(r.Context(), nodes, edges, runOpts)
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
	}

	outBytes, _ := json.Marshal(res.Output)
	nodeResBytes, _ := json.Marshal(res.NodeResults)
	var errStr *string
	if res.Error != "" {
		errStr = &res.Error
	}
	_ = s.deps.Workflows.UpdateRun(r.Context(), storage.WorkflowRun{
		ID:          runRecord.ID,
		UserID:      userID,
		Status:      res.Status,
		OutputData:  outBytes,
		NodeResults: nodeResBytes,
		Error:       errStr,
		DurationMs:  res.DurationMs,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"run": toWorkflowRunDTO(storage.WorkflowRun{
			ID:            runRecord.ID,
			WorkflowID:    wf.ID,
			UserID:        userID,
			Status:        res.Status,
			TriggerSource: "manual",
			InputData:     inputBytes,
			OutputData:    outBytes,
			NodeResults:   nodeResBytes,
			Error:         errStr,
			DurationMs:    res.DurationMs,
			CreatedAt:     runRecord.CreatedAt,
		}),
	})
}

func (s *Server) handleListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeJSON(w, http.StatusOK, map[string]any{"runs": []workflowRunDTO{}})
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	runs, err := s.deps.Workflows.ListRuns(r.Context(), id, userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list workflow runs")
		return
	}

	dtos := make([]workflowRunDTO, len(runs))
	for i, run := range runs {
		dtos[i] = toWorkflowRunDTO(run)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": dtos})
}

func (s *Server) handleGetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	runID := r.PathValue("runId")
	run, err := s.deps.Workflows.GetRun(r.Context(), runID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not get run")
		return
	}
	writeJSON(w, http.StatusOK, toWorkflowRunDTO(run))
}

// Workflow credentials handlers

func (s *Server) handleListWorkflowCredentials(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeJSON(w, http.StatusOK, map[string]any{"credentials": []credentialDTO{}})
		return
	}
	userID, _ := userIDFrom(r.Context())
	list, err := s.deps.Workflows.ListCredentials(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list credentials")
		return
	}
	dtos := make([]credentialDTO, len(list))
	for i, c := range list {
		dtos[i] = toCredentialDTO(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": dtos})
}

func (s *Server) handleCreateWorkflowCredential(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "workflows not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Name string            `json:"name"`
		Type string            `json:"type"`
		Data map[string]string `json:"data"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "malformed request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "credential name is required")
		return
	}

	dataBytes, _ := json.Marshal(req.Data)
	created, err := s.deps.Workflows.CreateCredential(r.Context(), storage.WorkflowCredential{
		UserID: userID,
		Name:   req.Name,
		Type:   req.Type,
		Data:   dataBytes,
	})
	if err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate_credential_name", "credential with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not create credential")
		return
	}
	writeJSON(w, http.StatusCreated, toCredentialDTO(created))
}

func (s *Server) handleDeleteWorkflowCredential(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "credential not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	if err := s.deps.Workflows.DeleteCredential(r.Context(), id, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "credential not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete credential")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Public Webhook trigger endpoint: POST /api/webhooks/{slug} or GET /api/webhooks/{slug}
func (s *Server) handlePublicWebhook(w http.ResponseWriter, r *http.Request) {
	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
		writeError(w, http.StatusNotFound, "not_found", "webhook slug is required")
		return
	}

	wf, err := s.deps.Workflows.GetByWebhookSlug(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "webhook not found or inactive")
		return
	}

	// Verify secret if configured
	if wf.WebhookSecret != "" {
		providedSecret := r.Header.Get("X-Webhook-Secret")
		if providedSecret == "" {
			providedSecret = r.URL.Query().Get("secret")
		}
		if providedSecret == "" && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			providedSecret = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if providedSecret != wf.WebhookSecret {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid webhook secret")
			return
		}
	}

	// Capture headers
	reqHeaders := map[string]string{}
	for k := range r.Header {
		reqHeaders[k] = r.Header.Get(k)
	}

	// Capture query params
	queryParams := map[string]any{}
	for k, v := range r.URL.Query() {
		if len(v) == 1 {
			queryParams[k] = v[0]
		} else {
			queryParams[k] = v
		}
	}

	// Capture body
	var bodyParsed any
	rawBody, _ := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &bodyParsed); err != nil {
			bodyParsed = string(rawBody)
		}
	} else {
		bodyParsed = map[string]any{}
	}

	inputPayload := map[string]any{
		"headers": reqHeaders,
		"query":   queryParams,
		"body":    bodyParsed,
		"method":  r.Method,
	}

	nodes, edges, err := workflow.ParseWorkflowGraph(wf.Nodes, wf.Edges)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid_graph", "could not parse workflow graph")
		return
	}

	// Fetch credentials
	credList, _ := s.deps.Workflows.ListCredentials(r.Context(), wf.UserID)
	credentialsMap := make(map[string]map[string]any, len(credList))
	for _, c := range credList {
		var d map[string]any
		_ = json.Unmarshal(c.Data, &d)
		credentialsMap[c.Name] = d
	}

	env := &workflow.ExecutionEnvironment{
		HTTPClient:        http.DefaultClient,
		AllowPrivateHosts: s.deps.Cfg.ToolAllowPrivateHosts,
		UserID:            wf.UserID,
		ModelResolver: func(modelName string) (model.StreamingModel, error) {
			reg := chat.NewModelRegistry(s.deps.ModelKeys)
			return reg.Resolve(chat.RunSpec{Model: modelName})
		},
	}

	engine := workflow.NewEngine(nil)

	inputBytes, _ := json.Marshal(inputPayload)
	runRecord, _ := s.deps.Workflows.CreateRun(r.Context(), storage.WorkflowRun{
		WorkflowID:    wf.ID,
		UserID:        wf.UserID,
		Status:        "running",
		TriggerSource: "webhook",
		InputData:     inputBytes,
	})

	runID := runRecord.ID
	if runID == "" {
		runID = uuid.NewString()
	}

	res, err := engine.Execute(r.Context(), nodes, edges, workflow.RunOptions{
		WorkflowID:    wf.ID,
		RunID:         runID,
		UserID:        wf.UserID,
		TriggerSource: "webhook",
		InputData:     inputPayload,
		Credentials:   credentialsMap,
		Env:           env,
	})
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
	}

	// Persist run
	outBytes, _ := json.Marshal(res.Output)
	nodeResBytes, _ := json.Marshal(res.NodeResults)
	var errStr *string
	if res.Error != "" {
		errStr = &res.Error
	}
	_ = s.deps.Workflows.UpdateRun(r.Context(), storage.WorkflowRun{
		ID:          runID,
		UserID:      wf.UserID,
		Status:      res.Status,
		OutputData:  outBytes,
		NodeResults: nodeResBytes,
		Error:       errStr,
		DurationMs:  res.DurationMs,
	})

	// If a custom webhook response was produced
	if res.Response != nil {
		for k, v := range res.Response.Headers {
			w.Header().Set(k, v)
		}
		status := res.Response.StatusCode
		if status == 0 {
			status = http.StatusOK
		}
		writeJSON(w, status, res.Response.Body)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         res.Status == "success",
		"runId":      runID,
		"output":     res.Output,
		"durationMs": res.DurationMs,
	})
}
