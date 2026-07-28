package persistence_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"miletos-go/internal/config"
	"miletos-go/internal/engine"
	enginepersistence "miletos-go/internal/engine/lifecycle/persistence"
	"miletos-go/internal/engine/nodes/core"
	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	postgresrepository "miletos-go/internal/infra/postgres"
	repository "miletos-go/internal/ports/persistence"
	sharedclock "miletos-go/internal/shared/clock"
)

const persistentSyncRunnerSchema = "workflow_runtime"

type persistentSyncRunnerFixture struct {
	pool  *pgxpool.Pool
	store *postgresrepository.Store
}

type persistentSyncRunnerClock struct {
	next time.Time
}

func (clock *persistentSyncRunnerClock) IsValid() bool {
	return clock != nil
}

func (clock *persistentSyncRunnerClock) Now() time.Time {
	current := clock.next.UTC()
	clock.next = clock.next.Add(time.Second)
	return current
}

func TestPostgreSQLPersistentSyncRunnerPersistsSuccessfulExecution(t *testing.T) {
	fixture := openPersistentSyncRunnerFixture(t)

	identity := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	companyID := workflow.CompanyID("cp07-company-" + identity)
	otherCompanyID := workflow.CompanyID("cp07-other-company-" + identity)
	workflowID := workflow.WorkflowID("cp07-workflow-" + identity)
	executionID := execution.WorkflowExecutionID("cp07-execution-" + identity)
	snapshotID := repository.DefinitionSnapshotID(executionID.String() + "/snapshot")
	payloadMarker := "cp07-private-payload-" + identity

	fixture.registerCleanup(t, executionID, snapshotID)

	definition := newPersistentSyncRunnerDefinition(
		t,
		companyID,
		workflowID,
		payloadMarker,
	)

	request, err := engine.NewExecutionRequest(
		executionID,
		definition,
		"cp07-correlation-"+identity,
		nil,
	)
	if err != nil {
		t.Fatalf("NewExecutionRequest() returned an error: %v", err)
	}

	runner := newPersistentSyncRunner(t, fixture.store)

	result, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("SyncRunner.Run() returned an error: %v", err)
	}

	if !result.IsValid() {
		t.Fatal("persistent SyncRunResult is invalid")
	}

	if !result.IsSucceeded() {
		t.Fatalf("workflow status = %s, want SUCCEEDED", result.Status())
	}

	workflowRecord, err := fixture.store.GetWorkflowExecution(
		context.Background(),
		companyID,
		executionID,
	)
	if err != nil {
		t.Fatalf("GetWorkflowExecution() returned an error: %v", err)
	}

	if workflowRecord.Status() != execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"persisted workflow status = %s, want SUCCEEDED",
			workflowRecord.Status(),
		)
	}

	_, err = fixture.store.GetWorkflowExecution(
		context.Background(),
		otherCompanyID,
		executionID,
	)
	if err == nil {
		t.Fatal("cross-tenant GetWorkflowExecution() returned no error")
	}
	if !repository.IsNotFound(err) {
		t.Fatalf("cross-tenant error = %T %v, want not-found", err, err)
	}

	nextSequence := fixture.requireWorkflowState(
		t,
		companyID,
		executionID,
		snapshotID,
	)

	fixture.requireSucceededNodes(t, companyID, executionID)

	timelineCount := fixture.requireContiguousTimeline(
		t,
		companyID,
		executionID,
	)

	if nextSequence != timelineCount+1 {
		t.Fatalf(
			"next sequence = %d, want %d",
			nextSequence,
			timelineCount+1,
		)
	}

	fixture.requireNoExecutionErrors(t, companyID, executionID)
	fixture.requireSnapshotPayload(t, companyID, snapshotID, payloadMarker)
	fixture.requireNoRuntimePayloadLeak(t, companyID, executionID, payloadMarker)
}

func openPersistentSyncRunnerFixture(t *testing.T) persistentSyncRunnerFixture {
	t.Helper()

	if strings.TrimSpace(os.Getenv("MILETOS_RUNTIME_POSTGRES_PASSWORD")) == "" {
		t.Skip("MILETOS_RUNTIME_POSTGRES_PASSWORD is not configured")
	}

	configuration, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() returned an error: %v", err)
	}

	if configuration.PostgreSQL.Schema != persistentSyncRunnerSchema {
		t.Fatalf(
			"PostgreSQL schema = %q, want %q",
			configuration.PostgreSQL.Schema,
			persistentSyncRunnerSchema,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := postgresrepository.OpenPool(ctx, configuration.PostgreSQL)
	if err != nil {
		t.Fatalf("OpenPool() returned an error: %v", err)
	}
	t.Cleanup(pool.Close)

	store, err := postgresrepository.NewStore(pool)
	if err != nil {
		t.Fatalf("NewStore() returned an error: %v", err)
	}

	return persistentSyncRunnerFixture{pool: pool, store: store}
}

func newPersistentSyncRunner(
	t *testing.T,
	store repository.ExecutionLifecycleStore,
) engine.SyncRunner {
	t.Helper()

	limits, err := runtime.NewRuntimeLimits(64, 1, 4096)
	if err != nil {
		t.Fatalf("NewRuntimeLimits() returned an error: %v", err)
	}

	descriptors, err := core.CoreDescriptors()
	if err != nil {
		t.Fatalf("CoreDescriptors() returned an error: %v", err)
	}

	pluginRegistry, err := plugin.NewRegistry(descriptors)
	if err != nil {
		t.Fatalf("plugin.NewRegistry() returned an error: %v", err)
	}

	registrations, err := core.DefaultExecutorRegistrations(limits)
	if err != nil {
		t.Fatalf("DefaultExecutorRegistrations() returned an error: %v", err)
	}

	executorRegistry, err := runtime.NewExecutorRegistry(registrations)
	if err != nil {
		t.Fatalf("NewExecutorRegistry() returned an error: %v", err)
	}

	dependencies, err := engine.NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		sharedclock.From((&persistentSyncRunnerClock{
			next: time.Date(2026, time.July, 18, 8, 0, 0, 0, time.UTC),
		}).Now),
	)
	if err != nil {
		t.Fatalf("NewEngineDependencies() returned an error: %v", err)
	}

	recorder, err := enginepersistence.NewRecorder(store)
	if err != nil {
		t.Fatalf("persistence.NewRecorder() returned an error: %v", err)
	}

	dependencies, err = dependencies.WithLifecycleRecorder(recorder)
	if err != nil {
		t.Fatalf("WithLifecycleRecorder() returned an error: %v", err)
	}

	runner, err := engine.NewSyncRunner(dependencies)
	if err != nil {
		t.Fatalf("NewSyncRunner() returned an error: %v", err)
	}

	return runner
}

func newPersistentSyncRunnerDefinition(
	t *testing.T,
	companyID workflow.CompanyID,
	workflowID workflow.WorkflowID,
	payloadMarker string,
) workflow.WorkflowDefinition {
	t.Helper()

	source := mustPersistentNode(
		t,
		"source",
		core.StaticInputPluginType,
		[]byte(fmt.Sprintf(`{"value":{"message":%q}}`, payloadMarker)),
	)
	pass := mustPersistentNode(
		t,
		"pass",
		core.PassThroughPluginType,
		[]byte(`{}`),
	)
	terminal := mustPersistentNode(
		t,
		"terminal",
		core.TerminalPluginType,
		[]byte(`{}`),
	)

	definition, err := workflow.NewWorkflowDefinition(
		workflowID,
		companyID,
		"CP-07 Persistent SyncRunner Workflow",
		1,
		[]workflow.NodeDefinition{source, pass, terminal},
		[]workflow.EdgeDefinition{
			mustPersistentEdge(t, "edge-source-pass", source.ID(), pass.ID()),
			mustPersistentEdge(t, "edge-pass-terminal", pass.ID(), terminal.ID()),
		},
		[]byte(`{"scope":"cp07-persistent-e2e"}`),
	)
	if err != nil {
		t.Fatalf("NewWorkflowDefinition() returned an error: %v", err)
	}

	return definition
}

func mustPersistentNode(
	t *testing.T,
	id string,
	pluginType workflow.PluginType,
	configuration []byte,
) workflow.NodeDefinition {
	t.Helper()

	node, err := workflow.NewNodeDefinition(
		workflow.NodeID(id),
		pluginType,
		core.CorePluginVersion,
		configuration,
		nil,
	)
	if err != nil {
		t.Fatalf("NewNodeDefinition(%s) returned an error: %v", id, err)
	}

	return node
}

func mustPersistentEdge(
	t *testing.T,
	id string,
	sourceNodeID workflow.NodeID,
	targetNodeID workflow.NodeID,
) workflow.EdgeDefinition {
	t.Helper()

	edge, err := workflow.NewEdgeDefinition(
		workflow.EdgeID(id),
		sourceNodeID,
		core.OutputPortName,
		targetNodeID,
		core.InputPortName,
	)
	if err != nil {
		t.Fatalf("NewEdgeDefinition(%s) returned an error: %v", id, err)
	}

	return edge
}

func (fixture persistentSyncRunnerFixture) requireWorkflowState(
	t *testing.T,
	companyID workflow.CompanyID,
	executionID execution.WorkflowExecutionID,
	snapshotID repository.DefinitionSnapshotID,
) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var status string
	var persistedSnapshotID string
	var nextSequence int64
	var lockVersion int64
	var validatingAt bool
	var startedAt bool
	var finishedAt bool

	err := fixture.pool.QueryRow(
		ctx,
		`
SELECT
    status,
    snapshot_id,
    next_sequence_number,
    lock_version,
    validating_at IS NOT NULL,
    started_at IS NOT NULL,
    finished_at IS NOT NULL
FROM workflow_runtime.workflow_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
		companyID.String(),
		executionID.String(),
	).Scan(
		&status,
		&persistedSnapshotID,
		&nextSequence,
		&lockVersion,
		&validatingAt,
		&startedAt,
		&finishedAt,
	)
	if err != nil {
		t.Fatalf("query workflow state: %v", err)
	}

	if status != execution.WorkflowExecutionStatusSucceeded.String() {
		t.Fatalf("database workflow status = %q, want SUCCEEDED", status)
	}
	if persistedSnapshotID != snapshotID.String() {
		t.Fatalf("database snapshot ID = %q, want %q", persistedSnapshotID, snapshotID)
	}
	if nextSequence <= 1 || lockVersion <= 0 {
		t.Fatalf(
			"workflow concurrency state: nextSequence=%d lockVersion=%d",
			nextSequence,
			lockVersion,
		)
	}
	if !validatingAt || !startedAt || !finishedAt {
		t.Fatalf(
			"workflow timestamps: validating=%t started=%t finished=%t",
			validatingAt,
			startedAt,
			finishedAt,
		)
	}

	return nextSequence
}

func (fixture persistentSyncRunnerFixture) requireSucceededNodes(
	t *testing.T,
	companyID workflow.CompanyID,
	executionID execution.WorkflowExecutionID,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := fixture.pool.Query(
		ctx,
		`
SELECT node_id, status, attempt, lock_version
FROM workflow_runtime.node_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
ORDER BY node_id
`,
		companyID.String(),
		executionID.String(),
	)
	if err != nil {
		t.Fatalf("query node executions: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var nodeID string
		var status string
		var attempt int16
		var lockVersion int64

		if err := rows.Scan(&nodeID, &status, &attempt, &lockVersion); err != nil {
			t.Fatalf("scan node execution: %v", err)
		}
		if status != execution.NodeExecutionStatusSucceeded.String() {
			t.Fatalf("node %s status = %q, want SUCCEEDED", nodeID, status)
		}
		if attempt != 1 || lockVersion < 3 {
			t.Fatalf(
				"node %s concurrency state: attempt=%d lockVersion=%d",
				nodeID,
				attempt,
				lockVersion,
			)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate node executions: %v", err)
	}
	if count != 3 {
		t.Fatalf("persisted node count = %d, want 3", count)
	}
}

func (fixture persistentSyncRunnerFixture) requireContiguousTimeline(
	t *testing.T,
	companyID workflow.CompanyID,
	executionID execution.WorkflowExecutionID,
) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := fixture.pool.Query(
		ctx,
		`
SELECT sequence_number
FROM (
    SELECT sequence_number
    FROM workflow_runtime.execution_events
    WHERE company_id = $1
      AND workflow_execution_id = $2
    UNION ALL
    SELECT sequence_number
    FROM workflow_runtime.execution_logs
    WHERE company_id = $1
      AND workflow_execution_id = $2
) AS timeline
ORDER BY sequence_number
`,
		companyID.String(),
		executionID.String(),
	)
	if err != nil {
		t.Fatalf("query timeline: %v", err)
	}
	defer rows.Close()

	expected := int64(1)
	count := int64(0)
	for rows.Next() {
		var sequence int64
		if err := rows.Scan(&sequence); err != nil {
			t.Fatalf("scan timeline sequence: %v", err)
		}
		if sequence != expected {
			t.Fatalf("timeline sequence = %d, want %d", sequence, expected)
		}
		expected++
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate timeline: %v", err)
	}
	if count == 0 {
		t.Fatal("execution timeline is empty")
	}

	return count
}

func (fixture persistentSyncRunnerFixture) requireNoExecutionErrors(
	t *testing.T,
	companyID workflow.CompanyID,
	executionID execution.WorkflowExecutionID,
) {
	t.Helper()

	var count int64
	err := fixture.pool.QueryRow(
		context.Background(),
		`
SELECT COUNT(*)
FROM workflow_runtime.execution_errors
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
		companyID.String(),
		executionID.String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("count execution errors: %v", err)
	}
	if count != 0 {
		t.Fatalf("execution error count = %d, want 0", count)
	}
}

func (fixture persistentSyncRunnerFixture) requireSnapshotPayload(
	t *testing.T,
	companyID workflow.CompanyID,
	snapshotID repository.DefinitionSnapshotID,
	payloadMarker string,
) {
	t.Helper()

	var definitionJSON string
	err := fixture.pool.QueryRow(
		context.Background(),
		`
SELECT definition_json::text
FROM workflow_runtime.workflow_definition_snapshots
WHERE company_id = $1
  AND snapshot_id = $2
`,
		companyID.String(),
		snapshotID.String(),
	).Scan(&definitionJSON)
	if err != nil {
		t.Fatalf("query definition snapshot: %v", err)
	}
	if !strings.Contains(definitionJSON, payloadMarker) {
		t.Fatalf("definition snapshot does not contain payload marker %q", payloadMarker)
	}
}

func (fixture persistentSyncRunnerFixture) requireNoRuntimePayloadLeak(
	t *testing.T,
	companyID workflow.CompanyID,
	executionID execution.WorkflowExecutionID,
	payloadMarker string,
) {
	t.Helper()

	var leakCount int64
	err := fixture.pool.QueryRow(
		context.Background(),
		`
SELECT
    (
        SELECT COUNT(*)
        FROM workflow_runtime.workflow_executions
        WHERE company_id = $1
          AND workflow_execution_id = $2
          AND terminal_outputs::text LIKE '%' || $3 || '%'
    )
    +
    (
        SELECT COUNT(*)
        FROM workflow_runtime.node_executions
        WHERE company_id = $1
          AND workflow_execution_id = $2
          AND (
              COALESCE(input_summary::text, '') LIKE '%' || $3 || '%'
              OR COALESCE(output_summary::text, '') LIKE '%' || $3 || '%'
          )
    )
    +
    (
        SELECT COUNT(*)
        FROM workflow_runtime.execution_events
        WHERE company_id = $1
          AND workflow_execution_id = $2
          AND metadata::text LIKE '%' || $3 || '%'
    )
    +
    (
        SELECT COUNT(*)
        FROM workflow_runtime.execution_logs
        WHERE company_id = $1
          AND workflow_execution_id = $2
          AND metadata::text LIKE '%' || $3 || '%'
    )
`,
		companyID.String(),
		executionID.String(),
		payloadMarker,
	).Scan(&leakCount)
	if err != nil {
		t.Fatalf("inspect runtime payload leakage: %v", err)
	}
	if leakCount != 0 {
		t.Fatalf("raw payload marker found in %d runtime-history records", leakCount)
	}
}

func (fixture persistentSyncRunnerFixture) registerCleanup(
	t *testing.T,
	executionID execution.WorkflowExecutionID,
	snapshotID repository.DefinitionSnapshotID,
) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		statements := []struct {
			query string
			arg   string
		}{
			{`DELETE FROM workflow_runtime.execution_errors WHERE workflow_execution_id = $1`, executionID.String()},
			{`DELETE FROM workflow_runtime.execution_logs WHERE workflow_execution_id = $1`, executionID.String()},
			{`DELETE FROM workflow_runtime.execution_events WHERE workflow_execution_id = $1`, executionID.String()},
			{`DELETE FROM workflow_runtime.node_executions WHERE workflow_execution_id = $1`, executionID.String()},
			{`DELETE FROM workflow_runtime.workflow_executions WHERE workflow_execution_id = $1`, executionID.String()},
			{`DELETE FROM workflow_runtime.workflow_definition_snapshots WHERE snapshot_id = $1`, snapshotID.String()},
		}

		for _, statement := range statements {
			if _, err := fixture.pool.Exec(ctx, statement.query, statement.arg); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}
	})
}
