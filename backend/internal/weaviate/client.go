package weaviate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/google/uuid"
)

const collection = "DocumentChunk"

// Client is a small REST/GraphQL client for the local Weaviate: one
// collection of BYO-vector chunks, hybrid-searched per user. It
// implements rag.VectorStore.
type Client struct {
	base string
	dims int
	http *http.Client
}

func New(baseURL string, dims int, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{base: baseURL, dims: dims, http: hc}
}

// EnsureCollection creates DocumentChunk when missing. Safe to call at
// every startup.
func (c *Client) EnsureCollection(ctx context.Context) error {
	exists, err := c.classExists(ctx)
	if err != nil {
		return fmt.Errorf("weaviate schema: %w", err)
	}
	if exists {
		return nil
	}
	schema := map[string]any{
		"class":      collection,
		"vectorizer": "none",
		"properties": []map[string]any{
			{"name": "user_id", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false},
			{"name": "document_id", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false},
			{"name": "doc_title", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false},
			{"name": "chunk_index", "dataType": []string{"int"}},
			{"name": "content", "dataType": []string{"text"}, "indexSearchable": true},
			{"name": "created_at", "dataType": []string{"text"}, "indexSearchable": false},
		},
	}
	if err := c.post(ctx, "/v1/schema", schema, nil); err != nil {
		return fmt.Errorf("weaviate create collection: %w", err)
	}
	return nil
}

func (c *Client) classExists(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/schema/"+collection, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("GET schema: %s", resp.Status)
	}
}

func chunkID(documentID string, index int) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(documentID+"/"+strconv.Itoa(index))).String()
}

// UpsertChunks writes one document's chunks with their vectors. Call
// DeleteDocument first — re-ingestion must not leave stale vectors.
func (c *Client) UpsertChunks(ctx context.Context, userID, documentID, docTitle string, chunks []rag.Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("weaviate upsert: %d chunks but %d vectors", len(chunks), len(vectors))
	}
	if len(chunks) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	objects := make([]map[string]any, len(chunks))
	for i, ch := range chunks {
		objects[i] = map[string]any{
			"class":  collection,
			"id":     chunkID(documentID, ch.Index),
			"vector": vectors[i],
			"properties": map[string]any{
				"user_id":     userID,
				"document_id": documentID,
				"doc_title":   docTitle,
				"chunk_index": ch.Index,
				"content":     ch.Content,
				"created_at":  now,
			},
		}
	}
	var batchResp struct {
		Objects []struct {
			Result struct {
				Errors struct {
					Error []struct {
						Message string `json:"message"`
					} `json:"error"`
				} `json:"errors"`
			} `json:"result"`
		} `json:"objects"`
	}
	if err := c.post(ctx, "/v1/batch/objects", map[string]any{"objects": objects}, &batchResp); err != nil {
		return fmt.Errorf("weaviate upsert: %w", err)
	}
	for i, o := range batchResp.Objects {
		if len(o.Result.Errors.Error) > 0 {
			return fmt.Errorf("weaviate upsert: object %d: %s", i, o.Result.Errors.Error[0].Message)
		}
	}
	return nil
}

// DeleteDocument removes every vector of one document.
func (c *Client) DeleteDocument(ctx context.Context, documentID string) error {
	body := map[string]any{
		"match": map[string]any{
			"class": collection,
			"where": map[string]any{
				"path":      []string{"document_id"},
				"operator":  "Equal",
				"valueText": documentID,
			},
		},
		"output": "minimal",
	}
	if err := c.post(ctx, "/v1/batch/delete", body, nil); err != nil {
		return fmt.Errorf("weaviate delete: %w", err)
	}
	return nil
}

// HybridSearch runs a BM25+vector hybrid query (alpha 0.5) scoped to
// one user's chunks and returns up to k scored hits.
func (c *Client) HybridSearch(ctx context.Context, userID, query string, k int) ([]rag.Scored, error) {
	gql := fmt.Sprintf(`{Get{%s(hybrid:{query:%s alpha:0.5},where:{path:["user_id"] operator:Equal valueText:%s},limit:%d})
		{content doc_title document_id chunk_index _additional{score}}}}`,
		collection, graphqlQuote(query), graphqlQuote(userID), k)
	var payload struct {
		Data struct {
			Get map[string][]struct {
				Content    string `json:"content"`
				DocTitle   string `json:"doc_title"`
				DocumentID string `json:"document_id"`
				ChunkIndex int    `json:"chunk_index"`
				Additional struct {
					Score string `json:"score"`
				} `json:"_additional"`
			}
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.post(ctx, "/v1/graphql", map[string]any{"query": gql}, &payload); err != nil {
		return nil, fmt.Errorf("weaviate search: %w", err)
	}
	if len(payload.Errors) > 0 {
		return nil, fmt.Errorf("weaviate search: %s", payload.Errors[0].Message)
	}
	rows := payload.Data.Get[collection]
	out := make([]rag.Scored, 0, len(rows))
	for _, row := range rows {
		score, err := strconv.ParseFloat(row.Additional.Score, 64)
		if err != nil {
			score = 0
		}
		out = append(out, rag.Scored{
			Chunk: rag.Chunk{
				DocumentID: row.DocumentID, DocTitle: row.DocTitle,
				Index: row.ChunkIndex, Content: row.Content,
			},
			Score: score,
		})
	}
	return out, nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	buf := &bytes.Buffer{}
	if body != nil {
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("post %s: read body: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		tail := payload
		if len(tail) > 300 {
			tail = tail[:300]
		}
		return fmt.Errorf("post %s: %s: %s", path, resp.Status, tail)
	}
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("post %s: decode: %w", path, err)
		}
	}
	return nil
}

// graphqlQuote renders s as a GraphQL string literal.
func graphqlQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
