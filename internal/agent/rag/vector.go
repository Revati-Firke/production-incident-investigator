package rag

import (
	"fmt"
	"strings"
)

// FormatVector converts a float32 slice to pgvector literal syntax.
func FormatVector(v []float32) (string, error) {
	if err := ValidateEmbedding(v); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(len(v)*8 + 2)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", x)
	}
	b.WriteByte(']')
	return b.String(), nil
}
