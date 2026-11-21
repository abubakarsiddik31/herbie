package storage

import (
	"strings"
	"testing"
)

func TestBuildPrefixTsQuery(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"single", "deploy", "'deploy':*"},
		{"words", "wal keep", "'wal':* & 'keep':*"},
		{"operators stripped", "a & b | (c) !d", "'a':* & 'b':* & 'c':* & 'd':*"},
		{"punctuation", "replication, lag!", "'replication':* & 'lag':*"},
		{"lowercased", "Deploy NOTES", "'deploy':* & 'notes':*"},
		{"empty", "   & | ! ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildPrefixTsQuery(tt.in); got != tt.want {
				t.Fatalf("BuildPrefixTsQuery(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	many := strings.Repeat("word ", maxSearchTerms+5)
	if got := strings.Count(BuildPrefixTsQuery(many), "&"); got != maxSearchTerms-1 {
		t.Fatalf("term cap = %d joins, want %d", got, maxSearchTerms-1)
	}
}
