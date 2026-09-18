package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestWorkflowsCRUDPersistence(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	userID := testUser(t, ctx, "workflow-crud@test.dev")
	if _, err := pool.Exec(ctx, `DELETE FROM workflows WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	workflows := NewWorkflows(pool)

	// Create workflow
	wf, err := workflows.Create(ctx, Workflow{
		UserID:          userID,
		Name:            "GitHub Sync Flow",
		Description:     "Sync issues to Slack",
		TriggerType:     "webhook",
		WebhookSecret:   "sec_123",
		Nodes:           json.RawMessage(`[{"id":"1","type":"trigger_webhook"},{"id":"2","type":"http_request"}]`),
		Edges:           json.RawMessage(`[{"id":"e1-2","source":"1","target":"2"}]`),
		ExposeAsTool:    true,
		ToolName:        "github_slack_sync",
		ToolDescription: "Syncs github issues to slack",
		IsActive:        true,
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if wf.ID == "" || wf.Name != "GitHub Sync Flow" {
		t.Fatalf("unexpected workflow: %+v", wf)
	}

	// Get workflow
	got, err := workflows.Get(ctx, wf.ID, userID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if got.ID != wf.ID || got.TriggerType != "webhook" || !got.ExposeAsTool {
		t.Fatalf("mismatch workflow: %+v", got)
	}

	// List workflows
	list, err := workflows.List(ctx, userID)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(list))
	}

	// List active tools
	tools, err := workflows.ListActiveTools(ctx, userID)
	if err != nil {
		t.Fatalf("list active tools: %v", err)
	}
	if len(tools) != 1 || tools[0].ToolName != "github_slack_sync" {
		t.Fatalf("expected 1 active tool, got %+v", tools)
	}

	// Update workflow
	newName := "GitHub Sync Flow Updated"
	newActive := false
	updated, err := workflows.Update(ctx, wf.ID, userID, WorkflowPatch{
		Name:     &newName,
		IsActive: &newActive,
	})
	if err != nil {
		t.Fatalf("update workflow: %v", err)
	}
	if updated.Name != newName || updated.IsActive != false {
		t.Fatalf("unexpected updated workflow: %+v", updated)
	}

	// Create run
	run, err := workflows.CreateRun(ctx, WorkflowRun{
		WorkflowID:    wf.ID,
		UserID:        userID,
		Status:        "running",
		TriggerSource: "webhook",
		InputData:     json.RawMessage(`{"event":"push"}`),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.ID == "" || run.Status != "running" {
		t.Fatalf("unexpected run: %+v", run)
	}

	// Update run
	run.Status = "success"
	run.OutputData = json.RawMessage(`{"sent":true}`)
	run.NodeResults = json.RawMessage(`{"1":{"status":"success"}}`)
	run.DurationMs = 150
	now := time.Now()
	run.FinishedAt = &now
	if err := workflows.UpdateRun(ctx, run); err != nil {
		t.Fatalf("update run: %v", err)
	}

	gotRun, err := workflows.GetRun(ctx, run.ID, userID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.Status != "success" || gotRun.DurationMs != 150 {
		t.Fatalf("unexpected got run: %+v", gotRun)
	}

	runs, err := workflows.ListRuns(ctx, wf.ID, userID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	// Credentials CRUD
	cred, err := workflows.CreateCredential(ctx, WorkflowCredential{
		UserID: userID,
		Name:   "slack_webhook_url",
		Type:   "bearer_token",
		Data:   json.RawMessage(`{"token":"xoxb-1234"}`),
	})
	if err != nil {
		t.Fatalf("create cred: %v", err)
	}
	if cred.ID == "" || cred.Name != "slack_webhook_url" {
		t.Fatalf("unexpected cred: %+v", cred)
	}

	creds, err := workflows.ListCredentials(ctx, userID)
	if err != nil {
		t.Fatalf("list creds: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 cred, got %d", len(creds))
	}

	// Delete workflow (cascades to runs)
	if err := workflows.Delete(ctx, wf.ID, userID); err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	if _, err := workflows.Get(ctx, wf.ID, userID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Delete credential
	if err := workflows.DeleteCredential(ctx, cred.ID, userID); err != nil {
		t.Fatalf("delete cred: %v", err)
	}
	if _, err := workflows.GetCredential(ctx, cred.ID, userID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
