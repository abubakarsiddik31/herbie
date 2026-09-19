package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// GoogleCalendarToolName is the identifier for the Google Calendar integration.
const GoogleCalendarToolName = "google_calendar"

var googleCalendarSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": ["list_events", "create_event", "delete_event"],
      "description": "Calendar action to execute: 'list_events' to view schedule, 'create_event' to add a meeting (requires approval), 'delete_event' to cancel an event (requires approval)."
    },
    "time_min": {
      "type": "string",
      "description": "Start of search time window in RFC3339 format (e.g. '2026-09-19T00:00:00Z'). Defaults to start of current day."
    },
    "time_max": {
      "type": "string",
      "description": "End of search time window in RFC3339 format (e.g. '2026-09-20T23:59:59Z')."
    },
    "max_results": {
      "type": "integer",
      "description": "Maximum number of events to return (1-50, default 10)."
    },
    "summary": {
      "type": "string",
      "description": "Title or summary of the calendar event (required for create_event)."
    },
    "description": {
      "type": "string",
      "description": "Detailed description or agenda for create_event."
    },
    "start_time": {
      "type": "string",
      "description": "Event start time in RFC3339 format (required for create_event, e.g. '2026-09-19T14:00:00Z')."
    },
    "end_time": {
      "type": "string",
      "description": "Event end time in RFC3339 format (required for create_event, e.g. '2026-09-19T15:00:00Z')."
    },
    "attendees": {
      "type": "array",
      "items": {"type": "string"},
      "description": "List of attendee email addresses for create_event."
    },
    "event_id": {
      "type": "string",
      "description": "Google Calendar event ID to delete (required for delete_event)."
    }
  },
  "required": ["action"]
}`)

// CalendarClient manages Google Calendar API communication.
type CalendarClient struct {
	BaseURL     string
	TokenFunc   func(ctx context.Context) (string, error)
	HTTPClient  *http.Client
	AuditRecord func(ctx context.Context, action, summary, status string)
}

type calendarArgs struct {
	Action      string   `json:"action"`
	TimeMin     string   `json:"time_min"`
	TimeMax     string   `json:"time_max"`
	MaxResults  int      `json:"max_results"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	Attendees   []string `json:"attendees"`
	EventID     string   `json:"event_id"`
}

// GoogleCalendarTool constructs the Google Calendar tool with schema and execution handler.
func GoogleCalendarTool(client CalendarClient) tool.Tool[Deps] {
	if client.BaseURL == "" {
		client.BaseURL = "https://www.googleapis.com"
	}
	if client.HTTPClient == nil {
		client.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}

	return tool.Tool[Deps]{
		Name:        GoogleCalendarToolName,
		Description: "Access Google Calendar to check agenda, list upcoming events, create new events (requires approval), or cancel events (requires approval).",
		Schema:      googleCalendarSchema,
		Timeout:     30 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, rawArgs json.RawMessage) (tool.Result, error) {
			var a calendarArgs
			if err := json.Unmarshal(rawArgs, &a); err != nil {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf("invalid calendar arguments: %w", err)}
			}

			if client.TokenFunc == nil {
				return tool.Text("Google Calendar is not connected. [Connect Google Calendar](connect:google_calendar) to access your schedule."), nil
			}

			token, err := client.TokenFunc(ctx)
			if err != nil || token == "" {
				return tool.Text("Google Calendar is not connected. [Connect Google Calendar](connect:google_calendar) to allow Herbie to view and manage your schedule."), nil
			}

			switch a.Action {
			case "list_events":
				return executeListEvents(ctx, client, token, a)
			case "create_event":
				return executeCreateEvent(ctx, client, token, a)
			case "delete_event":
				return executeDeleteEvent(ctx, client, token, a)
			default:
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf("unknown calendar action %q; must be 'list_events', 'create_event', or 'delete_event'", a.Action)}
			}
		},
	}
}

func executeListEvents(ctx context.Context, client CalendarClient, token string, a calendarArgs) (tool.Result, error) {
	timeMin := a.TimeMin
	if timeMin == "" {
		now := time.Now().UTC()
		timeMin = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	}

	maxResults := a.MaxResults
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 10
	}

	q := url.Values{
		"timeMin":      {timeMin},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
		"maxResults":   {fmt.Sprintf("%d", maxResults)},
	}
	if a.TimeMax != "" {
		q.Set("timeMax", a.TimeMax)
	}

	endpoint := fmt.Sprintf("%s/calendar/v3/calendars/primary/events?%s", strings.TrimRight(client.BaseURL, "/"), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tool.Result{}, fmt.Errorf("calendar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		if client.AuditRecord != nil {
			client.AuditRecord(ctx, "list_events", "fetch upcoming events", "error")
		}
		return tool.Result{}, fmt.Errorf("calendar api: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return tool.Text("Google Calendar connection has expired. [Reconnect Google Calendar](connect:google_calendar) to continue."), nil
	}
	if resp.StatusCode != http.StatusOK {
		if client.AuditRecord != nil {
			client.AuditRecord(ctx, "list_events", "fetch upcoming events", "error")
		}
		return tool.Text(fmt.Sprintf("Google Calendar returned error %d: %s", resp.StatusCode, string(body))), nil
	}

	var data struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Location    string `json:"location"`
			HtmlLink    string `json:"htmlLink"`
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
			Attendees []struct {
				Email       string `json:"email"`
				DisplayName string `json:"displayName"`
			} `json:"attendees"`
		} `json:"items"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return tool.Result{}, fmt.Errorf("unmarshal calendar events: %w", err)
	}

	if client.AuditRecord != nil {
		client.AuditRecord(ctx, "list_events", fmt.Sprintf("fetched %d events", len(data.Items)), "success")
	}

	if len(data.Items) == 0 {
		return tool.Text("No upcoming events found on Google Calendar for the requested time period."), nil
	}

	var sb strings.Builder
	sb.WriteString("Upcoming Google Calendar events:\n\n")
	for i, item := range data.Items {
		summary := item.Summary
		if summary == "" {
			summary = "(No Title)"
		}
		startStr := item.Start.DateTime
		if startStr == "" {
			startStr = item.Start.Date
		}
		endStr := item.End.DateTime
		if endStr == "" {
			endStr = item.End.Date
		}

		fmt.Fprintf(&sb, "%d. **%s**\n", i+1, summary)
		fmt.Fprintf(&sb, "   - Time: %s to %s\n", startStr, endStr)
		if item.Location != "" {
			fmt.Fprintf(&sb, "   - Location: %s\n", item.Location)
		}
		if item.Description != "" {
			desc := strings.TrimSpace(item.Description)
			if len(desc) > 100 {
				desc = desc[:97] + "..."
			}
			fmt.Fprintf(&sb, "   - Description: %s\n", desc)
		}
		if len(item.Attendees) > 0 {
			var atts []string
			for _, att := range item.Attendees {
				if att.DisplayName != "" {
					atts = append(atts, fmt.Sprintf("%s (%s)", att.DisplayName, att.Email))
				} else {
					atts = append(atts, att.Email)
				}
			}
			fmt.Fprintf(&sb, "   - Attendees: %s\n", strings.Join(atts, ", "))
		}
		fmt.Fprintf(&sb, "   - Event ID: `%s`\n\n", item.ID)
	}

	return tool.Text(strings.TrimSpace(sb.String())), nil
}

func executeCreateEvent(ctx context.Context, client CalendarClient, token string, a calendarArgs) (tool.Result, error) {
	if strings.TrimSpace(a.Summary) == "" {
		return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf("create_event requires 'summary'")}
	}
	if strings.TrimSpace(a.StartTime) == "" || strings.TrimSpace(a.EndTime) == "" {
		return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf("create_event requires 'start_time' and 'end_time' in RFC3339 format")}
	}

	// Safety Gate: Enforce Human-in-the-Loop approval
	if !tool.CallApproved(ctx) {
		return tool.Result{}, &tool.Deferred{
			Kind:   tool.DeferApproval,
			Reason: fmt.Sprintf("Create event %q from %s to %s", a.Summary, a.StartTime, a.EndTime),
		}
	}

	payload := map[string]any{
		"summary":     a.Summary,
		"description": a.Description,
		"start": map[string]string{
			"dateTime": a.StartTime,
		},
		"end": map[string]string{
			"dateTime": a.EndTime,
		},
	}
	if len(a.Attendees) > 0 {
		var atts []map[string]string
		for _, email := range a.Attendees {
			if e := strings.TrimSpace(email); e != "" {
				atts = append(atts, map[string]string{"email": e})
			}
		}
		payload["attendees"] = atts
	}

	bodyBytes, _ := json.Marshal(payload)
	endpoint := fmt.Sprintf("%s/calendar/v3/calendars/primary/events", strings.TrimRight(client.BaseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return tool.Result{}, fmt.Errorf("create calendar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		if client.AuditRecord != nil {
			client.AuditRecord(ctx, "create_event", fmt.Sprintf("create event %q", a.Summary), "error")
		}
		return tool.Result{}, fmt.Errorf("calendar create api: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return tool.Text("Google Calendar connection has expired. [Reconnect Google Calendar](connect:google_calendar) to continue."), nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		if client.AuditRecord != nil {
			client.AuditRecord(ctx, "create_event", fmt.Sprintf("create event %q", a.Summary), "error")
		}
		return tool.Text(fmt.Sprintf("Failed to create event (status %d): %s", resp.StatusCode, string(body))), nil
	}

	var created struct {
		ID       string `json:"id"`
		Summary  string `json:"summary"`
		HtmlLink string `json:"htmlLink"`
	}
	_ = json.Unmarshal(body, &created)

	if client.AuditRecord != nil {
		client.AuditRecord(ctx, "create_event", fmt.Sprintf("created event %q (id: %s)", a.Summary, created.ID), "success")
	}

	return tool.Text(fmt.Sprintf("Event created successfully!\n- **Title**: %s\n- **Start**: %s\n- **End**: %s\n- **ID**: `%s`\n- **Link**: %s",
		a.Summary, a.StartTime, a.EndTime, created.ID, created.HtmlLink)), nil
}

func executeDeleteEvent(ctx context.Context, client CalendarClient, token string, a calendarArgs) (tool.Result, error) {
	if strings.TrimSpace(a.EventID) == "" {
		return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf("delete_event requires 'event_id'")}
	}

	// Safety Gate: Enforce Human-in-the-Loop approval
	if !tool.CallApproved(ctx) {
		return tool.Result{}, &tool.Deferred{
			Kind:   tool.DeferApproval,
			Reason: fmt.Sprintf("Delete calendar event ID %q", a.EventID),
		}
	}

	endpoint := fmt.Sprintf("%s/calendar/v3/calendars/primary/events/%s", strings.TrimRight(client.BaseURL, "/"), url.PathEscape(a.EventID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return tool.Result{}, fmt.Errorf("delete calendar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		if client.AuditRecord != nil {
			client.AuditRecord(ctx, "delete_event", fmt.Sprintf("delete event %q", a.EventID), "error")
		}
		return tool.Result{}, fmt.Errorf("calendar delete api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return tool.Text("Google Calendar connection has expired. [Reconnect Google Calendar](connect:google_calendar) to continue."), nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		if client.AuditRecord != nil {
			client.AuditRecord(ctx, "delete_event", fmt.Sprintf("delete event %q", a.EventID), "error")
		}
		return tool.Text(fmt.Sprintf("Failed to delete event (status %d): %s", resp.StatusCode, string(body))), nil
	}

	if client.AuditRecord != nil {
		client.AuditRecord(ctx, "delete_event", fmt.Sprintf("deleted event %q", a.EventID), "success")
	}

	return tool.Text(fmt.Sprintf("Calendar event `%s` has been successfully deleted.", a.EventID)), nil
}
