package storage

import (
	"context"
	"testing"
	"time"
)

func TestAuditsCRUD(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	userID := testUser(t, ctx, "audit-test@test.dev")

	audits := NewAudits(pool)

	err := audits.RecordToolAudit(ctx, ToolAuditLog{
		UserID:        userID,
		CallerType:    "chat_agent",
		CallerID:      "conv-123",
		ToolName:      "web_search",
		Action:        "execute",
		InputSummary:  `{"query":"latest golang release"}`,
		OutputSummary: `Found Go 1.23 release notes`,
		Status:        "success",
		DurationMs:    150,
		CreatedAt:     time.Now(),
	})
	if err != nil {
		t.Fatalf("record tool audit: %v", err)
	}

	err = audits.RecordToolAudit(ctx, ToolAuditLog{
		UserID:        userID,
		CallerType:    "workflow",
		CallerID:      "wf-456",
		ToolName:      "github_sync",
		Action:        "execute",
		InputSummary:  `{"repo":"my-org/my-repo"}`,
		OutputSummary: ``,
		Status:        "failed",
		DurationMs:    300,
		CreatedAt:     time.Now().Add(1 * time.Second),
	})
	if err != nil {
		t.Fatalf("record second tool audit: %v", err)
	}

	list, err := audits.ListToolAudits(ctx, userID, 10)
	if err != nil {
		t.Fatalf("list tool audits: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 audit logs, got %d", len(list))
	}

	if list[0].ToolName != "github_sync" || list[0].Status != "failed" {
		t.Errorf("expected newest audit first, got: %+v", list[0])
	}
	if list[1].ToolName != "web_search" || list[1].Status != "success" {
		t.Errorf("expected second audit next, got: %+v", list[1])
	}
}
