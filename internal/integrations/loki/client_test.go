package loki_test

import (
	"context"
	"testing"

	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/loki"
)

func TestMockSearch_PaymentService(t *testing.T) {
	c := loki.NewMock()
	entries, err := c.Search(context.Background(), integrations.LogSearchRequest{
		Service:     "payment-service",
		Environment: "production",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected payment error logs, got %d", len(entries))
	}
	if c.Provider() != integrations.ProviderMock {
		t.Fatalf("provider=%s", c.Provider())
	}
}
