//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

const defaultBaseURL = "http://localhost:8080/api/v1"

func baseURL() string {
	if v := os.Getenv("E2E_BASE_URL"); v != "" {
		return v
	}
	return defaultBaseURL
}

type apiResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestHealthEndpoints(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	for _, path := range []string{"/health/live", "/health"} {
		resp, err := client.Get(baseURL() + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, resp.StatusCode)
		}

		var body apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if !body.Success {
			t.Fatalf("%s: success=false", path)
		}
	}
}

func TestIncidentLifecycleE2E(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}

	createBody := map[string]string{
		"title":        "E2E integration test incident",
		"severity":     "high",
		"service":      "payment-service",
		"environment":  "production",
		"description":  "Automated end-to-end test",
		"alert_source": "integration-test",
	}
	payload, _ := json.Marshal(createBody)

	resp, err := client.Post(baseURL()+"/incidents", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status %d: %s", resp.StatusCode, body)
	}

	var created apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if !created.Success {
		t.Fatal("create: success=false")
	}

	var data struct {
		Incident struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"incident"`
		Investigation struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"investigation"`
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(created.Data, &data); err != nil {
		t.Fatalf("unmarshal create data: %v", err)
	}
	if data.Incident.ID == "" || data.JobID == "" {
		t.Fatal("missing incident id or job id")
	}
	if data.Incident.Status != "RECEIVED" {
		t.Errorf("initial status = %s, want RECEIVED", data.Incident.Status)
	}

	incidentID := data.Incident.ID

	var invStatus string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		invResp, err := getJSON(client, fmt.Sprintf("%s/incidents/%s/investigation", baseURL(), incidentID))
		if err != nil {
			t.Fatalf("get investigation: %v", err)
		}
		var inv struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(invResp.Data, &inv)
		invStatus = inv.Status
		if invStatus == "completed" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if invStatus != "completed" {
		t.Fatalf("investigation status = %q, want completed", invStatus)
	}

	incResp, err := getJSON(client, fmt.Sprintf("%s/incidents/%s", baseURL(), incidentID))
	if err != nil {
		t.Fatalf("get incident: %v", err)
	}
	var inc struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(incResp.Data, &inc)
	if inc.Status != "ROOT_CAUSE_IDENTIFIED" && inc.Status != "INVESTIGATING" {
		t.Errorf("incident status = %s, want ROOT_CAUSE_IDENTIFIED (or INVESTIGATING if transition raced)", inc.Status)
	}

	// Prefer waiting until RCA is persisted on investigation
	var confidence float64
	var hasRootCause bool
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		invResp, err := getJSON(client, fmt.Sprintf("%s/incidents/%s/investigation", baseURL(), incidentID))
		if err != nil {
			t.Fatalf("get investigation rca: %v", err)
		}
		var inv struct {
			Status     string          `json:"status"`
			RootCause  json.RawMessage `json:"root_cause"`
			Confidence *float64        `json:"confidence"`
		}
		_ = json.Unmarshal(invResp.Data, &inv)
		if len(inv.RootCause) > 0 && inv.Confidence != nil {
			hasRootCause = true
			confidence = *inv.Confidence
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !hasRootCause {
		t.Fatal("expected investigation root_cause and confidence from AI agent")
	}
	if confidence < 0.8 {
		t.Errorf("confidence = %v, want >= 0.8 for payment-service DB exhaustion scenario", confidence)
	}

	runsResp, err := getJSON(client, fmt.Sprintf("%s/incidents/%s/agent-runs", baseURL(), incidentID))
	if err != nil {
		t.Fatalf("get agent runs: %v", err)
	}
	var runs []map[string]any
	if err := json.Unmarshal(runsResp.Data, &runs); err != nil {
		t.Fatalf("unmarshal agent runs: %v", err)
	}
	if len(runs) < 1 {
		t.Fatal("expected at least one agent run")
	}

	timelineResp, err := getJSON(client, fmt.Sprintf("%s/incidents/%s/timeline", baseURL(), incidentID))
	if err != nil {
		t.Fatalf("get timeline: %v", err)
	}
	var events []map[string]any
	if err := json.Unmarshal(timelineResp.Data, &events); err != nil {
		t.Fatalf("unmarshal timeline: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("timeline events = %d, want >= 3", len(events))
	}

	// Phase 3+: worker runs tools; evidence should be populated
	var evidenceCount int
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		evidenceResp, err := getJSON(client, fmt.Sprintf("%s/incidents/%s/evidence", baseURL(), incidentID))
		if err != nil {
			t.Fatalf("get evidence: %v", err)
		}
		var evidence []map[string]any
		_ = json.Unmarshal(evidenceResp.Data, &evidence)
		evidenceCount = len(evidence)
		if evidenceCount >= 5 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if evidenceCount < 5 {
		t.Fatalf("evidence count = %d, want >= 5 (worker tool executions)", evidenceCount)
	}
}

func TestToolsAPI(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}

	toolsResp, err := getJSON(client, baseURL()+"/tools")
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var tools []map[string]any
	if err := json.Unmarshal(toolsResp.Data, &tools); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}
	if len(tools) != 13 {
		t.Fatalf("tool count = %d, want 13", len(tools))
	}

	// Create incident for tool execution
	payload, _ := json.Marshal(map[string]string{
		"title": "Tool test", "severity": "high",
		"service": "payment-service", "environment": "production",
	})
	resp, err := client.Post(baseURL()+"/incidents", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()
	var created apiResponse
	_ = json.NewDecoder(resp.Body).Decode(&created)
	var data struct {
		Incident struct {
			ID string `json:"id"`
		} `json:"incident"`
	}
	_ = json.Unmarshal(created.Data, &data)
	incidentID := data.Incident.ID

	// Execute read-only tool
	execResp, err := client.Post(
		fmt.Sprintf("%s/incidents/%s/tools/search_logs/execute", baseURL(), incidentID),
		"application/json", bytes.NewReader([]byte(`{}`)),
	)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	defer execResp.Body.Close()
	if execResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(execResp.Body)
		t.Fatalf("execute status %d: %s", execResp.StatusCode, body)
	}

	// Approval-required tool without approval returns 202
	prPayload, _ := json.Marshal(map[string]any{
		"input": map[string]string{"title": "test", "body": "body"},
	})
	prResp, err := client.Post(
		fmt.Sprintf("%s/incidents/%s/tools/create_github_issue/execute", baseURL(), incidentID),
		"application/json", bytes.NewReader(prPayload),
	)
	if err != nil {
		t.Fatalf("execute approval tool: %v", err)
	}
	defer prResp.Body.Close()
	if prResp.StatusCode != http.StatusAccepted {
		t.Fatalf("approval tool status %d, want 202", prResp.StatusCode)
	}
}

func TestCreateIncidentValidation(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	payload := []byte(`{"title":"x","severity":"invalid","service":"s","environment":"prod"}`)
	resp, err := client.Post(baseURL()+"/incidents", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}

	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error == nil || body.Error.Code != "INVALID_SEVERITY" {
		t.Fatalf("error = %+v, want INVALID_SEVERITY", body.Error)
	}
}

func TestRAGDocumentsAndSearch(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}

	listResp, err := getJSON(client, baseURL()+"/documents?limit=20")
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listResp.Data, &page); err != nil {
		t.Fatalf("unmarshal documents: %v", err)
	}
	if page.Total < 1 {
		t.Fatalf("expected seeded documents, total=%d", page.Total)
	}

	searchPayload, _ := json.Marshal(map[string]any{
		"query":        "database connection pool exhaustion payment",
		"top_k":        3,
		"source_types": []string{"runbook", "playbook"},
	})
	resp, err := client.Post(baseURL()+"/rag/search", "application/json", bytes.NewReader(searchPayload))
	if err != nil {
		t.Fatalf("rag search: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("search status %d: %s", resp.StatusCode, body)
	}
	var envelope apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("search success=false: %+v", envelope.Error)
	}
	var result struct {
		Hits []map[string]any `json:"hits"`
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		t.Fatalf("unmarshal hits: %v", err)
	}
	if len(result.Hits) == 0 {
		t.Fatal("expected RAG hits for connection pool query")
	}
}

func getJSON(client *http.Client, url string) (*apiResponse, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if !body.Success {
		return nil, fmt.Errorf("api error: %+v", body.Error)
	}
	return &body, nil
}
