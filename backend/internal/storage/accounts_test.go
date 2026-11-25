package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactJSONRemovesSecretFieldsRecursively(t *testing.T) {
	raw := []byte(`{
		"headers":{"Authorization":"Bearer topsecret"},
		"webhook_secret":"whsec",
		"nested":{"password":"pw","keep":"v","tokens":"counting?"},
		"inputTokens":5,
		"input_tokens":7
	}`)
	out, err := redactJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"headers", "Authorization", "topsecret", "webhook_secret", "whsec", "password", `"pw"`, `"tokens"`} {
		if strings.Contains(string(out), forbidden) {
			t.Fatalf("redaction left %q in %s", forbidden, out)
		}
	}
	for _, kept := range []string{`"keep":"v"`, `"inputTokens":5`, `"input_tokens":7`} {
		if !strings.Contains(string(out), kept) {
			t.Fatalf("redaction removed %q from %s", kept, out)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
}
