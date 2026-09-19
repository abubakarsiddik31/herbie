package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/embedding"
	gemini "github.com/abubakarsiddik31/golem/providers/gemini"
)

// fakeGemini answers both embed endpoints with one dims-wide vector per
// requested text and a promptTokenCount equal to the request count, and
// records each batch's size in order.
func fakeGemini(t *testing.T, dims int, batches *[]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ":batchEmbedContents") {
			var single struct {
				Content struct {
					Parts []struct{ Text string } `json:"parts"`
				} `json:"content"`
			}
			_ = json.NewDecoder(r.Body).Decode(&single)
			*batches = append(*batches, 1)
			writeJSON(w, map[string]any{
				"embedding":     map[string]any{"values": vec(dims)},
				"usageMetadata": map[string]any{"promptTokenCount": 1},
			})
			return
		}
		var batch struct {
			Requests []struct {
				Content struct {
					Parts []struct{ Text string } `json:"parts"`
				} `json:"content"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Errorf("decode batch: %v", err)
		}
		*batches = append(*batches, len(batch.Requests))
		embeddings := make([]map[string]any, 0, len(batch.Requests))
		for range batch.Requests {
			embeddings = append(embeddings, map[string]any{"values": vec(dims)})
		}
		writeJSON(w, map[string]any{
			"embeddings":    embeddings,
			"usageMetadata": map[string]any{"promptTokenCount": len(batch.Requests)},
		})
	}))
}

func vec(dims int) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = 0.5
	}
	return v
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func newEmbedderAt(baseURL string, batch int) (*Embedder, error) {
	inner, err := gemini.NewEmbedder(gemini.EmbedderConfig{
		APIKey: "test-key", Model: "gemini-embedding-001", Dimensions: 4,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, err
	}
	return embedderWith(inner, batch), nil
}

func TestEmbedDocumentsBatches(t *testing.T) {
	var batches []int
	srv := fakeGemini(t, 4, &batches)
	defer srv.Close()

	e, err := newEmbedderAt(srv.URL, 2)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.EmbedDocuments(context.Background(), []string{"a", "b", "c", "d", "e"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Vectors) != 5 || len(res.Vectors[0]) != 4 {
		t.Fatalf("vectors: %d groups, width %d", len(res.Vectors), len(res.Vectors[0]))
	}
	if !reflect.DeepEqual(batches, []int{2, 2, 1}) {
		t.Fatalf("batch sizes: %v", batches)
	}
	if res.Usage.InputTokens != 5 {
		t.Fatalf("usage not summed: %+v", res.Usage)
	}
}

func TestEmbedQuerySingle(t *testing.T) {
	var batches []int
	srv := fakeGemini(t, 4, &batches)
	defer srv.Close()

	e, err := newEmbedderAt(srv.URL, 96)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.EmbedQuery(context.Background(), "what is in my notes?")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Vectors) != 1 || len(res.Vectors[0]) != 4 {
		t.Fatalf("query vectors: %v", res.Vectors)
	}
	if !reflect.DeepEqual(batches, []int{1}) {
		t.Fatalf("calls: %v", batches)
	}
}

func TestEmbedQueryCache(t *testing.T) {
	var batches []int
	srv := fakeGemini(t, 4, &batches)
	defer srv.Close()

	e, err := newEmbedderAt(srv.URL, 96)
	if err != nil {
		t.Fatal(err)
	}
	// First call hits the mock server
	res1, err := e.EmbedQuery(context.Background(), "repeated query")
	if err != nil {
		t.Fatal(err)
	}
	// Second call with same text should hit cache
	res2, err := e.EmbedQuery(context.Background(), "  repeated query  ")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res1.Vectors, res2.Vectors) {
		t.Fatalf("vectors should match: %v vs %v", res1.Vectors, res2.Vectors)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 HTTP call due to cache, got %d", len(batches))
	}
}

func TestEmbedDocumentsEmpty(t *testing.T) {
	srv := fakeGemini(t, 4, &[]int{})
	defer srv.Close()
	e, err := newEmbedderAt(srv.URL, 96)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.EmbedDocuments(context.Background(), nil); err == nil {
		t.Fatal("want error for empty batch")
	}
}

func TestEmbedHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	e, err := newEmbedderAt(srv.URL, 96)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.EmbedDocuments(context.Background(), []string{"a"})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want 429 surfaced, got %v", err)
	}
}

// Compile-time: the wrapper satisfies the golem port too.
var _ embedding.Embedder = (*Embedder)(nil)
