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
