package inttools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	inttools "github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/integrations"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/loki"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/prometheus"
)

func TestSearchLogsTool_UsesProvider(t *testing.T) {
	tool := inttools.NewSearchLogsTool(loki.NewMock())
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`), tools.Context{
		Service:     "payment-service",
		Environment: "production",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	if data["provider"] != "mock" {
		t.Fatalf("provider=%v", data["provider"])
	}
}

func TestQueryMetricsTool_UsesProvider(t *testing.T) {
	tool := inttools.NewQueryMetricsTool(prometheus.NewMock())
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`), tools.Context{Service: "payment-service"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary == "" {
		t.Fatal("empty summary")
	}
}
