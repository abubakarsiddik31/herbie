package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func validConfig() ToolConfig {
	return ToolConfig{
		Name:        "get_weather",
		Description: "Get current weather for coordinates",
		Method:      "GET",
		URLTemplate: "https://api.open-meteo.com/v1/forecast",
		Params: []ParamDef{
			{Name: "latitude", In: "query", Type: "number", Required: true, Description: "latitude"},
		},
	}
}

func TestValidateAcceptsMinimal(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidateRejections(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ToolConfig)
	}{
		{"bad name", func(c *ToolConfig) { c.Name = "Get-Weather" }},
		{"empty name", func(c *ToolConfig) { c.Name = "" }},
		{"empty description", func(c *ToolConfig) { c.Description = "" }},
		{"long description", func(c *ToolConfig) { c.Description = strings.Repeat("x", 2001) }},
		{"bad method", func(c *ToolConfig) { c.Method = "BREW" }},
		{"relative url", func(c *ToolConfig) { c.URLTemplate = "/api/forecast" }},
		{"non-http scheme", func(c *ToolConfig) { c.URLTemplate = "ftp://example.com/x" }},
		{"unknown placeholder", func(c *ToolConfig) { c.URLTemplate = "https://api.example.com/{{lat}}" }},
		{"too many params", func(c *ToolConfig) {
			c.Params = nil
			for i := 0; i < 17; i++ {
				c.Params = append(c.Params, ParamDef{Name: string(rune('a' + i)), In: "query", Type: "string"})
			}
		}},
		{"duplicate param", func(c *ToolConfig) {
			c.Params = append(c.Params, ParamDef{Name: "latitude", In: "query", Type: "number"})
		}},
		{"bad param name", func(c *ToolConfig) {
			c.Params[0].Name = "9lat"
		}},
		{"bad param location", func(c *ToolConfig) {
			c.Params[0].In = "header"
		}},
		{"bad param type", func(c *ToolConfig) {
			c.Params[0].Type = "integer"
		}},
		{"body on GET", func(c *ToolConfig) { c.BodyTemplate = `{"x":1}` }},
		{"body unknown placeholder", func(c *ToolConfig) {
			c.Method = "POST"
			c.BodyTemplate = `{"lat": {{lon}}}`
		}},
		{"body invalid json", func(c *ToolConfig) {
			c.Method = "POST"
			c.BodyTemplate = `{"lat": {{latitude}},}`
		}},
		{"too many headers", func(c *ToolConfig) {
			c.Headers = map[string]string{}
			for i := 0; i < 17; i++ {
				c.Headers[string(rune('a'+i))] = "v"
			}
		}},
	}
	for _, tc := range cases {
		cfg := validConfig()
		tc.mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: expected rejection, got nil", tc.name)
		}
	}
}

func TestValidateAcceptsPostWithBody(t *testing.T) {
	cfg := validConfig()
	cfg.Method = "POST"
	cfg.BodyTemplate = `{"latitude": {{latitude}}, "label": "x"}`
	if err := cfg.Validate(); err != nil {
		t.Fatalf("POST with body rejected: %v", err)
	}
}

func TestSchema(t *testing.T) {
	cfg := validConfig()
	cfg.Params = []ParamDef{
		{Name: "latitude", In: "query", Type: "number", Required: true, Description: "latitude"},
		{Name: "unit", In: "query", Type: "string", Description: "unit"},
	}
	schema, err := cfg.Schema()
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(schema, &got); err != nil {
		t.Fatalf("schema not json: %v", err)
	}
	if got["type"] != "object" || got["additionalProperties"] != false {
		t.Fatalf("schema envelope wrong: %v", got)
	}
	required, ok := got["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "latitude" {
		t.Fatalf("required wrong: %v", got["required"])
	}
	props, ok := got["properties"].(map[string]any)
	if !ok || props["latitude"].(map[string]any)["type"] != "number" || props["unit"].(map[string]any)["type"] != "string" {
		t.Fatalf("properties wrong: %v", props)
	}
}
