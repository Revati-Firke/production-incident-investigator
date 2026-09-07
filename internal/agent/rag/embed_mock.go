package rag

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// EmbeddingDimension is the fixed vector size stored in PostgreSQL.
const EmbeddingDimension = 384

// Embedder produces dense vectors for text.
type Embedder interface {
	Name() string
	Dimension() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// MockEmbedder is a deterministic bag-of-tokens hash embedder (no API key).
// Similar documents share overlapping token buckets so cosine search works locally.
type MockEmbedder struct {
	dim int
}

// NewMockEmbedder creates a mock embedder with EmbeddingDimension.
func NewMockEmbedder() *MockEmbedder {
	return &MockEmbedder{dim: EmbeddingDimension}
}

func (m *MockEmbedder) Name() string   { return "mock" }
func (m *MockEmbedder) Dimension() int { return m.dim }

func (m *MockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = hashEmbed(text, m.dim)
	}
	return out, nil
}

func hashEmbed(text string, dim int) []float32 {
	vec := make([]float32, dim)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		vec[0] = 1
		return vec
	}
	for _, tok := range tokens {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		idx := int(h.Sum32() % uint32(dim))
		sign := float32(1)
		if h.Sum32()&1 == 1 {
			sign = -1
		}
		vec[idx] += sign
		// Secondary bucket for soft similarity.
		h2 := fnv.New32a()
		_, _ = h2.Write([]byte(tok + "#2"))
		idx2 := int(h2.Sum32() % uint32(dim))
		vec[idx2] += 0.5 * sign
	}
	return l2Normalize(vec)
}

func tokenize(text string) []string {
	lower := strings.ToLower(text)
	var (
		tokens []string
		cur    strings.Builder
	)
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		tok := cur.String()
		cur.Reset()
		if len(tok) < 2 {
			return
		}
		tokens = append(tokens, tok)
	}
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func l2Normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	norm := math.Sqrt(sum)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}

// ValidateEmbedding checks dimension.
func ValidateEmbedding(v []float32) error {
	if len(v) != EmbeddingDimension {
		return fmt.Errorf("embedding dimension %d, want %d", len(v), EmbeddingDimension)
	}
	return nil
}
