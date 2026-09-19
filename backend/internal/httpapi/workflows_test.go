package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/tool"
	"github.com/google/uuid"
)

type fakeWorkflowStore struct {
	workflows   map[string]storage.Workflow
	runs        map[string]storage.WorkflowRun
	credentials map[string]storage.WorkflowCredential
}

func newFakeWorkflowStore() *fakeWorkflowStore {
	return &fakeWorkflowStore{
		workflows:   make(map[string]storage.Workflow),
		runs:        make(map[string]storage.WorkflowRun),
		credentials: make(map[string]storage.WorkflowCredential),
	}
}

func (f *fakeWorkflowStore) Create(_ context.Context, w storage.Workflow) (storage.Workflow, error) {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	w.CreatedAt = time.Now()
	w.UpdatedAt = time.Now()
	f.workflows[w.ID] = w
	return w, nil
}

func (f *fakeWorkflowStore) Get(_ context.Context, id, userID string) (storage.Workflow, error) {
	w, ok := f.workflows[id]
	if !ok || w.UserID != userID {
		return storage.Workflow{}, storage.ErrNotFound
	}
	return w, nil
}

func (f *fakeWorkflowStore) GetByWebhookSlug(_ context.Context, slug string) (storage.Workflow, error) {
	for _, w := range f.workflows {
		if w.WebhookSlug != nil && *w.WebhookSlug == slug && w.IsActive {
			return w, nil
		}
	}
	return storage.Workflow{}, storage.ErrNotFound
}

func (f *fakeWorkflowStore) List(_ context.Context, userID string) ([]storage.Workflow, error) {
	var out []storage.Workflow
	for _, w := range f.workflows {
		if w.UserID == userID {
			out = append(out, w)
		}
	}
	return out, nil
}

func (f *fakeWorkflowStore) ListActiveTools(_ context.Context, userID string) ([]storage.Workflow, error) {
	var out []storage.Workflow
	for _, w := range f.workflows {
		if w.UserID == userID && w.IsActive && w.ExposeAsTool {
			out = append(out, w)
		}
	}
	return out, nil
}

func (f *fakeWorkflowStore) Update(_ context.Context, id, userID string, patch storage.WorkflowPatch) (storage.Workflow, error) {
	w, ok := f.workflows[id]
	if !ok || w.UserID != userID {
		return storage.Workflow{}, storage.ErrNotFound
	}
	if patch.Name != nil {
		w.Name = *patch.Name
	}
	if patch.Description != nil {
		w.Description = *patch.Description
	}
	if patch.IsActive != nil {
		w.IsActive = *patch.IsActive
	}
	if patch.Nodes != nil {
		w.Nodes = *patch.Nodes
	}
	if patch.Edges != nil {
		w.Edges = *patch.Edges
	}
	w.UpdatedAt = time.Now()
	f.workflows[id] = w
	return w, nil
}

func (f *fakeWorkflowStore) Delete(_ context.Context, id, userID string) error {
	w, ok := f.workflows[id]
	if !ok || w.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.workflows, id)
	return nil
}

func (f *fakeWorkflowStore) CreateRun(_ context.Context, r storage.WorkflowRun) (storage.WorkflowRun, error) {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	r.CreatedAt = time.Now()
	f.runs[r.ID] = r
	return r, nil
}

func (f *fakeWorkflowStore) GetRun(_ context.Context, id, userID string) (storage.WorkflowRun, error) {
	r, ok := f.runs[id]
	if !ok || r.UserID != userID {
		return storage.WorkflowRun{}, storage.ErrNotFound
	}
	return r, nil
}

func (f *fakeWorkflowStore) ListRuns(_ context.Context, workflowID, userID string, _ int) ([]storage.WorkflowRun, error) {
	var out []storage.WorkflowRun
	for _, r := range f.runs {
		if r.WorkflowID == workflowID && r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeWorkflowStore) UpdateRun(_ context.Context, r storage.WorkflowRun) error {
	existing, ok := f.runs[r.ID]
	if !ok || existing.UserID != r.UserID {
		return storage.ErrNotFound
	}
	existing.Status = r.Status
	existing.OutputData = r.OutputData
	existing.NodeResults = r.NodeResults
	existing.Error = r.Error
	existing.DurationMs = r.DurationMs
	existing.FinishedAt = r.FinishedAt
	f.runs[r.ID] = existing
	return nil
}

func (f *fakeWorkflowStore) CreateCredential(_ context.Context, c storage.WorkflowCredential) (storage.WorkflowCredential, error) {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	f.credentials[c.ID] = c
	return c, nil
}

func (f *fakeWorkflowStore) GetCredential(_ context.Context, id, userID string) (storage.WorkflowCredential, error) {
	c, ok := f.credentials[id]
	if !ok || c.UserID != userID {
		return storage.WorkflowCredential{}, storage.ErrNotFound
	}
	return c, nil
}

func (f *fakeWorkflowStore) GetCredentialByProvider(_ context.Context, userID, provider string) (storage.WorkflowCredential, error) {
	for _, c := range f.credentials {
		if c.UserID == userID && c.Provider == provider {
			return c, nil
		}
	}
	return storage.WorkflowCredential{}, storage.ErrNotFound
}

func (f *fakeWorkflowStore) ListCredentials(_ context.Context, userID string) ([]storage.WorkflowCredential, error) {
	var out []storage.WorkflowCredential
	for _, c := range f.credentials {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeWorkflowStore) UpdateCredential(_ context.Context, c storage.WorkflowCredential) (storage.WorkflowCredential, error) {
	existing, ok := f.credentials[c.ID]
	if !ok || existing.UserID != c.UserID {
		return storage.WorkflowCredential{}, storage.ErrNotFound
	}
	existing.Name = c.Name
	existing.Type = c.Type
	existing.Provider = c.Provider
	existing.Scopes = c.Scopes
	existing.ExpiresAt = c.ExpiresAt
	existing.Data = c.Data
	existing.UpdatedAt = time.Now()
	f.credentials[c.ID] = existing
	return existing, nil
}

func (f *fakeWorkflowStore) DeleteCredential(_ context.Context, id, userID string) error {
	c, ok := f.credentials[id]
	if !ok || c.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.credentials, id)
	return nil
}

func newWorkflowTestServer(t *testing.T, wfStore *fakeWorkflowStore) (http.Handler, string) {
	t.Helper()
	secret := "0123456789abcdef0123456789abcdef"
	svc := authtest.NewService(secret)
	tm, err := auth.NewTokenMaker(secret)
	if err != nil {
		t.Fatal(err)
	}

	tok, _, err := tm.Issue("test-user-id", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewServer(ServerDeps{
		Cfg:       config.Config{FrontendOrigin: "http://localhost:5173"},
		Log:       log,
		Auth:      svc,
		Tokens:    tm,
		Workflows: wfStore,
	})
	return handler, tok
}

func TestWorkflowHTTPFlow(t *testing.T) {
	store := newFakeWorkflowStore()
	handler, token := newWorkflowTestServer(t, store)

	// 1. Create Workflow
	createBody := map[string]any{
		"name":        "Test Flow",
		"description": "A sample automation",
		"triggerType": "manual",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "manual", "name": "Start"},
			map[string]any{"id": "n2", "type": "code_transform", "name": "SetData", "data": map[string]any{
				"fields": map[string]any{"msg": "Hello {{ $json.user }}"},
			}},
		},
		"edges": []any{
			map[string]any{"id": "e1", "source": "n1", "target": "n2"},
		},
		"isActive": true,
	}
	b, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create workflow failed: %d %s", rec.Code, rec.Body.String())
	}

	var created workflowDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" || created.Name != "Test Flow" {
		t.Fatalf("unexpected created workflow: %+v", created)
	}

	// 2. List Workflows
	req = httptest.NewRequest(http.MethodGet, "/api/workflows", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list workflows failed: %d", rec.Code)
	}
	var listResp struct {
		Workflows []workflowDTO `json:"workflows"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(listResp.Workflows))
	}

	// 3. Run Workflow
	runReqBody := map[string]any{
		"input": map[string]any{"user": "Bob"},
	}
	rb, _ := json.Marshal(runReqBody)
	req = httptest.NewRequest(http.MethodPost, "/api/workflows/"+created.ID+"/run", bytes.NewReader(rb))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("run workflow failed: %d %s", rec.Code, rec.Body.String())
	}

	var runResp struct {
		Run workflowRunDTO `json:"run"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &runResp)
	if runResp.Run.Status != "success" {
		t.Fatalf("expected run status success, got %s", runResp.Run.Status)
	}

	// 4. Create & List Credentials
	credReq := map[string]any{
		"name": "my_api_key",
		"type": "api_key",
		"data": map[string]string{"key": "super_secret_value"},
	}
	cb, _ := json.Marshal(credReq)
	req = httptest.NewRequest(http.MethodPost, "/api/workflow-credentials", bytes.NewReader(cb))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create cred failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify sensitive data is masked
	var credRes credentialDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &credRes)
	if credRes.Data["key"] != headerMask {
		t.Errorf("expected masked key, got %v", credRes.Data["key"])
	}

	// 5. Delete Workflow
	req = httptest.NewRequest(http.MethodDelete, "/api/workflows/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete workflow failed: %d", rec.Code)
	}
}

func TestPublicWebhookTrigger(t *testing.T) {
	store := newFakeWorkflowStore()
	slug := "test-webhook"
	secret := "hook-secret"
	wf := storage.Workflow{
		ID:            "wf-1",
		UserID:        "user-1",
		Name:          "Webhook Flow",
		TriggerType:   "webhook",
		WebhookSlug:   &slug,
		WebhookSecret: secret,
		IsActive:      true,
		Nodes: json.RawMessage(`[
			{"id":"w1","type":"webhook"},
			{"id":"r1","type":"webhook_response","data":{"statusCode":201,"body":{"received":"{{ $json.body.message }}"}}}
		]`),
		Edges: json.RawMessage(`[{"id":"e1","source":"w1","target":"r1"}]`),
	}
	_, _ = store.Create(context.Background(), wf)

	handler, _ := newWorkflowTestServer(t, store)

	// Call webhook without secret -> 401
	body := `{"message": "alert!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/"+slug, strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without secret, got %d", rec.Code)
	}

	// Call webhook with secret -> 201 custom response
	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/"+slug, strings.NewReader(body))
	req.Header.Set("X-Webhook-Secret", secret)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["received"] != "alert!" {
		t.Errorf("expected received alert!, got %v", res["received"])
	}
}

func TestWorkflowToolApprovalGating(t *testing.T) {
	store := newFakeWorkflowStore()
	userID := "user-approvals"
	wf := storage.Workflow{
		ID:                  "wf-tool-approval",
		UserID:              userID,
		Name:                "Deploy App",
		TriggerType:         "manual",
		IsActive:            true,
		ExposeAsTool:        true,
		ToolName:            "deploy_prod",
		ToolDescription:     "Deploys to production",
		ToolRequireApproval: true,
		Nodes: json.RawMessage(`[
			{"id":"n1","type":"manual"},
			{"id":"n2","type":"code_transform","data":{"fields":{"status":"deployed"}}}
		]`),
		Edges: json.RawMessage(`[{"id":"e1","source":"n1","target":"n2"}]`),
	}
	_, _ = store.Create(context.Background(), wf)

	server := &Server{
		deps: ServerDeps{
			Workflows: store,
			Audits:    &fakeAuditStore{},
		},
	}

	tools, err := server.workflowTools(context.Background(), userID)
	if err != nil {
		t.Fatalf("workflowTools error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	// 1. Calling without approval must return Deferred approval
	ctx := context.Background()
	_, execErr := tools[0].Exec(ctx, chat.Deps{}, json.RawMessage(`{"input":"v1.0"}`))
	if execErr == nil {
		t.Fatal("expected Deferred approval error, got nil")
	}
	def, ok := execErr.(*tool.Deferred)
	if !ok || def.Kind != tool.DeferApproval {
		t.Fatalf("expected DeferApproval, got %T: %v", execErr, execErr)
	}

	// 2. Calling with approval set must succeed
	approvedCtx := tool.WithApprovedCall(ctx)
	res, approvedErr := tools[0].Exec(approvedCtx, chat.Deps{}, json.RawMessage(`{"input":"v1.0"}`))
	if approvedErr != nil {
		t.Fatalf("expected success with approval, got: %v", approvedErr)
	}
	if !strings.Contains(res.Text, "deployed") {
		t.Fatalf("expected deployed in output after approved run, got: %s", res.Text)
	}
}
