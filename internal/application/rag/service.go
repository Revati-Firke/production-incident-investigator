package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	agentrag "github.com/Revati-Firke/production-incident-investigator/internal/agent/rag"
	domainrag "github.com/Revati-Firke/production-incident-investigator/internal/domain/rag"
)

// Service handles document ingestion and retrieval.
type Service struct {
	repo         domainrag.Repository
	embedder     agentrag.Embedder
	chunkSize    int
	chunkOverlap int
	topK         int
	minScore     float64
}

// Config tunes the RAG application service.
type Config struct {
	ChunkSize    int
	ChunkOverlap int
	TopK         int
	MinScore     float64
}

// NewService creates a RAG application service.
func NewService(repo domainrag.Repository, embedder agentrag.Embedder, cfg Config) *Service {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = agentrag.DefaultChunkSize
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = agentrag.DefaultChunkOverlap
	}
	if cfg.TopK <= 0 {
		cfg.TopK = 5
	}
	return &Service{
		repo:         repo,
		embedder:     embedder,
		chunkSize:    cfg.ChunkSize,
		chunkOverlap: cfg.ChunkOverlap,
		topK:         cfg.TopK,
		minScore:     cfg.MinScore,
	}
}

// EmbedderName returns the active embedder name.
func (s *Service) EmbedderName() string {
	if s.embedder == nil {
		return ""
	}
	return s.embedder.Name()
}

// Ingest creates a document, chunks content, embeds, and stores vectors.
func (s *Service) Ingest(ctx context.Context, req domainrag.IngestRequest) (*domainrag.Document, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("content is required")
	}
	if req.SourceType == "" {
		req.SourceType = domainrag.SourceOther
	}
	if !validSource(req.SourceType) {
		return nil, fmt.Errorf("invalid source_type %q", req.SourceType)
	}

	meta := json.RawMessage(`{}`)
	if req.Metadata != nil {
		b, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("metadata: %w", err)
		}
		meta = b
	}

	doc := &domainrag.Document{
		ID:         uuid.New(),
		SourceType: req.SourceType,
		Title:      strings.TrimSpace(req.Title),
		URI:        strings.TrimSpace(req.URI),
		Content:    req.Content,
		Metadata:   meta,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, err
	}
	if err := s.reindex(ctx, doc); err != nil {
		_ = s.repo.DeleteDocument(ctx, doc.ID)
		return nil, err
	}
	return doc, nil
}

func (s *Service) reindex(ctx context.Context, doc *domainrag.Document) error {
	parts := agentrag.ChunkText(doc.Content, s.chunkSize, s.chunkOverlap)
	if len(parts) == 0 {
		return fmt.Errorf("no chunks produced from content")
	}
	vectors, err := s.embedder.Embed(ctx, parts)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	chunks := make([]domainrag.Chunk, len(parts))
	for i, part := range parts {
		chunks[i] = domainrag.Chunk{
			ID:         uuid.New(),
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    part,
			Embedding:  vectors[i],
			TokenCount: agentrag.ApproxTokenCount(part),
			Metadata:   json.RawMessage(`{}`),
			CreatedAt:  time.Now().UTC(),
		}
	}
	return s.repo.ReplaceChunks(ctx, doc.ID, chunks)
}

// Search runs similarity retrieval.
func (s *Service) Search(ctx context.Context, req domainrag.SearchRequest) ([]domainrag.Hit, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	topK := req.TopK
	if topK <= 0 {
		topK = s.topK
	}
	vectors, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	hits, err := s.repo.SearchSimilar(ctx, vectors[0], topK, req.SourceTypes)
	if err != nil {
		return nil, err
	}
	minScore := req.MinScore
	if minScore == 0 {
		minScore = s.minScore
	}
	if minScore <= 0 {
		return hits, nil
	}
	filtered := make([]domainrag.Hit, 0, len(hits))
	for _, h := range hits {
		if h.Score >= minScore {
			filtered = append(filtered, h)
		}
	}
	return filtered, nil
}

// ListDocuments returns paginated documents.
func (s *Service) ListDocuments(ctx context.Context, sourceType domainrag.SourceType, limit, offset int) ([]domainrag.Document, int, error) {
	return s.repo.ListDocuments(ctx, sourceType, limit, offset)
}

// GetDocument returns one document.
func (s *Service) GetDocument(ctx context.Context, id uuid.UUID) (*domainrag.Document, error) {
	return s.repo.GetDocument(ctx, id)
}

// DeleteDocument removes a document and its chunks.
func (s *Service) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDocument(ctx, id)
}

// CountDocuments returns total documents.
func (s *Service) CountDocuments(ctx context.Context) (int, error) {
	return s.repo.CountDocuments(ctx)
}

// SeedFromDirectory ingests markdown/text files when the knowledge base is empty (or force).
func (s *Service) SeedFromDirectory(ctx context.Context, dir string, force bool) (int, error) {
	if !force {
		n, err := s.repo.CountDocuments(ctx)
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return 0, nil
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read seed dir: %w", err)
	}

	ingested := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".txt" {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return ingested, err
		}
		title, source, body := parseSeedFile(name, string(raw))
		_, err = s.Ingest(ctx, domainrag.IngestRequest{
			SourceType: source,
			Title:      title,
			URI:        "file://" + path,
			Content:    body,
			Metadata: map[string]any{
				"seed_file": name,
			},
		})
		if err != nil {
			return ingested, fmt.Errorf("ingest %s: %w", name, err)
		}
		ingested++
	}
	return ingested, nil
}

func parseSeedFile(filename, raw string) (title string, source domainrag.SourceType, body string) {
	source = domainrag.SourceOther
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	switch {
	case strings.HasPrefix(base, "runbook-"):
		source = domainrag.SourceRunbook
		title = humanize(strings.TrimPrefix(base, "runbook-"))
	case strings.HasPrefix(base, "postmortem-"):
		source = domainrag.SourceIncidentPostmortem
		title = humanize(strings.TrimPrefix(base, "postmortem-"))
	case strings.HasPrefix(base, "playbook-"):
		source = domainrag.SourcePlaybook
		title = humanize(strings.TrimPrefix(base, "playbook-"))
	case strings.HasPrefix(base, "architecture-"):
		source = domainrag.SourceArchitecture
		title = humanize(strings.TrimPrefix(base, "architecture-"))
	default:
		title = humanize(base)
	}

	body = raw
	lines := strings.SplitN(raw, "\n", 2)
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "# "))
		if len(lines) > 1 {
			body = lines[1]
		} else {
			body = ""
		}
	}
	return title, source, strings.TrimSpace(body)
}

func humanize(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

func validSource(s domainrag.SourceType) bool {
	switch s {
	case domainrag.SourceRunbook, domainrag.SourceIncidentPostmortem, domainrag.SourcePlaybook,
		domainrag.SourceArchitecture, domainrag.SourceOther:
		return true
	default:
		return false
	}
}
