# Phase 5 — RAG Knowledge Retrieval

This document is the design reference for OpsPilot's retrieval-augmented generation
layer. Keep it updated when changing embeddings, schema, or tool wiring.

## Goals

- Ground investigation agents in **runbooks, playbooks, and postmortems**
- Prefer **evidence from documents** over hallucinated procedures
- Work **offline by default** with a mock embedder (no API key)
- Swap to OpenAI-compatible embeddings without schema changes

## Architecture

```text
Markdown / API ingest
        │
        ▼
  chunk + embed (384-d)
        │
        ▼
 PostgreSQL + pgvector
   documents
   document_chunks.embedding
        │
        ▼
 search_runbooks / search_previous_incidents (tools)
        │
        ▼
 Investigation worker evidence → LLM RCA
```

RAG is **not** a separate agent. It plugs into the existing tool framework so the
investigation loop already collects knowledge evidence on every job
(`search_runbooks` is in `defaultInvestigationTools`).

## Packages

| Path | Responsibility |
|------|----------------|
| `internal/domain/rag` | Document/chunk/hit models + repository port |
| `internal/agent/rag` | Chunking, embedders, vector formatting |
| `internal/agent/rag/tools` | RAG-backed knowledge tools |
| `internal/application/rag` | Ingest + search + seed orchestration |
| `internal/infrastructure/postgres/rag_repository.go` | pgvector persistence |
| `data/knowledge/` | Seed markdown corpus |
| `cmd/ingest` | CLI seeder |
| `migrations/000005_rag.*.sql` | Extension + tables + HNSW index |

## Embeddings

| Provider | Env | Notes |
|----------|-----|-------|
| `mock` (default) | `EMBEDDING_PROVIDER=mock` | Deterministic bag-of-tokens hash → L2-normalized 384-d vector |
| `openai` | `EMBEDDING_PROVIDER=openai` | `/v1/embeddings` with `dimensions=384` |

**Fixed dimension = 384** in `document_chunks.embedding vector(384)`.
Changing dimension requires a new migration and re-ingest.

## Document source types

- `runbook` — operational how-to
- `playbook` — incident response playbooks
- `incident_postmortem` — historical incidents (used by `search_previous_incidents`)
- `architecture` — system design notes
- `other`

Seed filename prefixes map to types: `runbook-*.md`, `playbook-*.md`,
`postmortem-*.md`, `architecture-*.md`.

## Tool mapping

| Tool | Filters |
|------|---------|
| `search_runbooks` | `runbook`, `playbook` |
| `search_previous_incidents` | `incident_postmortem` |

When `RAG_ENABLED=false`, wiring falls back to Phase 3 mock knowledge tools.

## HTTP API

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/documents` | Ingest document (chunk + embed) |
| `GET` | `/api/v1/documents` | List (`source_type`, `limit`, `offset`) |
| `GET` | `/api/v1/documents/{id}` | Get one |
| `DELETE` | `/api/v1/documents/{id}` | Delete document + chunks |
| `POST` | `/api/v1/rag/search` | Debug / operator similarity search |

### Search body

```json
{
  "query": "database connection pool",
  "top_k": 5,
  "source_types": ["runbook"],
  "min_score": 0
}
```

## Operational notes

1. **Postgres image must include pgvector** (`pgvector/pgvector:pg16`).
2. Upgrading from `postgres:16-alpine` requires recreating the volume:
   `docker compose down -v && docker compose up --build`.
3. API seeds `RAG_SEED_DIR` on start when the table is empty (`RAG_SEED_ON_START=true`).
4. Manual seed: `go run ./cmd/ingest -dir data/knowledge` (add `-force` to re-ingest).
5. Worker sets `RAG_SEED_ON_START=false` in Compose to avoid duplicate seed races.

## Future extensions (do not implement here unless requested)

- Hybrid BM25 + vector ranking
- Re-ranker model
- Automatic postmortem ingest when incidents resolve
- Populate `memory.Documents` in the investigation agent prompt
- Per-tenant knowledge bases
- Embedding cache / batch async reindex job

## Verification checklist

```bash
gofmt -w .
go test ./...
docker compose down -v
docker compose up --build -d
go test -tags=integration ./tests/integration/... -v
curl -s http://localhost:8080/api/v1/documents | jq .
curl -s -X POST http://localhost:8080/api/v1/rag/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"connection pool","top_k":3}' | jq .
```
