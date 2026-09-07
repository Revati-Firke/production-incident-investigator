package rag

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SourceType classifies knowledge documents for filtered retrieval.
type SourceType string

const (
	SourceRunbook            SourceType = "runbook"
	SourceIncidentPostmortem SourceType = "incident_postmortem"
	SourcePlaybook           SourceType = "playbook"
	SourceArchitecture       SourceType = "architecture"
	SourceOther              SourceType = "other"
)

// Document is an ingested knowledge artifact.
type Document struct {
	ID         uuid.UUID
	SourceType SourceType
	Title      string
	URI        string
	Content    string
	Metadata   json.RawMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Chunk is an embedded fragment of a document.
type Chunk struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	ChunkIndex int
	Content    string
	Embedding  []float32
	TokenCount int
	Metadata   json.RawMessage
	CreatedAt  time.Time
}

// Hit is a similarity search result with document context.
type Hit struct {
	ChunkID       uuid.UUID
	DocumentID    uuid.UUID
	ChunkIndex    int
	Content       string
	Score         float64
	DocumentTitle string
	DocumentURI   string
	SourceType    SourceType
	DocumentMeta  json.RawMessage
}

// IngestRequest is the input for creating/updating a document and its chunks.
type IngestRequest struct {
	SourceType SourceType
	Title      string
	URI        string
	Content    string
	Metadata   map[string]any
}

// SearchRequest controls retrieval.
type SearchRequest struct {
	Query       string
	TopK        int
	SourceTypes []SourceType
	MinScore    float64
}
