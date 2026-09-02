package cost

import "testing"

func TestChatCostMicros(t *testing.T) {
	r := Rates{ChatInputPerM: 0.30, ChatOutputPerM: 2.50}
	tests := []struct {
		name string
		in   int
		out  int
		want int64
	}{
		{"zero", 0, 0, 0},
		{"one million in", 1_000_000, 0, 300_000},    // $0.30
		{"one million out", 0, 1_000_000, 2_500_000}, // $2.50
		{"mixed", 1000, 500, 300 + 1250},             // 1000*0.3 + 500*2.5 micros
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.ChatCostMicros(tt.in, tt.out); got != tt.want {
				t.Fatalf("ChatCostMicros(%d,%d) = %d, want %d", tt.in, tt.out, got, tt.want)
			}
		})
	}
}

func TestTableChatCostMicrosFor(t *testing.T) {
	table := Table{
		Default: Rates{ChatInputPerM: 1, ChatOutputPerM: 2},
		ByModel: map[string]Rates{"gpt-5": {ChatInputPerM: 3, ChatOutputPerM: 4}},
	}
	if got := table.ChatCostMicrosFor("gpt-5", 1_000_000, 0); got != 3_000_000 {
		t.Errorf("known model rate = %d, want 3_000_000", got)
	}
	if got := table.ChatCostMicrosFor("unknown-model", 1_000_000, 0); got != 1_000_000 {
		t.Errorf("unknown model falls back to default = %d, want 1_000_000", got)
	}
	// Zero-value Table behaves like a single global rate of zero.
	var zero Table
	if got := zero.ChatCostMicrosFor("gpt-5", 1000, 1000); got != 0 {
		t.Errorf("zero table = %d, want 0", got)
	}
}
