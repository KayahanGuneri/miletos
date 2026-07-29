//go:build integration

package repository

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"miletos-go/internal/model"
)

func integrationDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	connectionURL := strings.TrimSpace(os.Getenv("MILETOS_TEST_DATABASE_URL"))
	if connectionURL == "" {
		t.Skip("MILETOS_TEST_DATABASE_URL is required for repository integration tests")
	}
	configuration, err := pgxpool.ParseConfig(connectionURL)
	if err != nil {
		t.Fatalf("MILETOS_TEST_DATABASE_URL is invalid")
	}
	if !safeRepositoryTestDatabase(
		configuration.ConnConfig.Host, configuration.ConnConfig.Database,
	) {
		t.Skipf(
			"MILETOS_TEST_DATABASE_URL is not explicitly test-safe (host %q)",
			configuration.ConnConfig.Host,
		)
	}
	t.Logf("using test PostgreSQL host %q", configuration.ConnConfig.Host)
	configuration.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), configuration)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("connect to integration database: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS workflow_runtime CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("reset integration schema: %v", err)
	}
	applyIntegrationMigrations(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS workflow_runtime CASCADE")
		pool.Close()
	})
	return pool
}

func safeRepositoryTestDatabase(host, database string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	database = strings.ToLower(strings.TrimSpace(database))
	if strings.Contains(host, "prod") || strings.Contains(host, "stag") ||
		strings.Contains(database, "prod") || strings.Contains(database, "stag") {
		return false
	}
	localHost := host == "localhost" || host == "127.0.0.1" || host == "::1"
	testHost := strings.Contains(host, "test")
	return (localHost || testHost) && strings.Contains(database, "test")
}

func applyIntegrationMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, helperFile, _, _ := runtime.Caller(0)
	directory := filepath.Join(filepath.Dir(helperFile), "..", "..", ".local", "migrations")
	files, err := filepath.Glob(filepath.Join(directory, "*.sql"))
	if err != nil || len(files) != 9 {
		t.Fatalf("expected nine local migration files in %s; prepare the test schema manually if unavailable", directory)
	}
	sort.Strings(files)
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", filepath.Base(file), err)
		}
		up := strings.SplitN(string(contents), "-- +goose Down", 2)[0]
		if _, err := pool.Exec(context.Background(), up); err != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(file), err)
		}
	}
}

func integrationWorkflow() model.Workflow {
	return model.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Integration Workflow", Revision: 1,
		Nodes: []model.Node{
			{
				ID: "root", Type: "core.static-input", Version: "v1",
				Configuration: map[string]any{"value": "payload"},
			},
			{ID: "terminal", Type: "core.terminal", Version: "v1"},
		},
		Edges: []model.Edge{{
			ID: "edge-1", SourceNodeID: "root", SourceOutputPort: "output",
			TargetNodeID: "terminal", TargetInputPort: "input",
		}},
		Metadata: map[string]any{"environment": "integration"},
	}
}

func seedIntegrationExecution(
	t *testing.T,
	pool *pgxpool.Pool,
	workflow model.Workflow,
	mode string,
) (model.WorkflowSnapshot, model.Execution) {
	t.Helper()
	workflowRepository := NewWorkflowRepository(pool)
	executionRepository := NewExecutionRepository(pool)
	snapshot, err := workflowRepository.Save(context.Background(), workflow)
	if err != nil {
		t.Fatalf("Save() workflow error = %v", err)
	}
	execution, err := executionRepository.Create(
		context.Background(), workflow, snapshot.ID, mode, "corr-integration", "", "",
	)
	if err != nil {
		t.Fatalf("Create() execution error = %v", err)
	}
	return snapshot, execution
}
