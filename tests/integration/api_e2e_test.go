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
	if inc.Status != "INVESTIGATING" {
		t.Errorf("incident status = %s, want INVESTIGATING (phase 2 stub)", inc.Status)
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
