package chat

import "testing"

func TestCatalogOrderedAndComplete(t *testing.T) {
	got := Catalog()
	want := []string{"gemini-2.5-flash", "gemini-2.5-pro", "gpt-5", "gpt-4.1", "claude-sonnet-4-5", "claude-haiku-4-5"}
	if len(got) != len(want) {
		t.Fatalf("Catalog() has %d entries, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("Catalog()[%d].ID = %q, want %q", i, got[i].ID, id)
		}
		if got[i].Provider == "" || got[i].Label == "" {
			t.Errorf("Catalog()[%d] (%s) missing provider or label", i, id)
		}
		if got[i].InputPerM <= 0 || got[i].OutputPerM <= 0 {
			t.Errorf("Catalog()[%d] (%s) has non-positive rates: %v/%v", i, id, got[i].InputPerM, got[i].OutputPerM)
		}
	}
}

func TestFindModel(t *testing.T) {
	if m, ok := FindModel("gpt-5"); !ok || m.Provider != "openai" {
		t.Errorf("FindModel(gpt-5) = (%+v, %v), want openai entry found", m, ok)
	}
	if m, ok := FindModel("nope"); ok || m.ID != "" {
		t.Errorf("FindModel(nope) = (%+v, %v), want zero, false", m, ok)
	}
}

func TestAvailableFiltersByProviderKey(t *testing.T) {
	keys := ProviderKeys{Gemini: "g"}
	got := Available(keys)
	if len(got) != 2 || got[0].ID != "gemini-2.5-flash" || got[1].ID != "gemini-2.5-pro" {
		t.Errorf("Available(gemini-only) = %v, want the two gemini entries", ids(got))
	}

	keys = ProviderKeys{Anthropic: "a"}
	got = Available(keys)
	if len(got) != 2 || got[0].ID != "claude-sonnet-4-5" {
		t.Errorf("Available(anthropic-only) = %v, want the two anthropic entries", ids(got))
	}

	keys = ProviderKeys{Gemini: "g", OpenAI: "o", Anthropic: "a"}
	if got := Available(keys); len(got) != 6 {
		t.Errorf("Available(all keys) = %v, want all 6", ids(got))
	}

	if got := Available(ProviderKeys{}); len(got) != 0 {
		t.Errorf("Available(no keys) = %v, want empty", ids(got))
	}
}

func TestDefaultModel(t *testing.T) {
	if m := DefaultModel(ProviderKeys{Gemini: "g"}); m.ID != "gemini-2.5-flash" {
		t.Errorf("DefaultModel(gemini) = %q, want gemini-2.5-flash", m.ID)
	}
	if m := DefaultModel(ProviderKeys{Anthropic: "a"}); m.ID != "claude-sonnet-4-5" {
		t.Errorf("DefaultModel(anthropic) = %q, want first available (claude-sonnet-4-5)", m.ID)
	}
	if m := DefaultModel(ProviderKeys{}); m.ID != "" {
		t.Errorf("DefaultModel(no keys) = %q, want empty", m.ID)
	}
}

func ids(specs []ModelSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.ID
	}
	return out
}
