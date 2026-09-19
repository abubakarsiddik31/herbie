package rag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
)

// Scored pairs a chunk with its retrieval score (hybrid BM25+vector).
// Context optionally carries the expanded neighbor window; empty means
// the core chunk content stands alone.
type Scored struct {
	Chunk   Chunk
	Score   float64
	Context string
}

// VectorStore is the retrieval index contract. internal/weaviate.Client
// implements it; tests use fakes.
type VectorStore interface {
	UpsertChunks(ctx context.Context, userID, documentID, docTitle string, chunks []Chunk, vectors [][]float32) error
	DeleteDocument(ctx context.Context, documentID string) error
	HybridSearch(ctx context.Context, userID, query string, queryVector []float32, alpha float64, limit int, docIDs []string) ([]Scored, error)
	ExpandRange(ctx context.Context, userID, docID string, lo, hi int) ([]Chunk, error)
}

// ObjectUploader is the slice of the object store the pipeline needs:
// keep the original upload, drop it with the document.
type ObjectUploader interface {
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
	Delete(ctx context.Context, key string) error
}

const (
	defaultK    = 5
	maxK        = 20
	maxKeyBytes = 9000 // sane cap so odd filenames cannot break the store
)

// SearchOptions scopes one retrieval call. DocIDs empty = all documents.
type SearchOptions struct {
	TopK   int
	DocIDs []string
}

// SearchConfig tunes retrieval. Alpha blends BM25/vector in Weaviate;
// candidates = min(max(TopK*CandidateMult, TopK), MaxCandidates).
// ExpandBefore/After widen each hit with neighbor chunks for context.
type SearchConfig struct {
	Alpha         float64
	CandidateMult int
	MaxCandidates int
	ExpandBefore  int
	ExpandAfter   int
}

// UsageReport carries everything the ledger needs: the exact query-embed
// tokens plus the optional rerank generation usage.
type UsageReport struct {
	Embed       EmbedUsage
	Reranked    bool
	RerankModel string
	RerankIn    int
	RerankOut   int
}

// EmbedUsage is the embedding ledger entry for one call: exact provider
// tokens when the API reports them, an estimate flagged Estimated when it
// does not (Gemini's embeddings endpoints return no usage metadata).
type EmbedUsage struct {
	InputTokens int
	Estimated   bool
}

// Service is the RAG vertical: ingest (extract → chunk → embed → index)
// and search (embed query → hybrid search → expand → rerank). It owns no
// storage rows; the HTTP layer tracks document state in Postgres around
// Ingest calls.
type Service struct {
	embed   *Embedder
	vs      VectorStore
	objects ObjectUploader

	ranker       *Ranker
	searchCfg    SearchConfig
	chunkTarget  int
	chunkOverlap int
}

func NewService(embed *Embedder, vs VectorStore, objects ObjectUploader) *Service {
	return &Service{
		embed:        embed,
		vs:           vs,
		objects:      objects,
		searchCfg:    SearchConfig{Alpha: 0.5, CandidateMult: 4, MaxCandidates: 40, ExpandBefore: 1, ExpandAfter: 1},
		chunkTarget:  512,
		chunkOverlap: 64,
	}
}

// WithRanker attaches the listwise reranker used by Search. Nil disables
// reranking (hybrid order is trimmed to TopK).
func (s *Service) WithRanker(r *Ranker) *Service {
	s.ranker = r
	return s
}

// WithTuning overrides the retrieval, expansion, and chunk budgets.
// Negative values clamp to 0 at use.
func (s *Service) WithTuning(alpha float64, mult, maxCand, before, after, chunkTarget, chunkOverlap int) *Service {
	s.searchCfg = SearchConfig{
		Alpha:         alpha,
		CandidateMult: mult,
		MaxCandidates: maxCand,
		ExpandBefore:  before,
		ExpandAfter:   after,
	}
	s.chunkTarget = chunkTarget
	s.chunkOverlap = chunkOverlap
	return s
}

// Ingest keeps the original bytes in the object store at the canonical key,
// then indexes the extracted text. Idempotent per document: stale vectors are deleted
// before the fresh upsert. Returns the chunk count.
func (s *Service) Ingest(ctx context.Context, userID, documentID, docTitle, mime string, content []byte, contentType string) (int, EmbedUsage, error) {
	return s.IngestWithKey(ctx, ObjectKey(userID, documentID, docTitle), userID, documentID, docTitle, mime, content, contentType)
}

// IngestWithKey allows specifying a custom object storage key (e.g. project-scoped folder key).
func (s *Service) IngestWithKey(ctx context.Context, key, userID, documentID, docTitle, mime string, content []byte, contentType string) (int, EmbedUsage, error) {
	if err := s.objects.Put(ctx, key, contentType, bytes.NewReader(content), int64(len(content))); err != nil {
		return 0, EmbedUsage{}, fmt.Errorf("store original: %w", err)
	}
	sections, err := ExtractSectionsWithFilename(mime, docTitle, bytes.NewReader(content))
	if err != nil {
		return 0, EmbedUsage{}, fmt.Errorf("extract text: %w", err)
	}
	hasText := false
	for _, sec := range sections {
		if strings.TrimSpace(sec.Text) != "" {
			hasText = true
			break
		}
	}
	if !hasText {
		return 0, EmbedUsage{}, fmt.Errorf("no text extracted from %q", docTitle)
	}
	chunks := ChunkSections(documentID, docTitle, sections, s.chunkTarget, s.chunkOverlap)
	if len(chunks) == 0 {
		return 0, EmbedUsage{}, fmt.Errorf("no chunks produced from %q", docTitle)
	}
	res, err := s.embed.EmbedDocuments(ctx, chunkTexts(chunks))
	if err != nil {
		return 0, EmbedUsage{}, fmt.Errorf("embed chunks: %w", err)
	}
	if err := s.vs.DeleteDocument(ctx, documentID); err != nil {
		return 0, EmbedUsage{}, fmt.Errorf("clear stale vectors: %w", err)
	}
	if err := s.vs.UpsertChunks(ctx, userID, documentID, docTitle, chunks, res.Vectors); err != nil {
		return 0, EmbedUsage{}, fmt.Errorf("index chunks: %w", err)
	}

	// Cache the full parsed markdown in the object store (MinIO) in the same folder
	if parsedText, err := ExtractTextWithFilename(mime, docTitle, bytes.NewReader(content)); err == nil && parsedText != "" {
		_ = s.objects.Put(ctx, ParsedObjectKey(key), "text/markdown", strings.NewReader(parsedText), int64(len(parsedText)))
	}

	total := strings.Join(chunkTexts(chunks), "")
	return len(chunks), embedUsage(res.Usage.InputTokens, total), nil
}

// Search embeds the query, over-retrieves hybrid candidates, widens each
// hit with its neighbor window, then reranks (when a ranker is attached)
// and trims to TopK. The UsageReport carries the query-embed tokens plus
// the optional rerank generation usage for the ledger.
func (s *Service) Search(ctx context.Context, userID, query string, opts SearchOptions) ([]Scored, UsageReport, error) {
	var rep UsageReport
	k := opts.TopK
	if k <= 0 {
		k = defaultK
	}
	if k > maxK {
		k = maxK
	}
	emb, err := s.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, rep, fmt.Errorf("embed query: %w", err)
	}
	rep.Embed = embedUsage(emb.Usage.InputTokens, query)
	candidates := k * s.searchCfg.CandidateMult
	if candidates < k {
		candidates = k
	}
	if candidates > s.searchCfg.MaxCandidates {
		candidates = s.searchCfg.MaxCandidates
	}
	rows, err := s.vs.HybridSearch(ctx, userID, query, emb.Vectors[0], s.searchCfg.Alpha, candidates, opts.DocIDs)
	if err != nil {
		return nil, rep, fmt.Errorf("search index: %w", err)
	}
	// When no reranker is attached, hybrid RRF from Weaviate is the final ranking.
	// Trim to TopK immediately to avoid expanding unneeded candidate windows.
	if s.ranker == nil && len(rows) > k {
		rows = rows[:k]
	}
	rows, err = s.expand(ctx, userID, rows)
	if err != nil {
		return nil, rep, fmt.Errorf("expand windows: %w", err)
	}
	if s.ranker != nil && len(rows) > 1 {
		ranked, usage, _ := s.ranker.Rerank(ctx, query, rows, k) // fail-open: never errors
		rep.Reranked = true
		rep.RerankModel = s.ranker.ModelName()
		rep.RerankIn, rep.RerankOut = usage.InputTokens, usage.OutputTokens
		return ranked, rep, nil
	}
	if len(rows) > k {
		rows = rows[:k]
	}
	return rows, rep, nil
}

// expand widens each hit with neighbor chunks (small-to-big): one range
// read per document, windows sliced locally, overlapping windows share
// reads. The core Chunk stays the citation unit; Context carries the window.
// Goroutines run range expansions concurrently across distinct documents.
func (s *Service) expand(ctx context.Context, userID string, rows []Scored) ([]Scored, error) {
	before, after := s.searchCfg.ExpandBefore, s.searchCfg.ExpandAfter
	if before <= 0 && after <= 0 || len(rows) == 0 {
		return rows, nil
	}
	lo := map[string]int{}
	hi := map[string]int{}
	for _, r := range rows {
		if l, ok := lo[r.Chunk.DocumentID]; !ok || r.Chunk.Index-before < l {
			lo[r.Chunk.DocumentID] = max(r.Chunk.Index-before, 0)
		}
		if h, ok := hi[r.Chunk.DocumentID]; !ok || r.Chunk.Index+after > h {
			hi[r.Chunk.DocumentID] = r.Chunk.Index + after
		}
	}
	byDoc := map[string]map[int]Chunk{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for docID := range lo {
		wg.Add(1)
		go func(dID string) {
			defer wg.Done()
			chunks, err := s.vs.ExpandRange(ctx, userID, dID, lo[dID], hi[dID])
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			m := map[int]Chunk{}
			for _, c := range chunks {
				m[c.Index] = c
			}
			byDoc[dID] = m
		}(docID)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	for i, r := range rows {
		m := byDoc[r.Chunk.DocumentID]
		var parts []string
		for idx := max(r.Chunk.Index-before, 0); idx <= r.Chunk.Index+after; idx++ {
			if c, ok := m[idx]; ok {
				parts = append(parts, c.Content)
			}
		}
		if len(parts) > 1 {
			rows[i].Context = strings.Join(parts, "\n\n")
		}
	}
	return rows, nil
}

// embedUsage trusts provider-reported tokens; absent them it falls back
// to the classic len/4 estimate, flagged so the ledger stays honest.
func embedUsage(providerTokens int, text string) EmbedUsage {
	if providerTokens > 0 {
		return EmbedUsage{InputTokens: providerTokens}
	}
	n := len(text) / 4
	if n == 0 {
		n = 1
	}
	return EmbedUsage{InputTokens: n, Estimated: true}
}

func chunkTexts(chunks []Chunk) []string {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	return texts
}

// ObjectKey builds the legacy object-store key: userID/documentID/filename.
func ObjectKey(userID, documentID, filename string) string {
	if len(filename) > maxKeyBytes {
		filename = filename[:maxKeyBytes]
	}
	return userID + "/" + documentID + "/" + filename
}

// ProjectObjectKey builds the folder-structured object key for project files:
// userID/projects/projectID/documentID/filename.
func ProjectObjectKey(userID, projectID, documentID, filename string) string {
	if len(filename) > maxKeyBytes {
		filename = filename[:maxKeyBytes]
	}
	return userID + "/projects/" + projectID + "/" + documentID + "/" + filename
}

// DocumentObjectKey builds the folder-structured object key for global documents:
// userID/documents/documentID/filename.
func DocumentObjectKey(userID, documentID, filename string) string {
	if len(filename) > maxKeyBytes {
		filename = filename[:maxKeyBytes]
	}
	return userID + "/documents/" + documentID + "/" + filename
}

// ParsedObjectKey builds the object-store key for the extracted markdown text in MinIO.
// When called with 1 argument (objectKey), it places parsed.md in the same folder as objectKey.
// When called with 2 arguments (userID, documentID), it defaults to userID/documentID/parsed.md.
func ParsedObjectKey(args ...string) string {
	if len(args) == 1 {
		dir := path.Dir(args[0])
		if dir == "." || dir == "" {
			return "parsed.md"
		}
		return dir + "/parsed.md"
	}
	if len(args) >= 2 {
		return args[0] + "/" + args[1] + "/parsed.md"
	}
	return "parsed.md"
}
