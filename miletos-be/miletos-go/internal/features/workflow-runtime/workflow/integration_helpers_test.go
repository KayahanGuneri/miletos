//go:build integration

package workflow_test

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
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/database"
)

func integrationDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	connectionURL := strings.TrimSpace(os.Getenv("MILETOS_TEST_DATABASE_URL"))
	if connectionURL == "" {
		t.Skip("MILETOS_TEST_DATABASE_URL is required for workflow integration tests")
	}
	configuration, err := pgxpool.ParseConfig(connectionURL)
	if err != nil {
		t.Fatalf("MILETOS_TEST_DATABASE_URL is invalid")
	}
	if !safeWorkflowTestDatabase(
		configuration.ConnConfig.Host, configuration.ConnConfig.Database,
	) {
		t.Skipf(
			"MILETOS_TEST_DATABASE_URL is not explicitly test-safe (host %q)",
			configuration.ConnConfig.Host,
		)
	}
	configuration.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), configuration)
	if err != nil {
		t.Fatalf("open workflow integration database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("connect to workflow integration database: %v", err)
	}
	if _, err := pool.Exec(
		context.Background(), "DROP SCHEMA IF EXISTS workflow_runtime CASCADE",
	); err != nil {
		pool.Close()
		t.Fatalf("reset workflow integration schema: %v", err)
	}
	_, helperFile, _, _ := runtime.Caller(0)
	files, err := filepath.Glob(filepath.Join(
		filepath.Dir(helperFile), "..", "..", "..", "..", ".local", "migrations", "*.sql",
	))
	if err != nil || len(files) != 13 {
		pool.Close()
		t.Fatalf("expected thirteen local migration files")
	}
	sort.Strings(files)
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			pool.Close()
			t.Fatalf("read migration %s: %v", filepath.Base(file), err)
		}
		up := strings.SplitN(string(contents), "-- +goose Down", 2)[0]
		if _, err := pool.Exec(context.Background(), up); err != nil {
			pool.Close()
			t.Fatalf("apply migration %s: %v", filepath.Base(file), err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(), "DROP SCHEMA IF EXISTS workflow_runtime CASCADE",
		)
		pool.Close()
	})
	return pool
}

func workflowIntegrationDatabaseClient(t *testing.T) *database.Client {
	t.Helper()
	client, err := database.Open(
		context.Background(),
		strings.TrimSpace(os.Getenv("MILETOS_TEST_DATABASE_URL")),
	)
	if err != nil {
		t.Fatalf("open GORM workflow integration database: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close GORM workflow integration database: %v", err)
		}
	})
	return client
}

func safeWorkflowTestDatabase(host, name string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(host, "prod") || strings.Contains(host, "stag") ||
		strings.Contains(name, "prod") || strings.Contains(name, "stag") {
		return false
	}
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	return (local || strings.Contains(host, "test")) && strings.Contains(name, "test")
}

func integrationWorkflow() workflowfeature.Workflow {
	return workflowfeature.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Workflow Integration", Revision: 1,
		Nodes: []workflowfeature.WorkflowNode{
			{
				ID: "root", Type: "core.static-input", Version: "v1",
				Configuration: map[string]any{"value": "payload"},
			},
			{ID: "terminal", Type: "core.terminal", Version: "v1"},
		},
		Edges: []workflowfeature.Edge{{
			ID: "edge-1", SourceNodeID: "root", SourceOutputPort: "output",
			TargetNodeID: "terminal", TargetInputPort: "input",
		}},
		Metadata: map[string]any{"environment": "integration"},
	}
}
