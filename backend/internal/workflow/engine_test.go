package workflow

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEngineLinearExecution(t *testing.T) {
	engine := NewEngine(nil)

	nodes := []Node{
		{
			ID:   "n1",
			Type: "manual",
			Name: "Start",
		},
		{
			ID:   "n2",
			Type: "code_transform",
			Name: "Transform",
			Data: map[string]any{
				"fields": map[string]any{
					"greeting": "Hello {{ $json.name }}",
					"value":    42,
				},
			},
		},
	}

	edges := []Edge{
		{
			ID:     "e1",
			Source: "n1",
			Target: "n2",
		},
	}

	res, err := engine.Execute(context.Background(), nodes, edges, RunOptions{
		InputData: map[string]any{"name": "World"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "success" {
		t.Fatalf("expected success, got %s (err: %s)", res.Status, res.Error)
	}

	out, ok := res.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any output, got %T", res.Output)
	}
	if out["greeting"] != "Hello World" || out["value"] != 42 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(res.NodeResults) != 2 {
		t.Fatalf("expected 2 node results, got %d", len(res.NodeResults))
	}
}

func TestEngineBranchingCondition(t *testing.T) {
	engine := NewEngine(nil)

	nodes := []Node{
		{
			ID:   "trigger",
			Type: "manual",
			Name: "Trigger",
		},
		{
			ID:   "cond",
			Type: "condition",
			Name: "Check Score",
			Data: map[string]any{
				"variable": "{{ $json.score }}",
				"operator": "greater_than",
				"value":    50,
			},
		},
		{
			ID:   "high",
			Type: "code_transform",
			Name: "High Path",
			Data: map[string]any{
				"fields": map[string]any{"level": "high"},
			},
		},
		{
			ID:   "low",
			Type: "code_transform",
			Name: "Low Path",
			Data: map[string]any{
				"fields": map[string]any{"level": "low"},
			},
		},
	}

	edges := []Edge{
		{ID: "e1", Source: "trigger", Target: "cond"},
		{ID: "e2", Source: "cond", Target: "high", SourceHandle: "true"},
		{ID: "e3", Source: "cond", Target: "low", SourceHandle: "false"},
	}

	// Test high branch (score = 80 > 50)
	resHigh, err := engine.Execute(context.Background(), nodes, edges, RunOptions{
		InputData: map[string]any{"score": 80},
	})
	if err != nil || resHigh.Status != "success" {
		t.Fatalf("expected high success: err=%v status=%s", err, resHigh.Status)
	}
	if _, ok := resHigh.NodeResults["high"]; !ok {
		t.Errorf("expected high node to execute")
	}
	if _, ok := resHigh.NodeResults["low"]; ok {
		t.Errorf("expected low node to NOT execute")
	}
	outHigh := resHigh.Output.(map[string]any)
	if outHigh["level"] != "high" {
		t.Errorf("expected level high, got %v", outHigh["level"])
	}

	// Test low branch (score = 20 <= 50)
	resLow, err := engine.Execute(context.Background(), nodes, edges, RunOptions{
		InputData: map[string]any{"score": 20},
	})
	if err != nil || resLow.Status != "success" {
		t.Fatalf("expected low success: err=%v status=%s", err, resLow.Status)
	}
	if _, ok := resLow.NodeResults["low"]; !ok {
		t.Errorf("expected low node to execute")
	}
	if _, ok := resLow.NodeResults["high"]; ok {
		t.Errorf("expected high node to NOT execute")
	}
	outLow := resLow.Output.(map[string]any)
	if outLow["level"] != "low" {
		t.Errorf("expected level low, got %v", outLow["level"])
	}
}

func TestEngineHTTPRequestNode(t *testing.T) {
	// Mock HTTP Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret_token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received": ` + string(body) + `, "ok": true}`))
	}))
	defer server.Close()

	engine := NewEngine(nil)

	nodes := []Node{
		{
			ID:   "t1",
			Type: "manual",
		},
		{
			ID:   "http1",
			Type: "http_request",
			Name: "Fetch API",
			Data: map[string]any{
				"url":    server.URL,
				"method": "POST",
				"auth": map[string]any{
					"type":  "bearer",
					"token": "secret_token",
				},
				"body": map[string]any{
					"msg": "ping",
				},
			},
		},
	}
	edges := []Edge{
		{ID: "e1", Source: "t1", Target: "http1"},
	}

	env := &ExecutionEnvironment{
		HTTPClient:        server.Client(),
		AllowPrivateHosts: true, // test uses 127.0.0.1
	}

	res, err := engine.Execute(context.Background(), nodes, edges, RunOptions{
		Env: env,
	})
	if err != nil || res.Status != "success" {
		t.Fatalf("expected http success: %v, status=%s, err=%s", err, res.Status, res.Error)
	}

	out := res.Output.(map[string]any)
	if out["status"] != 200 {
		t.Errorf("expected status 200, got %v", out["status"])
	}
	data := out["data"].(map[string]any)
	if data["ok"] != true {
		t.Errorf("expected data.ok == true")
	}
}
