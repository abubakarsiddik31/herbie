package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	ErrInvalidCiphertext = errors.New("vault: invalid ciphertext")
	ErrDecryptionFailed  = errors.New("vault: decryption failed")
)

// Vault handles AES-256-GCM encryption and decryption of secrets at rest.
type Vault struct {
	key [32]byte
}

// New creates a new Vault derived from a master secret.
func New(masterSecret string) *Vault {
	h := sha256.Sum256([]byte(masterSecret))
	return &Vault{key: h}
}

type encryptedEnvelope struct {
	Enc string `json:"_enc"`
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a JSON envelope.
func (v *Vault) Encrypt(plaintext []byte) (json.RawMessage, error) {
	block, err := aes.NewCipher(v.key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Seal appends ciphertext and tag to nonce
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	encStr := base64.StdEncoding.EncodeToString(sealed)

	env := encryptedEnvelope{Enc: encStr}
	return json.Marshal(env)
}

// Decrypt decrypts an envelope created by Encrypt. If the input is not
// an encrypted envelope, it returns the raw bytes unmodified for backwards compatibility.
func (v *Vault) Decrypt(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	var env encryptedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Enc == "" {
		// Not an envelope or malformed: return as plain bytes
		return raw, nil
	}

	data, err := base64.StdEncoding.DecodeString(env.Enc)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode: %v", ErrInvalidCiphertext, err)
	}

	block, err := aes.NewCipher(v.key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

// Common token regex patterns for redaction
var (
	bearerPattern = regexp.MustCompile(`(?i)(bearer\s+)([A-Za-z0-9_\-\.~+/=]{8,})`)
	githubPattern = regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{16,}\b`)
	slackPattern  = regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9\-]{10,}\b`)
	apiKeyPattern = regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{20,}\b`)
	jsonAuthRegex = regexp.MustCompile(`(?i)"(password|secret|token|apiKey|access_token|refresh_token)"\s*:\s*"([^"]+)"`)
)

// RedactSecrets scrubs known secret values and common token patterns from strings.
func RedactSecrets(text string, knownSecrets []string) string {
	if text == "" {
		return text
	}

	res := text

	// Redact known secrets first (longest to shortest)
	for _, s := range knownSecrets {
		s = strings.TrimSpace(s)
		if len(s) >= 4 { // Avoid scrubbing short generic substrings
			res = strings.ReplaceAll(res, s, "[REDACTED_SECRET]")
		}
	}

	// Pattern based scrubbing
	res = bearerPattern.ReplaceAllString(res, "${1}[REDACTED_TOKEN]")
	res = githubPattern.ReplaceAllString(res, "[REDACTED_GITHUB_TOKEN]")
	res = slackPattern.ReplaceAllString(res, "[REDACTED_SLACK_TOKEN]")
	res = apiKeyPattern.ReplaceAllString(res, "[REDACTED_API_KEY]")
	res = jsonAuthRegex.ReplaceAllString(res, `"$1":"[REDACTED]"`)

	return res
}

// SanitizeValue recursively sanitizes strings in arbitrary JSON/Go data structures.
func SanitizeValue(v any, knownSecrets []string) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		return RedactSecrets(val, knownSecrets)
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, item := range val {
			res[k] = SanitizeValue(item, knownSecrets)
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = SanitizeValue(item, knownSecrets)
		}
		return res
	default:
		return v
	}
}
