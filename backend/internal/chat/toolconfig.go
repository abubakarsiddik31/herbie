package chat

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// ParamDef is one user-declared tool parameter. In selects where it goes on
// the request (path placeholder or query string); Type is the JSON Schema type.
type ParamDef struct {
	Name        string `json:"name"`
	In          string `json:"in"`   // path | query
	Type        string `json:"type"` // string | number | boolean
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// ToolConfig is the user-authored definition of one HTTP API tool. It is the
// wire shape for the tools API and the input to BuildTools.
type ToolConfig struct {
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Method          string            `json:"method"`
	URLTemplate     string            `json:"url_template"`
	Params          []ParamDef        `json:"params"`
	BodyTemplate    string            `json:"body_template"`
	Headers         map[string]string `json:"headers"`
	RequireApproval bool              `json:"require_approval"`
}

const (
	maxToolDescriptionLen = 2000
	maxToolURLLen         = 2048
	maxBodyTemplateBytes  = 8192
	maxParams             = 16
	maxHeaders            = 16
	maxHeaderKeyBytes     = 128
	maxHeaderValueBytes   = 1024
)

var (
	nameRE         = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	paramNameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	placeholderRE  = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)
	allowedMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	allowedIn      = map[string]bool{"path": true, "query": true}
	allowedTypes   = map[string]bool{"string": true, "number": true, "boolean": true}
)

// Validate enforces the authoring rules from the design spec. It returns the
// first violation as a user-facing error message.
func (c ToolConfig) Validate() error {
	if !nameRE.MatchString(c.Name) {
		return fmt.Errorf("tool name must match ^[a-z][a-z0-9_]{0,63}$")
	}
	if n := utf8.RuneCountInString(c.Description); n < 1 || n > maxToolDescriptionLen {
		return fmt.Errorf("description must be 1-%d characters", maxToolDescriptionLen)
	}
	if !allowedMethods[c.Method] {
		return fmt.Errorf("method must be one of GET, POST, PUT, PATCH, DELETE")
	}
	if len(c.URLTemplate) == 0 || len(c.URLTemplate) > maxToolURLLen {
		return fmt.Errorf("url template must be 1-%d characters", maxToolURLLen)
	}
	if len(c.Params) > maxParams {
		return fmt.Errorf("at most %d parameters are allowed", maxParams)
	}
	declared := map[string]ParamDef{}
	for _, p := range c.Params {
		if !paramNameRE.MatchString(p.Name) {
			return fmt.Errorf("parameter %q: name must match ^[A-Za-z_][A-Za-z0-9_]{0,63}$", p.Name)
		}
		if declared[p.Name].In != "" {
			return fmt.Errorf("parameter %q: duplicate", p.Name)
		}
		if !allowedIn[p.In] {
			return fmt.Errorf("parameter %q: location must be path or query", p.Name)
		}
		if !allowedTypes[p.Type] {
			return fmt.Errorf("parameter %q: type must be string, number, or boolean", p.Name)
		}
		declared[p.Name] = p
	}
	// Every {{placeholder}} in the URL must name a declared path param, and
	// every declared path param must appear in the URL.
	placeholders := map[string]bool{}
	for _, m := range placeholderRE.FindAllStringSubmatch(c.URLTemplate, -1) {
		name := m[1]
		placeholders[name] = true
		if p, ok := declared[name]; !ok {
			return fmt.Errorf("url template: {{%s}} is not a declared parameter", name)
		} else if p.In != "path" {
			return fmt.Errorf("url template: {{%s}} is declared as a query parameter", name)
		}
	}
	for _, p := range c.Params {
		if p.In == "path" && !placeholders[p.Name] {
			return fmt.Errorf("parameter %q: declared as path but not used in the url template", p.Name)
		}
	}
	sample := placeholderRE.ReplaceAllString(c.URLTemplate, "x")
	u, err := url.Parse(sample)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("url template must be an absolute http(s) URL")
	}
	if c.BodyTemplate != "" {
		if c.Method != "POST" && c.Method != "PUT" && c.Method != "PATCH" {
			return fmt.Errorf("body template is only valid for POST, PUT, PATCH")
		}
		if len(c.BodyTemplate) > maxBodyTemplateBytes {
			return fmt.Errorf("body template must be at most %d bytes", maxBodyTemplateBytes)
		}
		for _, m := range placeholderRE.FindAllStringSubmatch(c.BodyTemplate, -1) {
			if _, ok := declared[m[1]]; !ok {
				return fmt.Errorf("body template: {{%s}} is not a declared parameter", m[1])
			}
		}
		if !json.Valid([]byte(sampleJSON(c.BodyTemplate, declared))) {
			return fmt.Errorf("body template does not substitute to valid JSON")
		}
	}
	if len(c.Headers) > maxHeaders {
		return fmt.Errorf("at most %d headers are allowed", maxHeaders)
	}
	for k, v := range c.Headers {
		if k == "" || len(k) > maxHeaderKeyBytes {
			return fmt.Errorf("header %q: invalid key", k)
		}
		if len(v) > maxHeaderValueBytes {
			return fmt.Errorf("header %q: value must be at most %d bytes", k, maxHeaderValueBytes)
		}
	}
	return nil
}

// sampleJSON substitutes each placeholder with a type-shaped JSON sample so
// validity of the body template can be checked at authoring time.
func sampleJSON(tpl string, declared map[string]ParamDef) string {
	return placeholderRE.ReplaceAllStringFunc(tpl, func(m string) string {
		p, ok := declared[placeholderRE.FindStringSubmatch(m)[1]]
		if !ok {
			return m
		}
		switch p.Type {
		case "number":
			return "1"
		case "boolean":
			return "true"
		default:
			return `"x"`
		}
	})
}

// Schema builds the JSON Schema golem advertises to the model.
func (c ToolConfig) Schema() (json.RawMessage, error) {
	properties := map[string]any{}
	var required []string
	for _, p := range c.Params {
		properties[p.Name] = map[string]any{
			"type":        p.Type,
			"description": p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("encode schema: %w", err)
	}
	return b, nil
}

// DecodeConfigs converts storage rows into validated tool configs. A row that
// no longer validates is skipped rather than failing the chat run: the user
// can fix or disable it, and one broken tool must not take the conversation
// down. The returned error names the first skipped row, for logging.
func DecodeConfigs(rows []storage.UserTool) ([]ToolConfig, error) {
	var out []ToolConfig
	var firstErr error
	for _, row := range rows {
		c := ToolConfig{
			Name:            row.Name,
			Description:     row.Description,
			Method:          row.Method,
			URLTemplate:     row.URLTemplate,
			BodyTemplate:    row.BodyTemplate,
			RequireApproval: row.RequireApproval,
		}
		if err := json.Unmarshal(row.Params, &c.Params); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("tool %q: decode params: %w", row.Name, err)
			}
			continue
		}
		if err := json.Unmarshal(row.Headers, &c.Headers); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("tool %q: decode headers: %w", row.Name, err)
			}
			continue
		}
		if err := c.Validate(); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("tool %q: %w", row.Name, err)
			}
			continue
		}
		out = append(out, c)
	}
	return out, firstErr
}

// replacePlaceholders substitutes each {{name}} with subst(name)'s escaped
// rendering; a placeholder subst rejects stays as-is.
func replacePlaceholders(tpl string, subst func(name string) (string, error), esc func(string) string) (string, error) {
	var b strings.Builder
	for _, m := range placeholderRE.FindAllStringSubmatchIndex(tpl, -1) {
		name := tpl[m[2]:m[3]]
		v, err := subst(name)
		if err != nil {
			return "", err
		}
		b.WriteString(tpl[b.Len():m[0]])
		b.WriteString(esc(v))
	}
	b.WriteString(tpl[b.Len():])
	return b.String(), nil
}
