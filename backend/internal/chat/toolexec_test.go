package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
)

// testEnv allows private hosts so httptest's 127.0.0.1 endpoints are reachable.
func testEnv() ToolEnv {
	env := DefaultToolEnv()
	env.AllowPrivateHosts = true
	return env
}

func weatherConfig(serverURL string) ToolConfig {
	return ToolConfig{
		Name:        "get_weather",
		Description: "weather",
		Method:      "GET",
		URLTemplate: serverURL + "/forecast",
		Params: []ParamDef{
			{Name: "latitude", In: "query", Type: "number", Required: true, Description: "lat"},
			{Name: "city", In: "path", Type: "string", Required: true, Description: "city"},
		},
	}
}

func TestExecuteSubstitutesPathAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.EscapedPath(), r.URL.Query().Get("latitude")
		_, _ = io.WriteString(w, `{"temp":21}`)
	}))
	defer srv.Close()

	cfg := weatherConfig(srv.URL)
	cfg.URLTemplate = srv.URL + "/city/{{city}}"
	args := json.RawMessage(`{"city":"São Paulo","latitude":40.7}`)
	out, err := executeHTTPTool(context.Background(), cfg, testEnv(), srv.Client(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotPath != "/city/S%C3%A3o%20Paulo" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "40.7" {
		t.Fatalf("latitude query = %q", gotQuery)
	}
	if !strings.Contains(out, `"temp"`) {
		t.Fatalf("body not returned: %q", out)
	}
}

func TestExecuteOptionalQueryOmittedWhenAbsent(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("unit")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	cfg := weatherConfig(srv.URL)
	cfg.URLTemplate = srv.URL + "/wx/{{city}}"
	cfg.Params = append(cfg.Params, ParamDef{Name: "unit", In: "query", Type: "string"})
	if _, err := executeHTTPTool(context.Background(), cfg, testEnv(), srv.Client(), json.RawMessage(`{"city":"x","latitude":1}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("absent optional param was sent: %q", gotQuery)
	}
}

func TestExecutePostBodyTemplate(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := weatherConfig(srv.URL)
	cfg.Method = "POST"
	cfg.BodyTemplate = `{"lat": {{latitude}}, "opt": {{unit}}}`
	cfg.Params = append(cfg.Params, ParamDef{Name: "unit", In: "query", Type: "string"})
	if _, err := executeHTTPTool(context.Background(), cfg, testEnv(), srv.Client(), json.RawMessage(`{"city":"x","latitude":40.5}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if string(gotBody) != `{"lat": 40.5, "opt": null}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestExecuteArgValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	cfg := weatherConfig(srv.URL)

	cases := map[string]json.RawMessage{
		"missing required": json.RawMessage(`{}`),
		"wrong type":       json.RawMessage(`{"city":"x","latitude":"high"}`),
		"unknown key":      json.RawMessage(`{"city":"x","latitude":1,"extra":true}`),
		"invalid json":     json.RawMessage(`{`),
	}
	for name, args := range cases {
		if _, err := executeHTTPTool(context.Background(), cfg, testEnv(), srv.Client(), args); err == nil {
			t.Errorf("%s: expected error", name)
		} else {
			var retry *model.ModelRetry
			if !errors.As(err, &retry) {
				t.Errorf("%s: err %T, want *model.ModelRetry", name, err)
			}
		}
	}
}

func TestExecuteNon2xxIsAResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"nope"}`)
	}))
	defer srv.Close()

	cfg := weatherConfig(srv.URL)
	out, err := executeHTTPTool(context.Background(), cfg, testEnv(), srv.Client(), json.RawMessage(`{"city":"x","latitude":1}`))
	if err != nil {
		t.Fatalf("non-2xx must not fail the run: %v", err)
	}
	if !strings.HasPrefix(out, "HTTP 404") {
		t.Fatalf("output = %q", out)
	}
}

func TestExecuteTransportErrorIsAResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cfg := weatherConfig(srv.URL)
	srv.Close() // dead endpoint

	out, err := executeHTTPTool(context.Background(), cfg, testEnv(), srv.Client(), json.RawMessage(`{"city":"x","latitude":1}`))
	if err != nil {
		t.Fatalf("transport error must not fail the run: %v", err)
	}
	if !strings.HasPrefix(out, "Tool error:") {
		t.Fatalf("output = %q", out)
	}
}

func TestExecuteResultTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 100))
	}))
	defer srv.Close()

	cfg := weatherConfig(srv.URL)
	env := testEnv()
	env.ResultMaxBytes = 10
	out, err := executeHTTPTool(context.Background(), cfg, env, srv.Client(), json.RawMessage(`{"city":"x","latitude":1}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "[truncated]") || len(out) > 40 {
		t.Fatalf("output = %q", out)
	}
}

func TestExecuteBlockedHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	cfg := weatherConfig("http://169.254.169.254/latest") // link-local metadata endpoint
	out, err := executeHTTPTool(context.Background(), cfg, DefaultToolEnv(), srv.Client(), json.RawMessage(`{"city":"x","latitude":1}`))
	if err != nil {
		t.Fatalf("blocked host must be a result, not an error: %v", err)
	}
	if !strings.Contains(out, "blocked") {
		t.Fatalf("output = %q", out)
	}
}

func TestCheckPublicHost(t *testing.T) {
	blocked := []string{
		"127.0.0.1:8080", "10.0.0.1:443", "192.168.1.1:443", "169.254.169.254:80",
		"0.0.0.0:80", "[::1]:8080", "[::ffff:127.0.0.1]:80", "[::ffff:169.254.169.254]:80",
	}
	for _, host := range blocked {
		if err := checkPublicHost(context.Background(), host); err == nil {
			t.Errorf("host %q should be blocked", host)
		}
	}
}

func TestNewToolHTTPClientBlocksPrivateRedirect(t *testing.T) {
	privateSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "private data")
	}))
	defer privateSrv.Close()

	redirectSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, privateSrv.URL, http.StatusFound)
	}))
	defer redirectSrv.Close()

	client := newToolHTTPClient(DefaultToolEnv()) // AllowPrivateHosts = false
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, redirectSrv.URL, nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected request or redirect to private server to be blocked")
	}
}
