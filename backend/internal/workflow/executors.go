package workflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem/model"
)

// ExecutionEnvironment provides dependencies required by node executors.
type ExecutionEnvironment struct {
	HTTPClient        *http.Client
	AllowPrivateHosts bool
	ModelResolver     func(modelName string) (model.StreamingModel, error)
	// ToolRunner executes a saved Golem tool by ID or Name
	ToolRunner func(ctx context.Context, userID, toolIdentifier string, args map[string]any) (string, error)
	UserID     string
}

// NodeExecutor defines the signature of a node type runner.
type NodeExecutor func(ctx context.Context, node Node, evalCtx *EvalContext, env *ExecutionEnvironment) (output any, nextHandle string, err error)

// Registry of built-in node executors
var DefaultExecutors = map[string]NodeExecutor{
	"manual":           executeManualTrigger,
	"trigger_manual":   executeManualTrigger,
	"webhook":          executeWebhookTrigger,
	"trigger_webhook":  executeWebhookTrigger,
	"chat_agent":       executeChatTrigger,
	"trigger_chat":     executeChatTrigger,
	"http_request":     executeHTTPRequest,
	"golem_tool":       executeGolemTool,
	"herbie_tool":      executeGolemTool,
	"condition":        executeCondition,
	"code_transform":   executeCodeTransform,
	"transform":        executeCodeTransform,
	"llm_prompt":       executeLLMPrompt,
	"github":           executeGitHubAction,
	"slack":            executeSlackAction,
	"discord":          executeDiscordAction,
	"webhook_response": executeWebhookResponse,
	"delay":            executeDelay,
}

func executeManualTrigger(_ context.Context, node Node, evalCtx *EvalContext, _ *ExecutionEnvironment) (any, string, error) {
	out := make(map[string]any)
	for k, v := range node.Data {
		out[k] = v
	}
	if inputMap, ok := evalCtx.JSON.(map[string]any); ok {
		for k, v := range inputMap {
			out[k] = v
		}
	} else if evalCtx.JSON != nil {
		out["input"] = evalCtx.JSON
	}
	if len(out) == 0 {
		out["triggeredAt"] = time.Now().Format(time.RFC3339)
	}
	return out, "", nil
}

func executeWebhookTrigger(_ context.Context, _ Node, evalCtx *EvalContext, _ *ExecutionEnvironment) (any, string, error) {
	if evalCtx.JSON != nil {
		return evalCtx.JSON, "", nil
	}
	return map[string]any{
		"headers": map[string]any{},
		"query":   map[string]any{},
		"body":    map[string]any{},
	}, "", nil
}

func executeChatTrigger(_ context.Context, _ Node, evalCtx *EvalContext, _ *ExecutionEnvironment) (any, string, error) {
	if evalCtx.JSON != nil {
		return evalCtx.JSON, "", nil
	}
	return map[string]any{"prompt": ""}, "", nil
}

func executeHTTPRequest(ctx context.Context, node Node, evalCtx *EvalContext, env *ExecutionEnvironment) (any, string, error) {
	client := env.HTTPClient
	if client == nil {
		client = NewSafeHTTPClient(env.AllowPrivateHosts, 20*time.Second)
	}

	rawURL, _ := node.Data["url"].(string)
	if rawURL == "" {
		return nil, "", fmt.Errorf("url is required")
	}
	resolvedURL := fmt.Sprintf("%v", ResolveString(rawURL, evalCtx))

	method, _ := node.Data["method"].(string)
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(strings.TrimSpace(method))

	// SSRF check
	if _, err := ValidatePublicURL(ctx, resolvedURL, env.AllowPrivateHosts); err != nil {
		return nil, "", err
	}

	var reqBody io.Reader
	if rawBody, ok := node.Data["body"]; ok && rawBody != nil {
		resolvedBody := ResolveAny(rawBody, evalCtx)
		switch b := resolvedBody.(type) {
		case string:
			if strings.TrimSpace(b) != "" {
				reqBody = strings.NewReader(b)
			}
		default:
			bs, err := json.Marshal(b)
			if err != nil {
				return nil, "", fmt.Errorf("marshal body: %w", err)
			}
			reqBody = bytes.NewReader(bs)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, resolvedURL, reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	// Default user agent & content type
	req.Header.Set("User-Agent", "GolemWorkflow/1.0")
	if reqBody != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Custom headers
	if rawHeaders, ok := node.Data["headers"].(map[string]any); ok {
		for k, v := range rawHeaders {
			resolvedVal := fmt.Sprintf("%v", ResolveAny(v, evalCtx))
			req.Header.Set(k, resolvedVal)
		}
	}

	// Auth handling
	if authData, ok := node.Data["auth"].(map[string]any); ok {
		authType, _ := authData["type"].(string)
		switch authType {
		case "bearer":
			tok := fmt.Sprintf("%v", ResolveAny(authData["token"], evalCtx))
			req.Header.Set("Authorization", "Bearer "+tok)
		case "basic":
			user := fmt.Sprintf("%v", ResolveAny(authData["username"], evalCtx))
			pass := fmt.Sprintf("%v", ResolveAny(authData["password"], evalCtx))
			enc := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
			req.Header.Set("Authorization", "Basic "+enc)
		case "api_key":
			keyName := fmt.Sprintf("%v", ResolveAny(authData["keyName"], evalCtx))
			keyVal := fmt.Sprintf("%v", ResolveAny(authData["keyValue"], evalCtx))
			inHeader, _ := authData["inHeader"].(bool)
			if inHeader || authData["inHeader"] == nil {
				req.Header.Set(keyName, keyVal)
			} else {
				q := req.URL.Query()
				q.Set(keyName, keyVal)
				req.URL.RawQuery = q.Encode()
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5MB cap
	if err != nil {
		return nil, "", fmt.Errorf("read response body: %w", err)
	}

	var parsedData any
	if err := json.Unmarshal(bodyBytes, &parsedData); err != nil {
		parsedData = string(bodyBytes)
	}

	respHeaders := map[string]string{}
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	out := map[string]any{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"headers":    respHeaders,
		"data":       parsedData,
	}

	if resp.StatusCode >= 400 {
		return out, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return out, "", nil
}

func executeGolemTool(ctx context.Context, node Node, evalCtx *EvalContext, env *ExecutionEnvironment) (any, string, error) {
	if env.ToolRunner == nil {
		return nil, "", fmt.Errorf("tool runner not configured in environment")
	}

	toolIdentifier, _ := node.Data["tool"].(string)
	if toolIdentifier == "" {
		toolIdentifier, _ = node.Data["toolName"].(string)
	}
	if toolIdentifier == "" {
		return nil, "", fmt.Errorf("tool identifier is required")
	}

	rawArgs, _ := node.Data["args"].(map[string]any)
	resolvedArgs := make(map[string]any, len(rawArgs))
	for k, v := range rawArgs {
		resolvedArgs[k] = ResolveAny(v, evalCtx)
	}

	rawResult, err := env.ToolRunner(ctx, env.UserID, toolIdentifier, resolvedArgs)
	if err != nil {
		return nil, "", fmt.Errorf("tool execution failed: %w", err)
	}

	var parsed any
	if err := json.Unmarshal([]byte(rawResult), &parsed); err != nil {
		parsed = rawResult
	}
	return parsed, "", nil
}

func executeCondition(_ context.Context, node Node, evalCtx *EvalContext, _ *ExecutionEnvironment) (any, string, error) {
	varPassed := true

	// Check if conditions array is provided
	if conditions, ok := node.Data["conditions"].([]any); ok && len(conditions) > 0 {
		for _, c := range conditions {
			condMap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if !evalSingleCondition(condMap, evalCtx) {
				varPassed = false
				break
			}
		}
	} else {
		// Single condition in node.Data
		varPassed = evalSingleCondition(node.Data, evalCtx)
	}

	handle := "false"
	if varPassed {
		handle = "true"
	}

	return map[string]any{
		"result": varPassed,
		"handle": handle,
	}, handle, nil
}

func evalSingleCondition(cond map[string]any, evalCtx *EvalContext) bool {
	rawVar := cond["variable"]
	operator, _ := cond["operator"].(string)
	rawValue := cond["value"]

	left := ResolveAny(rawVar, evalCtx)
	right := ResolveAny(rawValue, evalCtx)

	switch operator {
	case "equals", "==":
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
	case "not_equals", "!=":
		return fmt.Sprintf("%v", left) != fmt.Sprintf("%v", right)
	case "contains":
		return strings.Contains(fmt.Sprintf("%v", left), fmt.Sprintf("%v", right))
	case "not_contains":
		return !strings.Contains(fmt.Sprintf("%v", left), fmt.Sprintf("%v", right))
	case "greater_than", ">":
		lf, lok := toFloat(left)
		rf, rok := toFloat(right)
		return lok && rok && lf > rf
	case "less_than", "<":
		lf, lok := toFloat(left)
		rf, rok := toFloat(right)
		return lok && rok && lf < rf
	case "is_empty":
		if left == nil {
			return true
		}
		s := strings.TrimSpace(fmt.Sprintf("%v", left))
		return s == "" || s == "<nil>" || s == "[]" || s == "{}"
	case "is_not_empty":
		if left == nil {
			return false
		}
		s := strings.TrimSpace(fmt.Sprintf("%v", left))
		return s != "" && s != "<nil>" && s != "[]" && s != "{}"
	default:
		// Default to equality
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func executeCodeTransform(_ context.Context, node Node, evalCtx *EvalContext, _ *ExecutionEnvironment) (any, string, error) {
	rawFields, _ := node.Data["fields"].(map[string]any)
	if len(rawFields) > 0 {
		out := make(map[string]any, len(rawFields))
		for k, v := range rawFields {
			out[k] = ResolveAny(v, evalCtx)
		}
		return out, "", nil
	}

	// If template expression is given directly
	if rawOutput, ok := node.Data["output"]; ok && rawOutput != nil {
		return ResolveAny(rawOutput, evalCtx), "", nil
	}

	// Passthrough
	return evalCtx.JSON, "", nil
}

func executeLLMPrompt(ctx context.Context, node Node, evalCtx *EvalContext, env *ExecutionEnvironment) (any, string, error) {
	if env.ModelResolver == nil {
		return nil, "", fmt.Errorf("model resolver not configured in environment")
	}

	modelName, _ := node.Data["model"].(string)
	if modelName == "" {
		modelName = "gemini-3.5-flash"
	}

	rawPrompt, _ := node.Data["prompt"].(string)
	if rawPrompt == "" {
		return nil, "", fmt.Errorf("prompt is required")
	}
	resolvedPrompt := fmt.Sprintf("%v", ResolveString(rawPrompt, evalCtx))

	systemPrompt, _ := node.Data["systemPrompt"].(string)
	if systemPrompt != "" {
		systemPrompt = fmt.Sprintf("%v", ResolveString(systemPrompt, evalCtx))
	}

	streamingModel, err := env.ModelResolver(modelName)
	if err != nil {
		return nil, "", fmt.Errorf("resolve model %q: %w", modelName, err)
	}

	messages := []model.Message{}
	if systemPrompt != "" {
		messages = append(messages, model.Message{Role: model.RoleSystem, Content: systemPrompt})
	}
	messages = append(messages, model.Message{Role: model.RoleUser, Content: resolvedPrompt})

	resp, err := streamingModel.Generate(ctx, model.Request{
		Messages: messages,
	})
	if err != nil {
		return nil, "", fmt.Errorf("call model %q: %w", modelName, err)
	}

	fullText := strings.TrimSpace(resp.Message.Content)

	out := map[string]any{
		"text":  fullText,
		"model": modelName,
	}

	jsonOutput, _ := node.Data["jsonOutput"].(bool)
	if jsonOutput || strings.HasPrefix(fullText, "{") || strings.HasPrefix(fullText, "[") {
		var jsonParsed any
		// Clean markdown fences if model returned ```json ... ```
		cleanJSON := fullText
		if strings.HasPrefix(cleanJSON, "```") {
			lines := strings.Split(cleanJSON, "\n")
			if len(lines) >= 2 {
				cleanJSON = strings.Join(lines[1:len(lines)-1], "\n")
			}
		}
		if err := json.Unmarshal([]byte(cleanJSON), &jsonParsed); err == nil {
			out["json"] = jsonParsed
		}
	}

	return out, "", nil
}

func executeGitHubAction(ctx context.Context, node Node, evalCtx *EvalContext, env *ExecutionEnvironment) (any, string, error) {
	action, _ := node.Data["action"].(string)
	owner := fmt.Sprintf("%v", ResolveAny(node.Data["owner"], evalCtx))
	repo := fmt.Sprintf("%v", ResolveAny(node.Data["repo"], evalCtx))
	token := fmt.Sprintf("%v", ResolveAny(node.Data["token"], evalCtx))

	if owner == "" || repo == "" {
		return nil, "", fmt.Errorf("github owner and repo are required")
	}

	client := env.HTTPClient
	if client == nil {
		client = NewSafeHTTPClient(env.AllowPrivateHosts, 20*time.Second)
	}

	switch action {
	case "create_issue":
		title := fmt.Sprintf("%v", ResolveAny(node.Data["title"], evalCtx))
		body := fmt.Sprintf("%v", ResolveAny(node.Data["body"], evalCtx))
		if title == "" {
			return nil, "", fmt.Errorf("issue title is required")
		}

		payload, _ := json.Marshal(map[string]any{"title": title, "body": body})
		req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo), bytes.NewReader(payload))
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "GolemWorkflow/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)

		var out any
		_ = json.Unmarshal(b, &out)
		if resp.StatusCode >= 400 {
			return out, "", fmt.Errorf("github error %d: %s", resp.StatusCode, string(b))
		}
		return out, "", nil

	case "get_repo":
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo), nil)
		if err != nil {
			return nil, "", err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "GolemWorkflow/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)

		var out any
		_ = json.Unmarshal(b, &out)
		return out, "", nil

	default:
		return nil, "", fmt.Errorf("unsupported github action %q", action)
	}
}

func executeSlackAction(ctx context.Context, node Node, evalCtx *EvalContext, env *ExecutionEnvironment) (any, string, error) {
	webhookURL := fmt.Sprintf("%v", ResolveAny(node.Data["webhookUrl"], evalCtx))
	text := fmt.Sprintf("%v", ResolveAny(node.Data["text"], evalCtx))

	if webhookURL == "" {
		return nil, "", fmt.Errorf("slack webhookUrl is required")
	}
	if err := ValidateSlackWebhookURL(webhookURL); err != nil {
		return nil, "", err
	}

	payload, _ := json.Marshal(map[string]any{"text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := env.HTTPClient
	if client == nil {
		client = NewSafeHTTPClient(env.AllowPrivateHosts, 20*time.Second)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return string(b), "", fmt.Errorf("slack error %d: %s", resp.StatusCode, string(b))
	}
	return map[string]any{"ok": true, "response": string(b)}, "", nil
}

func executeDiscordAction(ctx context.Context, node Node, evalCtx *EvalContext, env *ExecutionEnvironment) (any, string, error) {
	webhookURL := fmt.Sprintf("%v", ResolveAny(node.Data["webhookUrl"], evalCtx))
	content := fmt.Sprintf("%v", ResolveAny(node.Data["content"], evalCtx))

	if webhookURL == "" {
		return nil, "", fmt.Errorf("discord webhookUrl is required")
	}
	if err := ValidateDiscordWebhookURL(webhookURL); err != nil {
		return nil, "", err
	}

	bodyMap := map[string]any{"content": content}
	if uname, ok := node.Data["username"].(string); ok && uname != "" {
		bodyMap["username"] = fmt.Sprintf("%v", ResolveAny(uname, evalCtx))
	}

	payload, _ := json.Marshal(bodyMap)
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := env.HTTPClient
	if client == nil {
		client = NewSafeHTTPClient(env.AllowPrivateHosts, 20*time.Second)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return string(b), "", fmt.Errorf("discord error %d: %s", resp.StatusCode, string(b))
	}
	return map[string]any{"ok": true, "response": string(b)}, "", nil
}

func executeWebhookResponse(_ context.Context, node Node, evalCtx *EvalContext, _ *ExecutionEnvironment) (any, string, error) {
	status := 200
	if rawStatus, ok := node.Data["statusCode"]; ok {
		if s, ok := toFloat(rawStatus); ok && s > 0 {
			status = int(s)
		}
	}

	resolvedBody := ResolveAny(node.Data["body"], evalCtx)
	headers := map[string]string{}
	if rawHeaders, ok := node.Data["headers"].(map[string]any); ok {
		for k, v := range rawHeaders {
			headers[k] = fmt.Sprintf("%v", ResolveAny(v, evalCtx))
		}
	}

	return map[string]any{
		"statusCode": status,
		"headers":    headers,
		"body":       resolvedBody,
	}, "", nil
}

func executeDelay(ctx context.Context, node Node, evalCtx *EvalContext, _ *ExecutionEnvironment) (any, string, error) {
	seconds := 1
	if s, ok := toFloat(node.Data["seconds"]); ok && s > 0 {
		seconds = int(s)
	}
	if seconds > 10 {
		seconds = 10 // safety cap
	}

	select {
	case <-time.After(time.Duration(seconds) * time.Second):
		return evalCtx.JSON, "", nil
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
}

// checkHost guards against SSRF to private/loopback/link-local networks.
func checkHost(ctx context.Context, hostPort string) error {
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
		return fmt.Errorf("private or loopback address %s is disallowed", ip)
	}
	return nil
}
