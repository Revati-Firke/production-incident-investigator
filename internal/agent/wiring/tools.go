package wiring

import (
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/mocks"
	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/postgres"
)

// NewToolService wires the tool registry, executor, and application service.
func NewToolService(pool *postgres.Pool, incidentRepo *postgres.IncidentRepository) (*tools.Executor, *apptool.Service, error) {
	registry, err := mocks.NewRegistry()
	if err != nil {
		return nil, nil, err
	}

	toolRepo := postgres.NewToolRepository(pool)
	incidentCtx := appincident.NewIncidentContext(incidentRepo)
	executor := tools.NewExecutor(registry, toolRepo, 30*time.Second)
	svc := apptool.NewService(executor, toolRepo, incidentCtx)
	return executor, svc, nil
}
