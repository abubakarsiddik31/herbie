package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem/tool"
)

func TestBuildToolsValidatesAndNames(t *testing.T) {
	cfgs := []ToolConfig{weatherConfig("https://api.example.com/wx/{{city}}")}
	tools, err := BuildTools(cfgs, DefaultToolEnv())
	if err != nil {
		t.Fatalf("BuildTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "get_weather" {
		t.Fatalf("tools = %+v", tools)
	}
	var schema map[string]any
	if err := json.Unmarshal(tools[0].Schema, &schema); err != nil || schema["type"] != "object" {
		t.Fatalf("schema not attached: %v", err)
	}
	if tools[0].Timeout != DefaultToolEnv().HTTPTimeout+10*time.Second {
		t.Fatalf("timeout = %s", tools[0].Timeout)
	}
}

func TestBuildToolsRejectsInvalidConfig(t *testing.T) {
	cfg := weatherConfig("https://api.example.com")
	cfg.Name = "Bad-Name"
	if _, err := BuildTools([]ToolConfig{cfg}, DefaultToolEnv()); err == nil {
		t.Fatal("expected validation failure")
	}
}

func TestBuildToolsApprovalGate(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	cfg := weatherConfig(srv.URL + "/wx/{{city}}")
	cfg.RequireApproval = true
	tools, err := BuildTools([]ToolConfig{cfg}, testEnv())
	if err != nil {
		t.Fatalf("BuildTools: %v", err)
	}

	// Without the approved marker the call defers and the API is NOT hit.
	_, err = tools[0].Exec(context.Background(), Deps{}, json.RawMessage(`{"city":"x","latitude":1}`))
	var deferred *tool.Deferred
	if !errors.As(err, &deferred) || deferred.Kind != tool.DeferApproval {
		t.Fatalf("err = %v, want DeferApproval", err)
	}
	if hits != 0 {
		t.Fatalf("side effect ran before approval (%d hits)", hits)
	}

	// With the approved marker the call executes.
	out, err := tools[0].Exec(tool.WithApprovedCall(context.Background()), Deps{}, json.RawMessage(`{"city":"x","latitude":1}`))
	if err != nil {
		t.Fatalf("approved re-run: %v", err)
	}
	if out != "{}" || hits != 1 {
		t.Fatalf("out = %q hits = %d", out, hits)
	}
}

func TestBuildToolsUngatedExecutesImmediately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	cfg := weatherConfig(srv.URL + "/wx/{{city}}")
	tools, err := BuildTools([]ToolConfig{cfg}, testEnv())
	if err != nil {
		t.Fatalf("BuildTools: %v", err)
	}
	out, err := tools[0].Exec(context.Background(), Deps{}, json.RawMessage(`{"city":"x","latitude":1}`))
	if err != nil || out != `{"ok":true}` {
		t.Fatalf("out = %q err = %v", out, err)
	}
}
