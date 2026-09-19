package vault

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestVaultEncryptDecrypt(t *testing.T) {
	v := New("super-secret-master-key-12345678")

	original := []byte(`{"token":"ghp_abcdef12345678901234","username":"testuser"}`)
	encrypted, err := v.Encrypt(original)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if strings.Contains(string(encrypted), "ghp_abcdef12345678901234") {
		t.Fatalf("encrypted payload leaked plaintext: %s", string(encrypted))
	}

	decrypted, err := v.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if string(decrypted) != string(original) {
		t.Fatalf("expected %s, got %s", string(original), string(decrypted))
	}
}

func TestVaultBackwardCompatibilityPlaintext(t *testing.T) {
	v := New("super-secret-master-key-12345678")

	plain := json.RawMessage(`{"token":"my-old-token"}`)
	decrypted, err := v.Decrypt(plain)
	if err != nil {
		t.Fatalf("expected no error for plaintext backward compat: %v", err)
	}
	if string(decrypted) != string(plain) {
		t.Fatalf("expected %s, got %s", string(plain), string(decrypted))
	}
}

func TestVaultTamperedCiphertext(t *testing.T) {
	v := New("super-secret-master-key-12345678")
	original := []byte(`{"token":"secret"}`)
	encrypted, err := v.Encrypt(original)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	var env encryptedEnvelope
	if err := json.Unmarshal(encrypted, &env); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	data, _ := base64.StdEncoding.DecodeString(env.Enc)
	data[len(data)-1] ^= 0xFF // Flip bits in GCM tag
	env.Enc = base64.StdEncoding.EncodeToString(data)
	tampered, _ := json.Marshal(env)

	_, err = v.Decrypt(tampered)
	if err == nil {
		t.Fatal("expected error on tampered ciphertext, got nil")
	}
}

func TestRedactSecrets(t *testing.T) {
	secrets := []string{"super_private_token_xyz"}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "known secret",
			input:    "Error: failed connecting with super_private_token_xyz to server",
			expected: "Error: failed connecting with [REDACTED_SECRET] to server",
		},
		{
			name:     "bearer token",
			input:    "Authorization: Bearer mySecretToken123456",
			expected: "Authorization: Bearer [REDACTED_TOKEN]",
		},
		{
			name:     "github token pattern",
			input:    "Found token ghp_1234567890abcdef1234 in response",
			expected: "Found token [REDACTED_GITHUB_TOKEN] in response",
		},
		{
			name:     "slack token pattern",
			input:    "Using xoxb-1234567890-abcdefghij",
			expected: "Using [REDACTED_SLACK_TOKEN]",
		},
		{
			name:     "openai api key pattern",
			input:    "API key: sk-abcdefghijklmnopqrstuvwxyz123456",
			expected: "API key: [REDACTED_API_KEY]",
		},
		{
			name:     "json secret field",
			input:    `{"status":"ok","token":"my-super-secret-token"}`,
			expected: `{"status":"ok","token":"[REDACTED]"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := RedactSecrets(tc.input, secrets)
			if actual != tc.expected {
				t.Fatalf("expected:\n%q\ngot:\n%q", tc.expected, actual)
			}
		})
	}
}

func TestSanitizeValue(t *testing.T) {
	secrets := []string{"super_secret_val"}
	data := map[string]any{
		"message": "Hello super_secret_val",
		"auth":    "Bearer 12345678abcdef",
		"nested": []any{
			"sk-123456789012345678901234",
			123,
		},
	}

	sanitized := SanitizeValue(data, secrets).(map[string]any)
	if sanitized["message"] != "Hello [REDACTED_SECRET]" {
		t.Errorf("expected redacted message, got %v", sanitized["message"])
	}
	if sanitized["auth"] != "Bearer [REDACTED_TOKEN]" {
		t.Errorf("expected redacted auth, got %v", sanitized["auth"])
	}
	nestedList := sanitized["nested"].([]any)
	if nestedList[0] != "[REDACTED_API_KEY]" {
		t.Errorf("expected redacted api key, got %v", nestedList[0])
	}
	if nestedList[1] != 123 {
		t.Errorf("expected number intact, got %v", nestedList[1])
	}
}
