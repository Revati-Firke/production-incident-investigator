package rag

import (
	"context"

	"github.com/google/uuid"
)

// Repository persists documents and vector chunks.
type Repository interface {
	CreateDocument(ctx context.Context, doc *Document) error
	ReplaceChunks(ctx context.Context, documentID uuid.UUID, chunks []Chunk) error
	GetDocument(ctx context.Context, id uuid.UUID) (*Document, error)
	ListDocuments(ctx context.Context, sourceType SourceType, limit, offset int) ([]Document, int, error)
	DeleteDocument(ctx context.Context, id uuid.UUID) error
	CountDocuments(ctx context.Context) (int, error)
	SearchSimilar(ctx context.Context, embedding []float32, topK int, sourceTypes []SourceType) ([]Hit, error)
}
