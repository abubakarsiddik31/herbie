package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/tool"
)

func TestGoogleCalendarTool_NotConnected(t *testing.T) {
	client := CalendarClient{
		TokenFunc: func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("not connected")
		},
	}
	tl := GoogleCalendarTool(client)

	args := json.RawMessage(`{"action": "list_events"}`)
	res, err := tl.Exec(context.Background(), Deps{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Text, "not connected") || !strings.Contains(res.Text, "connect:google_calendar") {
		t.Fatalf("expected connect prompt, got: %s", res.Text)
	}
}

func TestGoogleCalendarTool_ListEvents(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-valid-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/calendar/v3/calendars/primary/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items": [
				{
					"id": "evt_1",
					"summary": "Sprint Planning",
					"description": "Weekly team sync",
					"start": {"dateTime": "2026-09-19T10:00:00Z"},
					"end": {"dateTime": "2026-09-19T11:00:00Z"},
					"attendees": [{"email": "alice@example.com", "displayName": "Alice"}]
				}
			]
		}`))
	}))
	defer ts.Close()

	client := CalendarClient{
		BaseURL: ts.URL,
		TokenFunc: func(ctx context.Context) (string, error) {
			return "test-valid-token", nil
		},
	}
	tl := GoogleCalendarTool(client)

	args := json.RawMessage(`{"action": "list_events"}`)
	res, err := tl.Exec(context.Background(), Deps{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Text, "Sprint Planning") || !strings.Contains(res.Text, "Alice") {
		t.Fatalf("expected formatted event list, got: %s", res.Text)
	}
}

func TestGoogleCalendarTool_CreateEvent_RequiresApproval(t *testing.T) {
	client := CalendarClient{
		TokenFunc: func(ctx context.Context) (string, error) {
			return "test-valid-token", nil
		},
	}
	tl := GoogleCalendarTool(client)

	args := json.RawMessage(`{
		"action": "create_event",
		"summary": "Demo Review",
		"start_time": "2026-09-19T14:00:00Z",
		"end_time": "2026-09-19T15:00:00Z"
	}`)

	// Without approval: should defer
	_, err := tl.Exec(context.Background(), Deps{}, args)
	if err == nil {
		t.Fatal("expected approval deferral error, got nil")
	}
	var def *tool.Deferred
	if !errors.As(err, &def) || def.Kind != tool.DeferApproval {
		t.Fatalf("expected DeferApproval, got %v", err)
	}
	if !strings.Contains(def.Reason, "Demo Review") {
		t.Fatalf("expected reason to mention event title, got: %s", def.Reason)
	}
}

func TestGoogleCalendarTool_CreateEvent_Approved(t *testing.T) {
	var receivedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "created_123",
			"summary": "Demo Review",
			"htmlLink": "https://calendar.google.com/event?id=created_123"
		}`))
	}))
	defer ts.Close()

	client := CalendarClient{
		BaseURL: ts.URL,
		TokenFunc: func(ctx context.Context) (string, error) {
			return "test-valid-token", nil
		},
	}
	tl := GoogleCalendarTool(client)

	args := json.RawMessage(`{
		"action": "create_event",
		"summary": "Demo Review",
		"start_time": "2026-09-19T14:00:00Z",
		"end_time": "2026-09-19T15:00:00Z",
		"attendees": ["bob@example.com"]
	}`)

	// With approval
	ctx := tool.WithApprovedCall(context.Background())
	res, err := tl.Exec(ctx, Deps{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Text, "created_123") || !strings.Contains(res.Text, "Demo Review") {
		t.Fatalf("expected success message, got: %s", res.Text)
	}
	if receivedBody["summary"] != "Demo Review" {
		t.Fatalf("unexpected body sent to google: %+v", receivedBody)
	}
}

func TestGoogleCalendarTool_DeleteEvent_RequiresApprovalAndApproved(t *testing.T) {
	deleted := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/evt_to_delete") {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := CalendarClient{
		BaseURL: ts.URL,
		TokenFunc: func(ctx context.Context) (string, error) {
			return "test-valid-token", nil
		},
	}
	tl := GoogleCalendarTool(client)

	args := json.RawMessage(`{"action": "delete_event", "event_id": "evt_to_delete"}`)

	// Unapproved -> defers
	_, err := tl.Exec(context.Background(), Deps{}, args)
	if err == nil {
		t.Fatal("expected approval deferral, got nil")
	}

	// Approved -> executes delete
	ctx := tool.WithApprovedCall(context.Background())
	res, err := tl.Exec(ctx, Deps{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Fatal("expected DELETE request to be issued to server")
	}
	if !strings.Contains(res.Text, "successfully deleted") {
		t.Fatalf("expected deletion confirmation, got: %s", res.Text)
	}
}
