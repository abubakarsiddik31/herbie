package weaviate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{base: baseURL, dims: dims, http: hc}
}

// EnsureCollection creates DocumentChunk when missing and adds any
// missing retrieval properties to an existing collection. Additive and
// idempotent: never drops or recreates. Safe to call at every startup.
func (c *Client) EnsureCollection(ctx context.Context) error {
	have, exists, err := c.existingProperties(ctx)
	if err != nil {
		return fmt.Errorf("weaviate schema: %w", err)
	}
	if !exists {
		schema := map[string]any{
			"class":      collection,
			"vectorizer": "none",
			"properties": []map[string]any{
				{"name": "user_id", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false},
				{"name": "document_id", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false},
				{"name": "doc_title", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": false},
				{"name": "chunk_index", "dataType": []string{"int"}},
				{"name": "content", "dataType": []string{"text"}, "indexSearchable": true},
				{"name": "heading", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": true},
				{"name": "page", "dataType": []string{"int"}, "indexFilterable": true, "indexSearchable": false},
				{"name": "token_count", "dataType": []string{"int"}, "indexFilterable": true, "indexSearchable": false},
				{"name": "created_at", "dataType": []string{"text"}, "indexSearchable": false},
			},
		}
		if err := c.post(ctx, "/v1/schema", schema, nil); err != nil {
			return fmt.Errorf("weaviate create collection: %w", err)
		}
		return nil
	}
	if have == nil {
		// Empty-body probe answered 200 with nothing to decode: assume
		// the collection is complete and add nothing.
		return nil
	}
	for _, prop := range []map[string]any{
		{"name": "heading", "dataType": []string{"text"}, "indexFilterable": true, "indexSearchable": true},
		{"name": "page", "dataType": []string{"int"}, "indexFilterable": true, "indexSearchable": false},
		{"name": "token_count", "dataType": []string{"int"}, "indexFilterable": true, "indexSearchable": false},
	} {
		if have[prop["name"].(string)] {
			continue
		}
		if err := c.post(ctx, "/v1/schema/"+collection+"/properties", prop, nil); err != nil {
			return fmt.Errorf("weaviate add property: %w", err)
		}
	}
	return nil
}

// existingProperties probes the collection schema. It returns the set of
// known property names when the class exists, exists=false on 404, and a
// nil set with exists=true when the probe answered 200 with an empty
// body (nothing to infer — the caller adds nothing).
func (c *Client) existingProperties(ctx context.Context) (map[string]bool, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/schema/"+collection, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, nil
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, false, fmt.Errorf("GET schema: %s", resp.Status)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, true, nil
	}
	var schema struct {
		Properties []struct {
			Name string `json:"name"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &schema); err != nil {
		return nil, false, fmt.Errorf("decode schema: %w", err)
	}
	have := map[string]bool{}
	for _, p := range schema.Properties {
		have[p.Name] = true
	}
	return have, true, nil
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
				"heading":     ch.Heading,
				"page":        ch.Page,
				"token_count": ch.Tokens,
				"created_at":  now,
			},
		}
	}
	// Weaviate 1.28 answers a bare array of per-object results (no
	// wrapper object).
	var batchResp []struct {
		Result struct {
			Errors struct {
				Error []struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"errors"`
		} `json:"result"`
	}
	if err := c.post(ctx, "/v1/batch/objects", map[string]any{"objects": objects}, &batchResp); err != nil {
		return fmt.Errorf("weaviate upsert: %w", err)
	}
	for i, o := range batchResp {
		if len(o.Result.Errors.Error) > 0 {
			return fmt.Errorf("weaviate upsert: object %d: %s", i, o.Result.Errors.Error[0].Message)
		}
	}
	return nil
}

// DeleteDocument removes every vector of one document. Weaviate 1.28
// dropped the batch-delete REST route, so the ids come back from a
// filtered GraphQL query and each object is deleted individually.
func (c *Client) DeleteDocument(ctx context.Context, documentID string) error {
	ids, err := c.documentChunkIDs(ctx, documentID)
	if err != nil {
		return fmt.Errorf("weaviate delete: %w", err)
	}
	for _, id := range ids {
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			c.base+"/v1/objects/"+collection+"/"+id, nil)
		if err != nil {
			return fmt.Errorf("weaviate delete: %w", err)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("weaviate delete: %w", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		// 204 deleted; 404 already gone — both fine.
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("weaviate delete: object %s: %s", id, resp.Status)
		}
	}
	return nil
}

func (c *Client) documentChunkIDs(ctx context.Context, documentID string) ([]string, error) {
	gql := fmt.Sprintf(`{Get{%s(where:{path:["document_id"] operator:Equal valueText:%s},limit:10000){_additional{id}}}}`,
		collection, graphqlQuote(documentID))
	var payload struct {
		Data struct {
			Get map[string][]struct {
				Additional struct {
					ID string `json:"id"`
				} `json:"_additional"`
			}
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.post(ctx, "/v1/graphql", map[string]any{"query": gql}, &payload); err != nil {
		return nil, err
	}
	if len(payload.Errors) > 0 {
		return nil, fmt.Errorf("%s", payload.Errors[0].Message)
	}
	rows := payload.Data.Get[collection]
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Additional.ID)
	}
	return ids, nil
}

// HybridSearch runs a BM25+vector hybrid query scoped to one user's
// chunks and returns up to limit scored hits. Alpha blends BM25 and
// vector search; an empty docIDs filters nothing (all user documents).
// The query vector rides in the request: a vectorizer:none collection
// cannot vectorize the text server-side.
func (c *Client) HybridSearch(ctx context.Context, userID, query string, queryVector []float32, alpha float64, limit int, docIDs []string) ([]rag.Scored, error) {
	where := fmt.Sprintf(`where:{path:["user_id"] operator:Equal valueText:%s}`, graphqlQuote(userID))
	if len(docIDs) > 0 {
		ors := make([]string, len(docIDs))
		for i, d := range docIDs {
			ors[i] = fmt.Sprintf(`{path:["document_id"] operator:Equal valueText:%s}`, graphqlQuote(d))
		}
		where = fmt.Sprintf(`where:{operator:And operands:[{path:["user_id"] operator:Equal valueText:%s},{operator:Or operands:[%s]}]}`,
			graphqlQuote(userID), strings.Join(ors, ","))
	}
	gql := fmt.Sprintf(`{Get{%s(hybrid:{query:%s vector:%s alpha:%s fusionType: rankedFusion},%s,limit:%d){content doc_title document_id chunk_index heading page token_count _additional{score}}}}`,
		collection, graphqlQuote(query), graphqlVector(queryVector), strconv.FormatFloat(alpha, 'f', -1, 64), where, limit)
	var payload struct {
		Data struct {
			Get map[string][]struct {
				Content    string `json:"content"`
				DocTitle   string `json:"doc_title"`
				DocumentID string `json:"document_id"`
				ChunkIndex int    `json:"chunk_index"`
				Heading    string `json:"heading"`
				Page       int    `json:"page"`
				Tokens     int    `json:"token_count"`
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
				Heading: row.Heading, Page: row.Page, Tokens: row.Tokens,
			},
			Score: score,
		})
	}
	return out, nil
}

// ExpandRange reads one document's chunks in the inclusive index range.
// Ranges are small by construction, so no limit override is applied.
// The caller sorts; ordering here carries no contract.
func (c *Client) ExpandRange(ctx context.Context, userID, docID string, lo, hi int) ([]rag.Chunk, error) {
	gql := fmt.Sprintf(`{Get{%s(where:{operator:And operands:[{path:["user_id"] operator:Equal valueText:%s},{path:["document_id"] operator:Equal valueText:%s},{path:["chunk_index"] operator:GreaterThanEqual valueInt:%d},{path:["chunk_index"] operator:LessThanEqual valueInt:%d}]}){content doc_title document_id chunk_index heading page token_count}}}`,
		collection, graphqlQuote(userID), graphqlQuote(docID), lo, hi)
	var payload struct {
		Data struct {
			Get map[string][]struct {
				Content    string `json:"content"`
				DocTitle   string `json:"doc_title"`
				DocumentID string `json:"document_id"`
				ChunkIndex int    `json:"chunk_index"`
				Heading    string `json:"heading"`
				Page       int    `json:"page"`
				Tokens     int    `json:"token_count"`
			}
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.post(ctx, "/v1/graphql", map[string]any{"query": gql}, &payload); err != nil {
		return nil, fmt.Errorf("weaviate expand: %w", err)
	}
	if len(payload.Errors) > 0 {
		return nil, fmt.Errorf("weaviate expand: %s", payload.Errors[0].Message)
	}
	rows := payload.Data.Get[collection]
	out := make([]rag.Chunk, 0, len(rows))
	for _, row := range rows {
		out = append(out, rag.Chunk{
			DocumentID: row.DocumentID, DocTitle: row.DocTitle,
			Index: row.ChunkIndex, Content: row.Content,
			Heading: row.Heading, Page: row.Page, Tokens: row.Tokens,
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

// graphqlVector renders a vector as a GraphQL list of floats.
func graphqlVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
