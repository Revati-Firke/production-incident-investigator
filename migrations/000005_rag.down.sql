-- Phase 5 rollback: RAG knowledge base

DROP INDEX IF EXISTS idx_document_chunks_embedding_hnsw;
DROP INDEX IF EXISTS idx_document_chunks_document_id;
DROP TABLE IF EXISTS document_chunks;

DROP INDEX IF EXISTS idx_documents_created_at;
DROP INDEX IF EXISTS idx_documents_title;
DROP INDEX IF EXISTS idx_documents_source_type;
DROP TABLE IF EXISTS documents;

DROP EXTENSION IF EXISTS vector;
