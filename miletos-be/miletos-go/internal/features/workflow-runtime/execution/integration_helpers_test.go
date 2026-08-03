//go:build integration

package execution_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	executionfeature "miletos-go/internal/features/workflow-runtime/execution"
	model "miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/queue"
	repository "miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/database"
)

type recordingQueue struct {
	mutex sync.Mutex
	jobs  []model.NodeJob
	err   error
}

func (queue *recordingQueue) Push(_ context.Context, _, _ string, payload []byte) error {
	if queue.err != nil {
		return queue.err
	}
	var job model.NodeJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return err
	}
	queue.mutex.Lock()
	queue.jobs = append(queue.jobs, job)
	queue.mutex.Unlock()
	return nil
}

func (*recordingQueue) Consume(
	context.Context,
	string,
	func(context.Context, []byte) queue.RecordResult,
) error {
	return nil
}

func (queue *recordingQueue) snapshot() []model.NodeJob {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	return append([]model.NodeJob(nil), queue.jobs...)
}

func newEngineNodeProcessor(
	t *testing.T,
	workflows *workflowfeature.WorkflowRepository,
	executions *repository.ExecutionRepository,
	registry *plugin.NodeRegistry,
	scheduler *executionfeature.Scheduler,
	nodeQueue queue.Queue,
	topic string,
	maximumAttempts int,
	retryDelay time.Duration,
) *executionfeature.NodeProcessor {
	t.Helper()
	processor, err := executionfeature.NewNodeProcessor(
		workflows, executions, registry, scheduler, nodeQueue, topic,
		maximumAttempts, retryDelay,
	)
	if err != nil {
		t.Fatalf("NewNodeProcessor() error = %v", err)
	}
	return processor
}

func newEngineScheduler(
	t *testing.T,
	workflows *workflowfeature.WorkflowRepository,
	executions *repository.ExecutionRepository,
	nodeQueue queue.Queue,
	topic string,
) *executionfeature.Scheduler {
	t.Helper()
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	return executionfeature.NewScheduler(workflows, executions, nodeQueue, topic, registry)
}

func engineIntegrationDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	connectionURL := strings.TrimSpace(os.Getenv("MILETOS_TEST_DATABASE_URL"))
	if connectionURL == "" {
		t.Skip("MILETOS_TEST_DATABASE_URL is required for engine integration tests")
	}
	configuration, err := pgxpool.ParseConfig(connectionURL)
	if err != nil {
		t.Fatalf("MILETOS_TEST_DATABASE_URL is invalid")
	}
	if !safeEngineTestDatabase(configuration.ConnConfig.Host, configuration.ConnConfig.Database) {
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
	_, helperFile, _, _ := runtime.Caller(0)
	files, err := filepath.Glob(filepath.Join(
		filepath.Dir(helperFile), "..", "..", "..", "..", ".local", "migrations", "*.sql",
	))
	if err != nil || len(files) != 10 {
		pool.Close()
		t.Fatalf("expected ten local migration files")
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
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS workflow_runtime CASCADE")
		pool.Close()
	})
	return pool
}

func safeEngineTestDatabase(host, database string) bool {
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

func engineDatabaseClient(t *testing.T) *database.Client {
	t.Helper()
	client, err := database.Open(
		context.Background(),
		strings.TrimSpace(os.Getenv("MILETOS_TEST_DATABASE_URL")),
	)
	if err != nil {
		t.Fatalf("open GORM integration database: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close GORM integration database: %v", err)
		}
	})
	return client
}

func engineWorkflow() workflowfeature.Workflow {
	return workflowfeature.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Engine Integration", Revision: 1,
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
		Metadata: map[string]any{"suite": "integration"},
	}
}

func seedEngineExecution(
	t *testing.T,
	pool *pgxpool.Pool,
	workflow workflowfeature.Workflow,
	mode string,
) model.Execution {
	t.Helper()
	client := engineDatabaseClient(t)
	snapshot, err := workflowfeature.NewWorkflowRepository(client).Save(
		context.Background(), workflow,
	)
	if err != nil {
		t.Fatalf("Save() workflow error = %v", err)
	}
	execution, err := repository.NewExecutionRepository(client).Create(
		context.Background(), workflow, snapshot.ID, mode,
		model.ExecutionOriginManualDirect, "corr-engine", "", "",
	)
	if err != nil {
		t.Fatalf("Create() execution error = %v", err)
	}
	return execution
}

func runNodeSuccess(
	t *testing.T,
	executions *repository.ExecutionRepository,
	workflow workflowfeature.Workflow,
	execution model.Execution,
	nodeID string,
	output any,
) model.NodeJob {
	t.Helper()
	node, changed, err := executions.MarkNodeQueued(
		context.Background(), workflow.CompanyID, execution.ID, nodeID,
	)
	if err != nil || !changed {
		t.Fatalf("MarkNodeQueued(%s) = (%#v, %v, %v)", nodeID, node, changed, err)
	}
	job := model.NodeJob{
		CompanyID: workflow.CompanyID, WorkflowID: workflow.ID, ExecutionID: execution.ID,
		NodeID: nodeID, NodeExecutionID: node.ID, Attempt: node.Attempt,
		CorrelationID: execution.CorrelationID,
	}
	if started, err := executions.MarkNodeRunning(context.Background(), job); err != nil || !started {
		t.Fatalf("MarkNodeRunning(%s) = (%v, %v)", nodeID, started, err)
	}
	if err := executions.SaveNodeSuccess(context.Background(), job, output); err != nil {
		t.Fatalf("SaveNodeSuccess(%s) error = %v", nodeID, err)
	}
	return job
}

type executionArtifactCounts struct {
	nodes    int
	attempts int
	events   int
	jobs     int
}

func countExecutionArtifacts(
	t *testing.T,
	pool *pgxpool.Pool,
	executionID string,
	queue *recordingQueue,
) executionArtifactCounts {
	t.Helper()
	var counts executionArtifactCounts
	if err := pool.QueryRow(
		context.Background(),
		"SELECT count(*) FROM workflow_runtime.node_executions WHERE workflow_execution_id = $1",
		executionID,
	).Scan(&counts.nodes); err != nil {
		t.Fatalf("count node executions: %v", err)
	}
	if err := pool.QueryRow(
		context.Background(),
		"SELECT count(*) FROM workflow_runtime.node_execution_attempts WHERE workflow_execution_id = $1",
		executionID,
	).Scan(&counts.attempts); err != nil {
		t.Fatalf("count node attempts: %v", err)
	}
	if err := pool.QueryRow(
		context.Background(),
		"SELECT count(*) FROM workflow_runtime.execution_events WHERE workflow_execution_id = $1",
		executionID,
	).Scan(&counts.events); err != nil {
		t.Fatalf("count execution events: %v", err)
	}
	counts.jobs = len(queue.snapshot())
	return counts
}

func setTerminalExecution(
	t *testing.T,
	pool *pgxpool.Pool,
	executionID string,
	status model.ExecutionStatus,
) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		UPDATE workflow_runtime.workflow_executions
		SET status = $2,
			terminal_outputs = '{"preservedOutput":"value"}'::jsonb,
			failure_summary = '{"message":"preserved error"}'::jsonb,
			finished_at = created_at,
			updated_at = created_at
		WHERE workflow_execution_id = $1`,
		executionID, status,
	); err != nil {
		t.Fatalf("set terminal execution status %s: %v", status, err)
	}
}

func nodeStartedAt(
	t *testing.T,
	pool *pgxpool.Pool,
	nodeExecutionID string,
) time.Time {
	t.Helper()
	var startedAt time.Time
	if err := pool.QueryRow(
		context.Background(),
		"SELECT started_at FROM workflow_runtime.node_executions WHERE node_execution_id = $1",
		nodeExecutionID,
	).Scan(&startedAt); err != nil {
		t.Fatalf("load node started_at: %v", err)
	}
	return startedAt
}
