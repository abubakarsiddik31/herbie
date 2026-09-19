package storage

import (
	"bytes"
	"regexp"
	"strings"
)

var nullJSONEscapePattern = regexp.MustCompile(`(^|[^\\])((?:\\\\)*)\\u0000`)

// SanitizeText removes NUL bytes (0x00) and ensures valid UTF-8.
// PostgreSQL text/varchar columns reject strings containing 0x00 bytes
// with "invalid byte sequence for encoding UTF8: 0x00".
func SanitizeText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.ToValidUTF8(s, "")
}

// SanitizeJSONBytes removes raw 0x00 bytes and unescaped \u0000 sequences
// that PostgreSQL jsonb explicitly rejects with "unsupported Unicode escape sequence \u0000".
func SanitizeJSONBytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	b = bytes.ReplaceAll(b, []byte{0}, nil)
	b = nullJSONEscapePattern.ReplaceAll(b, []byte(`$1$2`))
	return b
}
