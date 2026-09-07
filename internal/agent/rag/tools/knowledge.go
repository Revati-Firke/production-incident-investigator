package ragtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	apprag "github.com/Revati-Firke/production-incident-investigator/internal/application/rag"
	domainrag "github.com/Revati-Firke/production-incident-investigator/internal/domain/rag"
)

// SearchRunbooksTool retrieves runbooks/playbooks via RAG.
type SearchRunbooksTool struct {
	svc  *apprag.Service
	topK int
}

// NewSearchRunbooksTool creates the RAG-backed search_runbooks tool.
func NewSearchRunbooksTool(svc *apprag.Service, topK int) *SearchRunbooksTool {
	if topK <= 0 {
		topK = 5
	}
	return &SearchRunbooksTool{svc: svc, topK: topK}
}

func (t *SearchRunbooksTool) Name() string                      { return "search_runbooks" }
func (t *SearchRunbooksTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *SearchRunbooksTool) Description() string {
	return "Search runbooks and troubleshooting documentation using semantic RAG retrieval"
}
func (t *SearchRunbooksTool) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query; defaults to incident context if empty"},
		},
	}
}

func (t *SearchRunbooksTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	query := extractQuery(input, toolCtx)
	hits, err := t.svc.Search(ctx, domainrag.SearchRequest{
		Query: query,
		TopK:  t.topK,
		SourceTypes: []domainrag.SourceType{
			domainrag.SourceRunbook,
			domainrag.SourcePlaybook,
		},
	})
	if err != nil {
		return tools.Result{}, err
	}
	return hitsToResult("runbooks", hits), nil
}

// SearchIncidentsTool retrieves similar historical incident postmortems via RAG.
type SearchIncidentsTool struct {
	svc  *apprag.Service
	topK int
}

// NewSearchIncidentsTool creates the RAG-backed search_previous_incidents tool.
func NewSearchIncidentsTool(svc *apprag.Service, topK int) *SearchIncidentsTool {
	if topK <= 0 {
		topK = 5
	}
	return &SearchIncidentsTool{svc: svc, topK: topK}
}

func (t *SearchIncidentsTool) Name() string                      { return "search_previous_incidents" }
func (t *SearchIncidentsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *SearchIncidentsTool) Description() string {
	return "Search historical incident postmortems for similar patterns using semantic RAG retrieval"
}
func (t *SearchIncidentsTool) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query; defaults to incident context if empty"},
		},
	}
}

func (t *SearchIncidentsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	query := extractQuery(input, toolCtx)
	hits, err := t.svc.Search(ctx, domainrag.SearchRequest{
		Query:       query,
		TopK:        t.topK,
		SourceTypes: []domainrag.SourceType{domainrag.SourceIncidentPostmortem},
	})
	if err != nil {
		return tools.Result{}, err
	}
	return hitsToResult("previous_incidents", hits), nil
}

func extractQuery(input json.RawMessage, toolCtx tools.Context) string {
	var in struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(input, &in)
	q := strings.TrimSpace(in.Query)
	if q != "" {
		return q
	}
	parts := make([]string, 0, 3)
	if toolCtx.Service != "" {
		parts = append(parts, toolCtx.Service)
	}
	if toolCtx.Environment != "" {
		parts = append(parts, toolCtx.Environment)
	}
	if len(parts) == 0 {
		return "production incident troubleshooting"
	}
	return strings.Join(parts, " ") + " incident"
}

func hitsToResult(kind string, hits []domainrag.Hit) tools.Result {
	docs := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		docs = append(docs, map[string]any{
			"document_id": h.DocumentID.String(),
			"title":       h.DocumentTitle,
			"uri":         h.DocumentURI,
			"source_type": string(h.SourceType),
			"excerpt":     h.Content,
			"score":       h.Score,
			"chunk_index": h.ChunkIndex,
		})
	}
	summary := fmt.Sprintf("Found %d relevant %s via RAG", len(docs), kind)
	if len(docs) == 0 {
		summary = fmt.Sprintf("No relevant %s found", kind)
	}
	return tools.Result{
		Summary: summary,
		Data: map[string]any{
			"documents": docs,
			"provider":  "rag",
		},
	}
}
