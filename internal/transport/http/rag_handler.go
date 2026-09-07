package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	apprag "github.com/Revati-Firke/production-incident-investigator/internal/application/rag"
	domainrag "github.com/Revati-Firke/production-incident-investigator/internal/domain/rag"
)

// RAGHandler serves document ingest and search APIs.
type RAGHandler struct {
	rag *apprag.Service
}

// NewRAGHandler creates a RAG HTTP handler. rag may be nil when RAG is disabled.
func NewRAGHandler(rag *apprag.Service) *RAGHandler {
	return &RAGHandler{rag: rag}
}

type ingestDocumentRequest struct {
	SourceType string         `json:"source_type"`
	Title      string         `json:"title"`
	URI        string         `json:"uri"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata"`
}

type searchRequest struct {
	Query       string   `json:"query"`
	TopK        int      `json:"top_k"`
	SourceTypes []string `json:"source_types"`
	MinScore    float64  `json:"min_score"`
}

// Ingest handles POST /api/v1/documents.
func (h *RAGHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	if !h.requireRAG(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req ingestDocumentRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	doc, err := h.rag.Ingest(r.Context(), domainrag.IngestRequest{
		SourceType: domainrag.SourceType(strings.TrimSpace(req.SourceType)),
		Title:      req.Title,
		URI:        req.URI,
		Content:    req.Content,
		Metadata:   req.Metadata,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "INGEST_FAILED", err.Error())
		return
	}
	writeSuccess(w, http.StatusCreated, documentResponse(doc))
}

// List handles GET /api/v1/documents.
func (h *RAGHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.requireRAG(w) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	source := domainrag.SourceType(strings.TrimSpace(r.URL.Query().Get("source_type")))

	docs, total, err := h.rag.ListDocuments(r.Context(), source, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}
	items := make([]any, 0, len(docs))
	for i := range docs {
		items = append(items, documentResponse(&docs[i]))
	}
	if limit <= 0 {
		limit = 50
	}
	writeSuccess(w, http.StatusOK, PaginatedData{Items: items, Total: total, Limit: limit, Offset: offset})
}

// Get handles GET /api/v1/documents/{id}.
func (h *RAGHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !h.requireRAG(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid document ID")
		return
	}
	doc, err := h.rag.GetDocument(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Document not found")
		return
	}
	writeSuccess(w, http.StatusOK, documentResponse(doc))
}

// Delete handles DELETE /api/v1/documents/{id}.
func (h *RAGHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.requireRAG(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid document ID")
		return
	}
	if err := h.rag.DeleteDocument(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Document not found")
		return
	}
	writeSuccess(w, http.StatusOK, map[string]any{"deleted": true, "id": id.String()})
}

// Search handles POST /api/v1/rag/search.
func (h *RAGHandler) Search(w http.ResponseWriter, r *http.Request) {
	if !h.requireRAG(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req searchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	types := make([]domainrag.SourceType, 0, len(req.SourceTypes))
	for _, s := range req.SourceTypes {
		types = append(types, domainrag.SourceType(strings.TrimSpace(s)))
	}

	hits, err := h.rag.Search(r.Context(), domainrag.SearchRequest{
		Query:       req.Query,
		TopK:        req.TopK,
		SourceTypes: types,
		MinScore:    req.MinScore,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "SEARCH_FAILED", err.Error())
		return
	}

	items := make([]map[string]any, 0, len(hits))
	for _, hHit := range hits {
		items = append(items, map[string]any{
			"chunk_id":       hHit.ChunkID.String(),
			"document_id":    hHit.DocumentID.String(),
			"chunk_index":    hHit.ChunkIndex,
			"content":        hHit.Content,
			"score":          hHit.Score,
			"document_title": hHit.DocumentTitle,
			"document_uri":   hHit.DocumentURI,
			"source_type":    string(hHit.SourceType),
		})
	}
	writeSuccess(w, http.StatusOK, map[string]any{
		"embedder": h.rag.EmbedderName(),
		"hits":     items,
	})
}

func (h *RAGHandler) requireRAG(w http.ResponseWriter) bool {
	if h.rag == nil {
		writeError(w, http.StatusServiceUnavailable, "RAG_DISABLED", "RAG is disabled (set RAG_ENABLED=true)")
		return false
	}
	return true
}

func documentResponse(doc *domainrag.Document) map[string]any {
	return map[string]any{
		"id":          doc.ID.String(),
		"source_type": string(doc.SourceType),
		"title":       doc.Title,
		"uri":         doc.URI,
		"content":     doc.Content,
		"metadata":    json.RawMessage(doc.Metadata),
		"created_at":  doc.CreatedAt,
		"updated_at":  doc.UpdatedAt,
	}
}
