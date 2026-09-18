package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem/model"
)

// ToolEnv bounds one tool execution. It comes from config (Task 2) and is
// shared by every user tool in the process.
type ToolEnv struct {
	HTTPTimeout       time.Duration
	HTTPMaxBytes      int64
	ResultMaxBytes    int64
	AllowPrivateHosts bool
}

// DefaultToolEnv matches the config defaults; tests and interim callers use it.
func DefaultToolEnv() ToolEnv {
	return ToolEnv{
		HTTPTimeout:       20 * time.Second,
		HTTPMaxBytes:      1 << 20,
		ResultMaxBytes:    32 << 10,
		AllowPrivateHosts: false,
	}
}

// parseExecArgs decodes and validates the model's raw arguments against the
// tool's declared params. Every violation returns *model.ModelRetry so the
// run's tool retry budget feeds it back to the model for correction.
func parseExecArgs(cfg ToolConfig, args json.RawMessage) (map[string]any, error) {
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(args))
	if err := dec.Decode(&m); err != nil {
		return nil, &model.ModelRetry{Err: fmt.Errorf("arguments must be a JSON object: %w", err)}
	}
	declared := map[string]ParamDef{}
	for _, p := range cfg.Params {
		declared[p.Name] = p
	}
	for k, v := range m {
		p, ok := declared[k]
		if !ok {
			return nil, &model.ModelRetry{Err: fmt.Errorf("unknown argument %q", k)}
		}
		switch p.Type {
		case "string":
			if _, ok := v.(string); !ok {
				return nil, &model.ModelRetry{Err: fmt.Errorf("argument %q must be a string", k)}
			}
		case "number":
			if _, ok := v.(float64); !ok {
				return nil, &model.ModelRetry{Err: fmt.Errorf("argument %q must be a number", k)}
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return nil, &model.ModelRetry{Err: fmt.Errorf("argument %q must be a boolean", k)}
			}
		}
	}
	for _, p := range cfg.Params {
		if p.Required {
			if _, ok := m[p.Name]; !ok {
				return nil, &model.ModelRetry{Err: fmt.Errorf("argument %q is required", p.Name)}
			}
		}
	}
	return m, nil
}

// argString renders an argument for URL positions.
func argString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// argJSON renders an argument for JSON body positions: numbers and booleans
// inline, strings quoted, absent values as null.
func argJSON(v any, present bool) string {
	if !present {
		return "null"
	}
	if s, ok := v.(string); ok {
		b, _ := json.Marshal(s)
		return string(b)
	}
	return argString(v)
}

// renderURL substitutes path placeholders and appends declared query params.
func renderURL(cfg ToolConfig, args map[string]any) (string, error) {
	raw, err := replacePlaceholders(cfg.URLTemplate,
		func(name string) (string, error) {
			v, ok := args[name]
			if !ok {
				return "", fmt.Errorf("missing path parameter %q", name)
			}
			return argString(v), nil
		}, url.PathEscape)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("rendered url: %w", err)
	}
	q := u.Query()
	for _, p := range cfg.Params {
		if p.In != "query" {
			continue
		}
		if v, ok := args[p.Name]; ok {
			q.Set(p.Name, argString(v))
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// renderBody substitutes placeholders in the body template; ok is false when
// the tool has no body template.
func renderBody(cfg ToolConfig, args map[string]any) ([]byte, bool, error) {
	if cfg.BodyTemplate == "" {
		return nil, false, nil
	}
	out, err := replacePlaceholders(cfg.BodyTemplate,
		func(name string) (string, error) {
			v, ok := args[name]
			return argJSON(v, ok), nil
		}, func(s string) string { return s })
	if err != nil {
		return nil, false, err
	}
	return []byte(out), true, nil
}

// checkPublicHost rejects loopback, private, link-local, and unspecified
// addresses — the SSRF guard for user-configured URLs. hostPort is host:port
// or host; names are resolved and every resolved address must pass.
func checkPublicHost(ctx context.Context, hostPort string) error {
	host := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return checkIP(ip)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", host, err)
	}
	for _, ip := range ips {
		if err := checkIP(ip.IP); err != nil {
			return err
		}
	}
	return nil
}

func checkIP(ip net.IP) error {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("private or link-local address %s", ip)
	}
	return nil
}

// newToolHTTPClient builds an HTTP client bounded by the tool environment,
// with socket-level IP validation against SSRF / DNS rebinding and redirect
// filtering when private hosts are disallowed.
func newToolHTTPClient(env ToolEnv) *http.Client {
	dialer := &net.Dialer{
		Timeout:   env.HTTPTimeout,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if !env.AllowPrivateHosts {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve %q: %w", host, err)
			}
			var lastErr error
			for _, ip := range ips {
				if err := checkIP(ip.IP); err != nil {
					lastErr = fmt.Errorf("host %s resolved to blocked address: %w", host, err)
					continue
				}
				target := net.JoinHostPort(ip.IP.String(), port)
				conn, err := dialer.DialContext(ctx, network, target)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("no allowed addresses for %s", host)
		}
	} else {
		transport.DialContext = dialer.DialContext
	}

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if !env.AllowPrivateHosts {
				if err := checkPublicHost(req.Context(), req.URL.Host); err != nil {
					return fmt.Errorf("redirect blocked: %w", err)
				}
			}
			return nil
		},
	}
}

// truncateResult caps the tool result handed back to the model, keeping the
// boundary on a valid UTF-8 rune.
func truncateResult(s string, limit int64) string {
	if limit <= 0 || int64(len(s)) <= limit {
		return s
	}
	cut := s[:limit]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "\n[truncated]"
}

// executeHTTPTool runs one user tool call end to end. It never fails the run
// for anything the model or the API did: argument problems come back as
// *model.ModelRetry, and API/network problems come back as text results the
// model can explain to the user. Only ctx cancellation propagates.
func executeHTTPTool(ctx context.Context, cfg ToolConfig, env ToolEnv, client *http.Client, args json.RawMessage) (string, error) {
	if client == nil {
		client = newToolHTTPClient(env)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	parsed, perr := parseExecArgs(cfg, args)
	if perr != nil {
		return "", perr
	}
	target, err := renderURL(cfg, parsed)
	if err != nil {
		return "Tool error: " + err.Error(), nil
	}
	if !env.AllowPrivateHosts {
		if u, perr := url.Parse(target); perr == nil {
			if herr := checkPublicHost(ctx, u.Host); herr != nil {
				return "Tool error: host blocked (" + u.Host + ")", nil
			}
		}
	}
	body, hasBody, err := renderBody(cfg, parsed)
	if err != nil {
		return "Tool error: " + err.Error(), nil
	}
	req, err := http.NewRequestWithContext(ctx, cfg.Method, target, bytes.NewReader(body))
	if err != nil {
		return "Tool error: " + err.Error(), nil
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "GolemChatbot/1.0 (https://github.com/abubakarsiddik31/golem)")
	}
	if hasBody && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "Tool error: " + err.Error(), nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, env.HTTPMaxBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "Tool error: read response: " + err.Error(), nil
	}
	if int64(len(raw)) > env.HTTPMaxBytes {
		raw = raw[:env.HTTPMaxBytes]
	}
	var out string
	if resp.StatusCode >= 300 {
		out = fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, http.StatusText(resp.StatusCode), raw)
	} else {
		out = string(raw)
	}
	return truncateResult(out, env.ResultMaxBytes), nil
}
