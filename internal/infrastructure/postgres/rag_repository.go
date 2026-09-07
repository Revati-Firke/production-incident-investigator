package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	agentrag "github.com/Revati-Firke/production-incident-investigator/internal/agent/rag"
	domainrag "github.com/Revati-Firke/production-incident-investigator/internal/domain/rag"
)

// RAGRepository persists documents and pgvector chunks.
type RAGRepository struct {
	pool *Pool
}

// NewRAGRepository creates a RAG repository.
func NewRAGRepository(pool *Pool) *RAGRepository {
	return &RAGRepository{pool: pool}
}

func (r *RAGRepository) CreateDocument(ctx context.Context, doc *domainrag.Document) error {
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}
	now := time.Now().UTC()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = now
	}
	if len(doc.Metadata) == 0 {
		doc.Metadata = json.RawMessage(`{}`)
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO documents (id, source_type, title, uri, content, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		doc.ID, string(doc.SourceType), doc.Title, doc.URI, doc.Content, doc.Metadata, doc.CreatedAt, doc.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

func (r *RAGRepository) ReplaceChunks(ctx context.Context, documentID uuid.UUID, chunks []domainrag.Chunk) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin replace chunks: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}

	for i := range chunks {
		c := &chunks[i]
		if c.ID == uuid.Nil {
			c.ID = uuid.New()
		}
		c.DocumentID = documentID
		if c.CreatedAt.IsZero() {
			c.CreatedAt = time.Now().UTC()
		}
		if len(c.Metadata) == 0 {
			c.Metadata = json.RawMessage(`{}`)
		}
		vec, err := agentrag.FormatVector(c.Embedding)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO document_chunks (id, document_id, chunk_index, content, embedding, token_count, metadata, created_at)
			VALUES ($1,$2,$3,$4,$5::vector,$6,$7,$8)`,
			c.ID, c.DocumentID, c.ChunkIndex, c.Content, vec, c.TokenCount, c.Metadata, c.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert chunk %d: %w", c.ChunkIndex, err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE documents SET updated_at = $1 WHERE id = $2`, time.Now().UTC(), documentID); err != nil {
		return fmt.Errorf("touch document: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit replace chunks: %w", err)
	}
	return nil
}

func (r *RAGRepository) GetDocument(ctx context.Context, id uuid.UUID) (*domainrag.Document, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, source_type, title, uri, content, metadata, created_at, updated_at
		FROM documents WHERE id = $1`, id)
	doc, err := scanDocument(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("document not found")
		}
		return nil, err
	}
	return doc, nil
}

func (r *RAGRepository) ListDocuments(ctx context.Context, sourceType domainrag.SourceType, limit, offset int) ([]domainrag.Document, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var (
		total int
		err   error
	)
	if sourceType == "" {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM documents`).Scan(&total)
	} else {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM documents WHERE source_type = $1`, string(sourceType)).Scan(&total)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("count documents: %w", err)
	}

	var rows pgx.Rows
	if sourceType == "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, source_type, title, uri, content, metadata, created_at, updated_at
			FROM documents ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, source_type, title, uri, content, metadata, created_at, updated_at
			FROM documents WHERE source_type = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			string(sourceType), limit, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	docs := make([]domainrag.Document, 0)
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, 0, err
		}
		docs = append(docs, *doc)
	}
	return docs, total, rows.Err()
}

func (r *RAGRepository) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("document not found")
	}
	return nil
}

func (r *RAGRepository) CountDocuments(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM documents`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *RAGRepository) SearchSimilar(ctx context.Context, embedding []float32, topK int, sourceTypes []domainrag.SourceType) ([]domainrag.Hit, error) {
	if topK <= 0 {
		topK = 5
	}
	vec, err := agentrag.FormatVector(embedding)
	if err != nil {
		return nil, err
	}

	args := []any{vec, topK}
	filter := ""
	if len(sourceTypes) > 0 {
		types := make([]string, len(sourceTypes))
		for i, st := range sourceTypes {
			types[i] = string(st)
		}
		args = append(args, types)
		filter = fmt.Sprintf(" AND d.source_type = ANY($%d)", len(args))
	}

	query := fmt.Sprintf(`
		SELECT c.id, c.document_id, c.chunk_index, c.content,
		       1 - (c.embedding <=> $1::vector) AS score,
		       d.title, d.uri, d.source_type, d.metadata
		FROM document_chunks c
		JOIN documents d ON d.id = c.document_id
		WHERE TRUE%s
		ORDER BY c.embedding <=> $1::vector
		LIMIT $2`, filter)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search similar: %w", err)
	}
	defer rows.Close()

	hits := make([]domainrag.Hit, 0)
	for rows.Next() {
		var h domainrag.Hit
		var source string
		if err := rows.Scan(
			&h.ChunkID, &h.DocumentID, &h.ChunkIndex, &h.Content, &h.Score,
			&h.DocumentTitle, &h.DocumentURI, &source, &h.DocumentMeta,
		); err != nil {
			return nil, fmt.Errorf("scan hit: %w", err)
		}
		h.SourceType = domainrag.SourceType(source)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func scanDocument(row pgx.Row) (*domainrag.Document, error) {
	var doc domainrag.Document
	var source string
	err := row.Scan(
		&doc.ID, &source, &doc.Title, &doc.URI, &doc.Content, &doc.Metadata, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	doc.SourceType = domainrag.SourceType(source)
	return &doc, nil
}
