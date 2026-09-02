package cost

import "math"

// Rates holds USD prices per 1M tokens. Values come from config; the
// ledger stores raw tokens so historic rows reprice if rates change.
type Rates struct {
	ChatInputPerM  float64
	ChatOutputPerM float64
}

// ChatCostMicros returns the cost of one chat run in integer micro-USD.
// Per token, one micro-USD equals the rate itself (USD/1M tokens).
func (r Rates) ChatCostMicros(inputTokens, outputTokens int) int64 {
	return int64(math.Round(float64(inputTokens)*r.ChatInputPerM + float64(outputTokens)*r.ChatOutputPerM))
}

// Table prices models individually, falling back to the global rates for
// any model without its own entry.
type Table struct {
	Default Rates
	ByModel map[string]Rates
}

// RatesFor returns the rates for one model ID.
func (t Table) RatesFor(model string) Rates {
	if r, ok := t.ByModel[model]; ok {
		return r
	}
	return t.Default
}

// ChatCostMicrosFor prices one run of the named model.
func (t Table) ChatCostMicrosFor(model string, inputTokens, outputTokens int) int64 {
	return t.RatesFor(model).ChatCostMicros(inputTokens, outputTokens)
}
