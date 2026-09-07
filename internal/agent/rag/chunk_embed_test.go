package rag_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/rag"
)

func TestChunkText_RespectsSizeAndOverlap(t *testing.T) {
	text := strings.Repeat("word ", 400)
	chunks := rag.ChunkText(text, 200, 40)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if strings.TrimSpace(c) == "" {
			t.Fatalf("chunk %d empty", i)
		}
	}
}

func TestMockEmbedder_DeterministicAndNormalized(t *testing.T) {
	e := rag.NewMockEmbedder()
	a, err := e.Embed(context.Background(), []string{"database connection pool exhaustion"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Embed(context.Background(), []string{"database connection pool exhaustion"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a[0]) != rag.EmbeddingDimension {
		t.Fatalf("dim=%d", len(a[0]))
	}
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("not deterministic at %d", i)
		}
	}

	var sum float64
	for _, v := range a[0] {
		sum += float64(v) * float64(v)
	}
	if sum < 0.99 || sum > 1.01 {
		t.Fatalf("expected ~unit vector, got norm^2=%f", sum)
	}
}

func TestMockEmbedder_SimilarQueriesCloser(t *testing.T) {
	e := rag.NewMockEmbedder()
	vecs, err := e.Embed(context.Background(), []string{
		"database connection pool leak payment service",
		"postgres connection pool exhaustion in payments",
		"redis cache miss storm thundering herd",
	})
	if err != nil {
		t.Fatal(err)
	}
	simDB := cosine(vecs[0], vecs[1])
	simOther := cosine(vecs[0], vecs[2])
	if simDB <= simOther {
		t.Fatalf("expected db-related texts closer: db=%.4f other=%.4f", simDB, simOther)
	}
}

func cosine(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func TestFormatVector(t *testing.T) {
	v := make([]float32, rag.EmbeddingDimension)
	v[0] = 1
	s, err := rag.FormatVector(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		t.Fatalf("bad format %q", s)
	}
}
