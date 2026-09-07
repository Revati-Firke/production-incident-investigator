package grafana_test

import (
	"encoding/json"
	"testing"

	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/grafana"
)

func TestIncidentFromAlert(t *testing.T) {
	raw := json.RawMessage(`{
		"title": "HighLatency",
		"status": "firing",
		"commonLabels": {
			"alertname": "HighLatency",
			"service": "payment-service",
			"environment": "production",
			"severity": "critical"
		},
		"alerts": [{
			"annotations": {"summary": "P95 latency above 3s"}
		}]
	}`)
	title, desc, sev, svc, env, err := grafana.IncidentFromAlert(raw)
	if err != nil {
		t.Fatal(err)
	}
	if title != "HighLatency" {
		t.Fatalf("title=%q", title)
	}
	if sev != "critical" {
		t.Fatalf("severity=%q", sev)
	}
	if svc != "payment-service" || env != "production" {
		t.Fatalf("svc=%q env=%q", svc, env)
	}
	if desc == "" {
		t.Fatal("expected description")
	}
}
