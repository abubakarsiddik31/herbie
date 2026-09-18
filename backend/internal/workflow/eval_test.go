package workflow

import (
	"reflect"
	"testing"
)

func TestResolveString(t *testing.T) {
	ctx := &EvalContext{
		JSON: map[string]any{
			"user": map[string]any{
				"name": "Alice",
				"age":  30,
				"tags": []any{"admin", "dev"},
			},
			"score": 98.5,
		},
		Nodes: map[string]any{
			"node_1": map[string]any{
				"status": "active",
			},
			"Fetch Weather": map[string]any{
				"temp": 24,
				"city": "London",
			},
		},
		Env: map[string]string{
			"ENV": "production",
		},
		Credentials: map[string]map[string]any{
			"slack": {
				"token": "xoxb-secret-token",
			},
		},
	}

	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{
			name:     "exact typed object access",
			input:    "{{ $json.user.age }}",
			expected: 30,
		},
		{
			name:     "array index access",
			input:    "{{ $json.user.tags[0] }}",
			expected: "admin",
		},
		{
			name:     "named node access with bracket",
			input:    `{{ $nodes["Fetch Weather"].city }}`,
			expected: "London",
		},
		{
			name:     "node access by id",
			input:    "{{ $nodes.node_1.status }}",
			expected: "active",
		},
		{
			name:     "credential access",
			input:    "{{ $credentials.slack.token }}",
			expected: "xoxb-secret-token",
		},
		{
			name:     "env access",
			input:    "{{ $env.ENV }}",
			expected: "production",
		},
		{
			name:     "interpolated string",
			input:    "Hello {{ $json.user.name }}, temp in {{ $nodes['Fetch Weather'].city }} is {{ $nodes['Fetch Weather'].temp }}C!",
			expected: "Hello Alice, temp in London is 24C!",
		},
		{
			name:     "missing field evaluates to nil/empty",
			input:    "Missing {{ $json.not_existing }} here",
			expected: "Missing  here",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveString(tc.input, ctx)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("expected %v (%T), got %v (%T)", tc.expected, tc.expected, got, got)
			}
		})
	}
}

func TestResolveAny(t *testing.T) {
	ctx := &EvalContext{
		JSON: map[string]any{
			"url": "https://api.github.com/repos/owner/repo",
			"id":  123,
		},
	}

	input := map[string]any{
		"endpoint": "{{ $json.url }}",
		"id":       "{{ $json.id }}",
		"nested": map[string]any{
			"text": "Item #{{ $json.id }}",
		},
		"list": []any{"{{ $json.id }}", "static"},
	}

	res, ok := ResolveAny(input, ctx).(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any")
	}

	if res["endpoint"] != "https://api.github.com/repos/owner/repo" {
		t.Errorf("unexpected endpoint: %v", res["endpoint"])
	}
	if res["id"] != 123 {
		t.Errorf("unexpected id: %v", res["id"])
	}
	nested := res["nested"].(map[string]any)
	if nested["text"] != "Item #123" {
		t.Errorf("unexpected nested: %v", nested["text"])
	}
	list := res["list"].([]any)
	if list[0] != 123 || list[1] != "static" {
		t.Errorf("unexpected list: %v", list)
	}
}
