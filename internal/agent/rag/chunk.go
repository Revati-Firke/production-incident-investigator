package rag

import (
	"strings"
	"unicode"
)

const (
	// DefaultChunkSize is approximate characters per chunk.
	DefaultChunkSize = 800
	// DefaultChunkOverlap is characters of overlap between adjacent chunks.
	DefaultChunkOverlap = 120
)

// ChunkText splits text into overlapping chunks for embedding.
func ChunkText(text string, size, overlap int) []string {
	if size <= 0 {
		size = DefaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 4
	}

	normalized := normalizeWhitespace(text)
	if normalized == "" {
		return nil
	}
	if len(normalized) <= size {
		return []string{normalized}
	}

	var chunks []string
	start := 0
	for start < len(normalized) {
		end := start + size
		if end >= len(normalized) {
			chunks = append(chunks, strings.TrimSpace(normalized[start:]))
			break
		}

		// Prefer breaking on paragraph / sentence / word boundaries.
		cut := findBreak(normalized, start, end)
		chunk := strings.TrimSpace(normalized[start:cut])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		next := cut - overlap
		if next <= start {
			next = cut
		}
		start = next
	}
	return chunks
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if r == '\n' {
				b.WriteByte('\n')
				prevSpace = true
				continue
			}
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func findBreak(s string, start, end int) int {
	window := s[start:end]
	if i := strings.LastIndex(window, "\n\n"); i > len(window)/3 {
		return start + i + 2
	}
	if i := strings.LastIndexAny(window, ".!?"); i > len(window)/3 {
		return start + i + 1
	}
	if i := strings.LastIndex(window, " "); i > len(window)/3 {
		return start + i + 1
	}
	return end
}

// ApproxTokenCount estimates tokens (~4 chars/token).
func ApproxTokenCount(s string) int {
	n := (len(s) + 3) / 4
	if n < 1 && s != "" {
		return 1
	}
	return n
}
