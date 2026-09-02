package chat

// ProviderKeys carries the server-side provider credentials the registry
// needs to build clients. Empty key = provider not configured = its models
// are absent from the user-facing catalog.
type ProviderKeys struct {
	Gemini           string
	OpenAI           string
	Anthropic        string
	GeminiBaseURL    string // optional proxy / test override
	OpenAIBaseURL    string
	AnthropicBaseURL string
}

// ModelSpec is one selectable chat model: its wire name, the provider that
// serves it, a UI label, and the estimated USD rate per 1M tokens used by
// the cost ledger (which stores raw tokens, so rows reprice if rates move).
type ModelSpec struct {
	ID                    string
	Provider              string // "gemini" | "openai" | "anthropic"
	Label                 string
	InputPerM, OutputPerM float64
}

// catalog is the server's curated model list, in dropdown order. The default
// model is the first entry. Adding a row here is all it takes to expose a
// new model, provided the provider key is configured.
var catalog = []ModelSpec{
	{ID: "gemini-2.5-flash", Provider: "gemini", Label: "Gemini 2.5 Flash", InputPerM: 0.30, OutputPerM: 2.50},
	{ID: "gemini-2.5-pro", Provider: "gemini", Label: "Gemini 2.5 Pro", InputPerM: 1.25, OutputPerM: 10.00},
	{ID: "gpt-5", Provider: "openai", Label: "GPT-5", InputPerM: 1.25, OutputPerM: 10.00},
	{ID: "gpt-4.1", Provider: "openai", Label: "GPT-4.1", InputPerM: 2.00, OutputPerM: 8.00},
	{ID: "claude-sonnet-4-5", Provider: "anthropic", Label: "Claude Sonnet 4.5", InputPerM: 3.00, OutputPerM: 15.00},
	{ID: "claude-haiku-4-5", Provider: "anthropic", Label: "Claude Haiku 4.5", InputPerM: 1.00, OutputPerM: 5.00},
}

// Catalog returns the full curated model list in dropdown order.
func Catalog() []ModelSpec {
	out := make([]ModelSpec, len(catalog))
	copy(out, catalog)
	return out
}

// FindModel looks up one model by wire ID.
func FindModel(id string) (ModelSpec, bool) {
	for _, m := range catalog {
		if m.ID == id {
			return m, true
		}
	}
	return ModelSpec{}, false
}

// Available filters the catalog to providers with a configured key,
// preserving dropdown order.
func Available(keys ProviderKeys) []ModelSpec {
	var out []ModelSpec
	for _, m := range catalog {
		if providerKey(keys, m.Provider) != "" {
			out = append(out, m)
		}
	}
	return out
}

// DefaultModel is the model new conversations use when none is set: the
// first catalog entry whose provider is configured.
func DefaultModel(keys ProviderKeys) ModelSpec {
	avail := Available(keys)
	if len(avail) == 0 {
		return ModelSpec{}
	}
	return avail[0]
}

func providerKey(keys ProviderKeys, provider string) string {
	switch provider {
	case "gemini":
		return keys.Gemini
	case "openai":
		return keys.OpenAI
	case "anthropic":
		return keys.Anthropic
	}
	return ""
}
