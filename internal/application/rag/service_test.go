package rag_test

import (
	"context"
	"encoding/json"
	"testing"

	agentrag "github.com/Revati-Firke/production-incident-investigator/internal/agent/rag"
	apprag "github.com/Revati-Firke/production-incident-investigator/internal/application/rag"
	domainrag "github.com/Revati-Firke/production-incident-investigator/internal/domain/rag"
	"github.com/google/uuid"
)

type memRepo struct {
	docs   map[uuid.UUID]*domainrag.Document
	chunks map[uuid.UUID][]domainrag.Chunk
}

func newMemRepo() *memRepo {
	return &memRepo{
		docs:   map[uuid.UUID]*domainrag.Document{},
		chunks: map[uuid.UUID][]domainrag.Chunk{},
	}
}

func (m *memRepo) CreateDocument(_ context.Context, doc *domainrag.Document) error {
	cp := *doc
	m.docs[doc.ID] = &cp
	return nil
}

func (m *memRepo) ReplaceChunks(_ context.Context, documentID uuid.UUID, chunks []domainrag.Chunk) error {
	cp := make([]domainrag.Chunk, len(chunks))
	copy(cp, chunks)
	m.chunks[documentID] = cp
	return nil
}

func (m *memRepo) GetDocument(_ context.Context, id uuid.UUID) (*domainrag.Document, error) {
	d, ok := m.docs[id]
	if !ok {
		return nil, context.Canceled
	}
	cp := *d
	return &cp, nil
}

func (m *memRepo) ListDocuments(_ context.Context, sourceType domainrag.SourceType, limit, offset int) ([]domainrag.Document, int, error) {
	out := make([]domainrag.Document, 0)
	for _, d := range m.docs {
		if sourceType != "" && d.SourceType != sourceType {
			continue
		}
		out = append(out, *d)
	}
	total := len(out)
	if offset > len(out) {
		return nil, total, nil
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

func (m *memRepo) DeleteDocument(_ context.Context, id uuid.UUID) error {
	delete(m.docs, id)
	delete(m.chunks, id)
	return nil
}

func (m *memRepo) CountDocuments(_ context.Context) (int, error) {
	return len(m.docs), nil
}

func (m *memRepo) SearchSimilar(_ context.Context, embedding []float32, topK int, sourceTypes []domainrag.SourceType) ([]domainrag.Hit, error) {
	type scored struct {
		hit   domainrag.Hit
		score float64
	}
	var all []scored
	allowed := map[domainrag.SourceType]bool{}
	for _, st := range sourceTypes {
		allowed[st] = true
	}
	for docID, chunks := range m.chunks {
		doc := m.docs[docID]
		if doc == nil {
			continue
		}
		if len(allowed) > 0 && !allowed[doc.SourceType] {
			continue
		}
		for _, c := range chunks {
			var sum float64
			for i := range embedding {
				sum += float64(embedding[i]) * float64(c.Embedding[i])
			}
			all = append(all, scored{
				score: sum,
				hit: domainrag.Hit{
					ChunkID:       c.ID,
					DocumentID:    docID,
					ChunkIndex:    c.ChunkIndex,
					Content:       c.Content,
					Score:         sum,
					DocumentTitle: doc.Title,
					DocumentURI:   doc.URI,
					SourceType:    doc.SourceType,
					DocumentMeta:  doc.Metadata,
				},
			})
		}
	}
	// simple selection sort by score desc
	for i := 0; i < len(all); i++ {
		best := i
		for j := i + 1; j < len(all); j++ {
			if all[j].score > all[best].score {
				best = j
			}
		}
		all[i], all[best] = all[best], all[i]
	}
	if topK > len(all) {
		topK = len(all)
	}
	out := make([]domainrag.Hit, 0, topK)
	for i := 0; i < topK; i++ {
		out = append(out, all[i].hit)
	}
	return out, nil
}

func TestService_IngestAndSearch(t *testing.T) {
	repo := newMemRepo()
	svc := apprag.NewService(repo, agentrag.NewMockEmbedder(), apprag.Config{TopK: 3})

	doc, err := svc.Ingest(context.Background(), domainrag.IngestRequest{
		SourceType: domainrag.SourceRunbook,
		Title:      "DB Pool Exhaustion",
		Content:    "Database connection pool exhaustion. Check connection leak in repository layer. Restart pods as temporary mitigation.",
		Metadata:   map[string]any{"team": "payments"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID == uuid.Nil {
		t.Fatal("missing id")
	}
	if len(repo.chunks[doc.ID]) == 0 {
		t.Fatal("expected chunks")
	}

	_, err = svc.Ingest(context.Background(), domainrag.IngestRequest{
		SourceType: domainrag.SourceIncidentPostmortem,
		Title:      "Redis storm",
		Content:    "Redis cache miss storm caused origin overload and thundering herd retries.",
	})
	if err != nil {
		t.Fatal(err)
	}

	hits, err := svc.Search(context.Background(), domainrag.SearchRequest{
		Query:       "payment database connection pool leak",
		TopK:        2,
		SourceTypes: []domainrag.SourceType{domainrag.SourceRunbook},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	if hits[0].DocumentTitle != "DB Pool Exhaustion" {
		t.Fatalf("unexpected top hit %q", hits[0].DocumentTitle)
	}

	raw, _ := json.Marshal(hits[0])
	if len(raw) == 0 {
		t.Fatal("marshal")
	}
}
