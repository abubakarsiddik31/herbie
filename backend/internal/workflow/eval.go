package workflow

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Matches `{{ expr }}`
	exprRegex = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)
)

// EvalContext holds all available scopes for expression evaluation.
type EvalContext struct {
	JSON        any                       // $json: data from the preceding node
	Nodes       map[string]any            // $nodes: outputs of previously executed nodes by ID and Name
	Env         map[string]string         // $env: environment variables
	Credentials map[string]map[string]any // $credentials: stored credentials by name
}

// CollectCredentialSecrets extracts all string values from credentials for redaction filtering.
func CollectCredentialSecrets(creds map[string]map[string]any) []string {
	if creds == nil {
		return nil
	}
	var secrets []string
	for _, cred := range creds {
		for _, v := range cred {
			if s, ok := v.(string); ok && len(strings.TrimSpace(s)) >= 4 {
				secrets = append(secrets, strings.TrimSpace(s))
			}
		}
	}
	return secrets
}

// ResolveAny recursively evaluates expressions in strings, slices, and maps.
func ResolveAny(input any, ctx *EvalContext) any {
	if ctx == nil {
		return input
	}
	switch v := input.(type) {
	case string:
		return ResolveString(v, ctx)
	case map[string]any:
		res := make(map[string]any, len(v))
		for k, val := range v {
			res[k] = ResolveAny(val, ctx)
		}
		return res
	case []any:
		res := make([]any, len(v))
		for i, val := range v {
			res[i] = ResolveAny(val, ctx)
		}
		return res
	default:
		return input
	}
}

// ResolveString evaluates an expression in a string.
// If the string is strictly `{{ expr }}`, returns the typed value.
// Otherwise, performs string substitution.
func ResolveString(input string, ctx *EvalContext) any {
	trimmed := strings.TrimSpace(input)
	// Check if input is exactly one expression: `{{ expr }}`
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") {
		sub := trimmed[2 : len(trimmed)-2]
		if !strings.Contains(sub, "{{") && !strings.Contains(sub, "}}") {
			return evalExpr(strings.TrimSpace(sub), ctx)
		}
	}

	// Otherwise replace each `{{ expr }}` with its string value
	return exprRegex.ReplaceAllStringFunc(input, func(m string) string {
		sub := exprRegex.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		val := evalExpr(strings.TrimSpace(sub[1]), ctx)
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	})
}

// evalExpr parses and resolves a single expression like:
// $json.user.name
// $nodes["Fetch Weather"].temperature
// $nodes.node_123.data
// $credentials.github.token
// $env.APP_NAME
func evalExpr(expr string, ctx *EvalContext) any {
	if ctx == nil || expr == "" {
		return nil
	}

	parts := tokenizePath(expr)
	if len(parts) == 0 {
		return nil
	}

	rootToken := parts[0]
	var current any

	switch rootToken {
	case "$json":
		current = ctx.JSON
	case "$env":
		if len(parts) > 1 && ctx.Env != nil {
			return ctx.Env[parts[1]]
		}
		return ctx.Env
	case "$credentials":
		if len(parts) > 1 && ctx.Credentials != nil {
			credName := parts[1]
			credMap := ctx.Credentials[credName]
			if len(parts) == 2 {
				return credMap
			}
			return getProperty(credMap, parts[2:])
		}
		return ctx.Credentials
	case "$nodes":
		if len(parts) > 1 && ctx.Nodes != nil {
			nodeKey := parts[1]
			nodeOut := ctx.Nodes[nodeKey]
			if len(parts) == 2 {
				return nodeOut
			}
			return getProperty(nodeOut, parts[2:])
		}
		return ctx.Nodes
	default:
		// Fallback: check if it directly refers to a key in $json
		current = ctx.JSON
		return getProperty(current, parts)
	}

	return getProperty(current, parts[1:])
}

// tokenizePath splits expressions like `$nodes["My Node"].items[0].name` into
// ["$nodes", "My Node", "items", "0", "name"].
func tokenizePath(expr string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		switch {
		case inQuote:
			if ch == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(ch)
			}
		case ch == '"' || ch == '\'':
			inQuote = true
			quoteChar = ch
		case ch == '.' || ch == '[' || ch == ']':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// getProperty traverses nested maps and slices by keys/indexes.
func getProperty(obj any, path []string) any {
	current := obj
	for _, key := range path {
		if current == nil {
			return nil
		}

		switch v := current.(type) {
		case map[string]any:
			current = v[key]
		case map[string]string:
			current = v[key]
		case []any:
			idx, err := strconv.Atoi(key)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			current = v[idx]
		default:
			return nil
		}
	}
	return current
}
