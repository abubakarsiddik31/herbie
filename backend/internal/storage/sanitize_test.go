package storage

import (
	"bytes"
	"testing"
)

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "clean string",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "string with embedded null bytes",
			input:    "Hello\x00, \x00world!\x00",
			expected: "Hello, world!",
		},
		{
			name:     "string with only null bytes",
			input:    "\x00\x00\x00",
			expected: "",
		},
		{
			name:     "string with invalid utf8",
			input:    "bad\xff\xfeutf8",
			expected: "badutf8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeText(tc.input)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestSanitizeJSONBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "nil or empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "valid json without nulls",
			input:    []byte(`{"message":"hello"}`),
			expected: []byte(`{"message":"hello"}`),
		},
		{
			name:     "json with unescaped null escape",
			input:    []byte(`{"message":"hello\u0000world"}`),
			expected: []byte(`{"message":"helloworld"}`),
		},
		{
			name:     "json with raw 0x00 bytes",
			input:    []byte("{\"message\":\"hello\x00world\"}"),
			expected: []byte(`{"message":"helloworld"}`),
		},
		{
			name:     "json with escaped backslash before u0000",
			input:    []byte(`{"path":"C:\\u0000"}`),
			expected: []byte(`{"path":"C:\\u0000"}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeJSONBytes(tc.input)
			if !bytes.Equal(got, tc.expected) {
				t.Fatalf("expected %s, got %s", string(tc.expected), string(got))
			}
		})
	}
}
