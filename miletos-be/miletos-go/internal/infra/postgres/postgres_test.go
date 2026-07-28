package postgres

import (
	bytes "bytes"
	context "context"
	base64 "encoding/base64"
	json "encoding/json"
	errors "errors"
	fmt "fmt"
	pgx "github.com/jackc/pgx/v5"
	pgconn "github.com/jackc/pgx/v5/pgconn"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
	math "math"
	runtime "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
	reflect "reflect"
	strings "strings"
	sync "sync"
	testing "testing"
	time "time"
)

func TestAsyncContextStoreIntegrationCASLifecycleAndIsolation(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerAsyncContextCleanup(t, store, scope.executionID.String())
	now := time.Now().UTC()
	inserted := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "progress", `{"step":1}`, 1, now, now)
	insertWrite, _ := repository.NewAsyncContextWrite(inserted, 0)
	if err := store.CompareAndSwapAsyncContextVariable(ctx, insertWrite); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSwapAsyncContextVariable(ctx, insertWrite); !repository.IsConflict(err) {
		t.Fatalf("duplicate insert=%v", err)
	}
	hydrated, err := store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, "progress")
	if err != nil || hydrated.Version() != 1 {
		t.Fatalf("inserted=%d/%s/%v", hydrated.Version(), hydrated.Value().String(), err)
	}
	requireSemanticJSONEqual(t, hydrated.Value().Bytes(), []byte(`{"step":1}`))
	updated := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "progress", `{"step":2}`, 2, now, now.Add(time.Second))
	updateWrite, _ := repository.NewAsyncContextWrite(updated, 1)
	if err := store.CompareAndSwapAsyncContextVariable(ctx, updateWrite); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSwapAsyncContextVariable(ctx, updateWrite); !repository.IsStaleWrite(err) {
		t.Fatalf("stale update=%v", err)
	}
	hydrated, err = store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, "progress")
	if err != nil || hydrated.Version() != 2 {
		t.Fatalf("updated=%d/%s/%v", hydrated.Version(), hydrated.Value().String(), err)
	}
	requireSemanticJSONEqual(t, hydrated.Value().Bytes(), []byte(`{"step":2}`))
	if _, err := store.GetAsyncContextVariable(ctx, "other-company", scope.executionID, "progress"); !repository.IsNotFound(err) {
		t.Fatalf("cross-tenant get=%v", err)
	}
	staleDelete, _ := repository.NewAsyncContextDelete(scope.companyID, scope.executionID, "progress", 1, now.Add(2*time.Second))
	if err := store.DeleteAsyncContextVariable(ctx, staleDelete); !repository.IsStaleWrite(err) {
		t.Fatalf("stale delete=%v", err)
	}
	deletion, _ := repository.NewAsyncContextDelete(scope.companyID, scope.executionID, "progress", 2, now.Add(3*time.Second))
	if err := store.DeleteAsyncContextVariable(ctx, deletion); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, "progress"); !repository.IsNotFound(err) {
		t.Fatalf("deleted variable=%v", err)
	}
}

func TestAsyncContextStoreIntegrationConcurrentCASAndRollback(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerAsyncContextCleanup(t, store, scope.executionID.String())
	now := time.Now().UTC()
	base := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "counter", `0`, 1, now, now)
	baseWrite, _ := repository.NewAsyncContextWrite(base, 0)
	if err := store.CompareAndSwapAsyncContextVariable(ctx, baseWrite); err != nil {
		t.Fatal(err)
	}
	writes := make([]repository.AsyncContextWrite, 2)
	for index := range writes {
		candidate := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "counter", string(rune('1'+index)), 2, now, now.Add(time.Second))
		writes[index], _ = repository.NewAsyncContextWrite(candidate, 1)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, write := range writes {
		write := write
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- store.CompareAndSwapAsyncContextVariable(ctx, write)
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes, stale := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if repository.IsStaleWrite(err) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent CAS success=%d stale=%d", successes, stale)
	}
	hydrated, err := store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, "counter")
	if err != nil || hydrated.Version() != 2 {
		t.Fatalf("concurrent version=%d/%v", hydrated.Version(), err)
	}

	rollbackVariable := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "rollback", `true`, 1, now, now)
	rollbackWrite, _ := repository.NewAsyncContextWrite(rollbackVariable, 0)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareAndSwapAsyncContextVariable(ctx, tx, rollbackWrite); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, "rollback"); !repository.IsNotFound(err) {
		t.Fatalf("rollback variable=%v", err)
	}
}

func TestAsyncContextStoreIntegrationRejectsMalformedJSONAndForeignExecution(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerAsyncContextCleanup(t, store, scope.executionID.String())
	now := time.Now().UTC()
	foreign := asyncContextVariableFixture(t, "other-company", scope.executionID.String(), "foreign", `true`, 1, now, now)
	foreignWrite, _ := repository.NewAsyncContextWrite(foreign, 0)
	if err := store.CompareAndSwapAsyncContextVariable(ctx, foreignWrite); !repository.IsConflict(err) {
		t.Fatalf("foreign execution=%v", err)
	}
	_, err := store.pool.Exec(ctx, `INSERT INTO workflow_runtime.async_context_variables (`+asyncContextColumns+`) VALUES ($1,$2,$3,$4::jsonb,$5,$5,1)`, scope.companyID.String(), scope.executionID.String(), "malformed", `{"broken":`, now)
	if err == nil {
		t.Fatal("PostgreSQL accepted malformed persisted JSON")
	}
	if _, err := store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, "malformed"); !repository.IsNotFound(err) {
		t.Fatalf("malformed JSON left durable row=%v", err)
	}
}

func registerAsyncContextCleanup(t *testing.T, store *Store, executionID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = store.pool.Exec(ctx, `DELETE FROM workflow_runtime.async_context_variables WHERE workflow_execution_id=$1`, executionID)
	})
}

func requireSemanticJSONEqual(t *testing.T, actual, expected []byte) {
	t.Helper()
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("actual JSON is invalid: %v", err)
	}
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatalf("expected JSON is invalid: %v", err)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf("JSON values differ: actual=%s expected=%s", actual, expected)
	}
}

func TestAsyncContextStoreContractAndCASSQL(t *testing.T) {
	var implementation repository.AsyncContextStore = &Store{}
	if implementation == nil {
		t.Fatal("async context contract unavailable")
	}
	for _, sql := range []string{insertAsyncContextSQL, updateAsyncContextSQL, getAsyncContextSQL, deleteAsyncContextSQL} {
		if strings.Contains(strings.ToUpper(sql), "SELECT *") {
			t.Fatal("context SQL uses SELECT *")
		}
	}
	if !strings.Contains(updateAsyncContextSQL, "AND version=$7") || !strings.Contains(updateAsyncContextSQL, "SET value_json=$4::jsonb, updated_at=$5, version=$6") {
		t.Fatal("update SQL does not implement compare-and-swap")
	}
	if !strings.Contains(deleteAsyncContextSQL, "AND version=$4") {
		t.Fatal("delete SQL does not implement compare-and-swap")
	}
}

func TestScanAsyncContextVariableHydratesAndDefensivelyCopies(t *testing.T) {
	now := time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC)
	row := asyncContextTestRow{company: "company-1", execution: "execution-1", key: "progress", value: []byte(`{"step":2}`), created: now, updated: now.Add(time.Second), version: 2}
	variable, err := scanAsyncContextVariable(row)
	if err != nil || variable.Version() != 2 || variable.Key() != "progress" {
		t.Fatalf("variable=%v/%v", variable, err)
	}
	first := variable.Value().Bytes()
	first[0] = '['
	if string(variable.Value().Bytes()) != `{"step":2}` {
		t.Fatal("context value exposed mutable storage")
	}
	row.value = []byte(`{"step":`)
	if _, err := scanAsyncContextVariable(row); err == nil || strings.Contains(err.Error(), "step") {
		t.Fatalf("malformed value error=%v", err)
	}
}

func TestAsyncContextConstraintMapping(t *testing.T) {
	for _, test := range []struct{ code, constraint string }{
		{postgreSQLUniqueViolationCode, "async_context_variables_pk"},
		{postgreSQLForeignKeyViolationCode, "async_context_variables_execution_fk"},
		{postgreSQLCheckViolationCode, "async_context_variables_value_size"},
	} {
		cause := &pgconn.PgError{Code: test.code, ConstraintName: test.constraint}
		err := mapAsyncContextWriteError("insert", cause)
		if !repository.IsConflict(err) || !errors.Is(err, cause) {
			t.Fatalf("constraint=%s mapping=%v", test.constraint, err)
		}
	}
}

func asyncContextVariableFixture(t *testing.T, company, executionID, key, jsonValue string, version int64, created, updated time.Time) repository.AsyncContextVariable {
	t.Helper()
	value, err := runtime.NewRuntimeValue([]byte(jsonValue))
	if err != nil {
		t.Fatal(err)
	}
	variable, err := repository.NewAsyncContextVariable(repository.AsyncContextVariableParams{
		CompanyID: workflow.CompanyID(company), WorkflowExecutionID: execution.WorkflowExecutionID(executionID),
		Key: key, Value: value, CreatedAt: created, UpdatedAt: updated, Version: version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return variable
}

type asyncContextTestRow struct {
	company, execution, key string
	value                   []byte
	created, updated        time.Time
	version                 int64
	err                     error
}

func (row asyncContextTestRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 7 {
		return errors.New("unexpected async context scan count")
	}
	*dest[0].(*string) = row.company
	*dest[1].(*string) = row.execution
	*dest[2].(*string) = row.key
	*dest[3].(*[]byte) = append([]byte(nil), row.value...)
	*dest[4].(*time.Time) = row.created
	*dest[5].(*time.Time) = row.updated
	*dest[6].(*int64) = row.version
	return nil
}

func TestAsyncInputStoreIntegrationPayloadIdentityFanOutAndFanIn(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerAsyncInputCleanup(t, store, scope.executionID)
	extraSource := addAsyncInputNode(t, ctx, store, scope, "extra-source", "extra-source")
	extraTarget := addAsyncInputNode(t, ctx, store, scope, "extra-target", "extra-target")

	inline := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-b", 1, integrationInlinePayload(t, `{"kind":"inline"}`))
	artifact := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-a", 1, integrationArtifactPayload(t))
	if err := store.CreateAsyncNodeInput(ctx, inline); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAsyncNodeInput(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	inputs, err := store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, scope.resultNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || inputs[0].EdgeID() != "edge-a" || inputs[1].EdgeID() != "edge-b" {
		t.Fatalf("deterministic inputs=%v", asyncInputEdges(inputs))
	}
	if inputs[0].CompanyID() != scope.companyID || inputs[0].WorkflowExecutionID() != scope.executionID || inputs[0].SourceNodeID() != "command" || inputs[0].TargetNodeID() != "result" || inputs[0].SourceOutputPort() != "result" || inputs[0].TargetInputPort() != "input" || inputs[0].SourceAttempt() != 1 {
		t.Fatalf("hydrated identity is incomplete: %+v", inputs[0])
	}
	hydratedArtifact, exists := inputs[0].Payload().Artifact()
	if !exists || hydratedArtifact.ID() != "artifact-integration" || hydratedArtifact.Metadata()["scope"] != "integration" {
		t.Fatalf("artifact=%v/%t", hydratedArtifact, exists)
	}
	data, exists := inputs[1].Payload().InlineData()
	if !exists || string(data) != `{"kind":"inline"}` || inputs[1].Payload().Metadata()["scope"] != "integration" {
		t.Fatalf("inline=%q/%t/%v", data, exists, inputs[1].Payload().Metadata())
	}

	// Fan-out: one source result can durably route over separate edge identities.
	fanOut := integrationAsyncInput(t, scope, scope.commandNodeID, "command", extraTarget, "extra-target", "edge-c", 1, integrationInlinePayload(t, `{"kind":"fan-out"}`))
	if err := store.CreateAsyncNodeInput(ctx, fanOut); err != nil {
		t.Fatal(err)
	}
	fanOutInputs, err := store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, extraTarget)
	if err != nil || len(fanOutInputs) != 1 || fanOutInputs[0].EdgeID() != "edge-c" {
		t.Fatalf("fan-out=%v/%v", asyncInputEdges(fanOutInputs), err)
	}

	// Fan-in: a second source and edge can target the same node without overwrite.
	fanIn := integrationAsyncInput(t, scope, extraSource, "extra-source", scope.resultNodeID, "result", "edge-d", 1, integrationInlinePayload(t, `{"kind":"fan-in"}`))
	if err := store.CreateAsyncNodeInput(ctx, fanIn); err != nil {
		t.Fatal(err)
	}
	inputs, err = store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, scope.resultNodeID)
	if err != nil || len(inputs) != 3 || fmt.Sprint(asyncInputEdges(inputs)) != "[edge-a edge-b edge-d]" {
		t.Fatalf("fan-in order=%v/%v", asyncInputEdges(inputs), err)
	}

	otherTenant, err := store.ListAsyncNodeInputs(ctx, workflow.CompanyID("other-company"), scope.executionID, scope.resultNodeID)
	if err != nil || len(otherTenant) != 0 {
		t.Fatalf("cross-tenant list=%d/%v", len(otherTenant), err)
	}
}

func TestAsyncInputStoreIntegrationDuplicateAndGraphIdentityEnforcement(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerAsyncInputCleanup(t, store, scope.executionID)
	input := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-duplicate", 1, integrationInlinePayload(t, `{"value":1}`))
	if err := store.CreateAsyncNodeInput(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAsyncNodeInput(ctx, input); !repository.IsConflict(err) {
		t.Fatalf("duplicate logical input=%v", err)
	}

	encoded, err := encodeRuntimePayload(input.Payload())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.pool.Exec(ctx, `INSERT INTO workflow_runtime.async_node_inputs (`+asyncInputColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13)`,
		"different-id-"+scope.prefix, input.CompanyID().String(), input.WorkflowExecutionID().String(), input.TargetNodeExecutionID().String(), input.SourceNodeExecutionID().String(), input.TargetNodeID().String(), input.SourceNodeID().String(), input.EdgeID().String(), input.SourceOutputPort(), input.TargetInputPort(), input.SourceAttempt(), string(encoded), input.CreatedAt())
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "async_node_inputs_logical_uk" {
		t.Fatalf("logical unique constraint=%v", err)
	}

	graphTests := []struct {
		name   string
		mutate func(*repository.AsyncNodeInputParams)
	}{
		{"cross tenant", func(p *repository.AsyncNodeInputParams) { p.CompanyID = "other-company" }},
		{"invalid source execution", func(p *repository.AsyncNodeInputParams) { p.SourceNodeExecutionID = "missing-source" }},
		{"invalid target execution", func(p *repository.AsyncNodeInputParams) { p.TargetNodeExecutionID = "missing-target" }},
		{"source node mismatch", func(p *repository.AsyncNodeInputParams) { p.SourceNodeID = "wrong-source" }},
		{"target node mismatch", func(p *repository.AsyncNodeInputParams) { p.TargetNodeID = "wrong-target" }},
	}
	for index, test := range graphTests {
		t.Run(test.name, func(t *testing.T) {
			params := integrationAsyncInputParams(scope, scope.commandNodeID, "command", scope.resultNodeID, "result", fmt.Sprintf("edge-invalid-%d", index), 1, integrationInlinePayload(t, `{"invalid":true}`))
			test.mutate(&params)
			invalid, err := repository.NewAsyncNodeInput(params)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.CreateAsyncNodeInput(ctx, invalid); !repository.IsConflict(err) {
				t.Fatalf("graph identity=%v", err)
			}
		})
	}
}

func TestAsyncInputStoreIntegrationRollbackAndMalformedPersistence(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerAsyncInputCleanup(t, store, scope.executionID)
	input := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-rollback", 1, integrationInlinePayload(t, `{"rollback":true}`))
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createAsyncNodeInput(ctx, tx, input); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	inputs, err := store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, scope.resultNodeID)
	if err != nil || len(inputs) != 0 {
		t.Fatalf("rollback inputs=%d/%v", len(inputs), err)
	}
	if err := store.CreateAsyncNodeInput(ctx, input); err != nil {
		t.Fatalf("reinsert after rollback=%v", err)
	}
	_, err = store.pool.Exec(ctx, `UPDATE workflow_runtime.async_node_inputs SET payload_json='{"version":1,"source":"INLINE","contentType":"application/json","unknown":true}'::jsonb WHERE async_input_id=$1`, input.IdentityKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, scope.resultNodeID); err == nil || err.Error() != "PostgreSQL decode persisted runtime payload: operation failed" {
		t.Fatalf("malformed persisted payload=%v", err)
	}
}

func TestAsyncInputStoreIntegrationConcurrentIdempotencyAndDifferentEdges(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerAsyncInputCleanup(t, store, scope.executionID)
	input := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-concurrent", 1, integrationInlinePayload(t, `{"concurrent":true}`))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- store.CreateAsyncNodeInput(ctx, input)
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if repository.IsConflict(err) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("same logical concurrency success=%d conflict=%d", successes, conflicts)
	}

	differentEdges := []repository.AsyncNodeInput{
		integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-concurrent-a", 2, integrationInlinePayload(t, `{"edge":"a"}`)),
		integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-concurrent-b", 2, integrationInlinePayload(t, `{"edge":"b"}`)),
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	for _, value := range differentEdges {
		value := value
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- store.CreateAsyncNodeInput(ctx, value)
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	inputs, err := store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, scope.resultNodeID)
	if err != nil || len(inputs) != 3 {
		t.Fatalf("concurrent durable inputs=%d/%v", len(inputs), err)
	}
}

func integrationAsyncInput(
	t *testing.T,
	scope asyncPersistenceScope,
	sourceExecution execution.NodeExecutionID,
	sourceNode workflow.NodeID,
	targetExecution execution.NodeExecutionID,
	targetNode workflow.NodeID,
	edge string,
	attempt int16,
	payload runtime.Payload,
) repository.AsyncNodeInput {
	t.Helper()
	input, err := repository.NewAsyncNodeInput(integrationAsyncInputParams(scope, sourceExecution, sourceNode, targetExecution, targetNode, edge, attempt, payload))
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func integrationAsyncInputParams(
	scope asyncPersistenceScope,
	sourceExecution execution.NodeExecutionID,
	sourceNode workflow.NodeID,
	targetExecution execution.NodeExecutionID,
	targetNode workflow.NodeID,
	edge string,
	attempt int16,
	payload runtime.Payload,
) repository.AsyncNodeInputParams {
	return repository.AsyncNodeInputParams{
		CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID,
		TargetNodeExecutionID: targetExecution, SourceNodeExecutionID: sourceExecution,
		TargetNodeID: targetNode, SourceNodeID: sourceNode, EdgeID: workflow.EdgeID(edge),
		SourceOutputPort: "result", TargetInputPort: "input", SourceAttempt: attempt,
		Payload: payload, CreatedAt: time.Now().UTC(),
	}
}

func integrationInlinePayload(t *testing.T, data string) runtime.Payload {
	t.Helper()
	payload, err := runtime.NewInlinePayload(runtime.ContentTypeApplicationJSON, []byte(data), map[string]string{"scope": "integration"}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func integrationArtifactPayload(t *testing.T) runtime.Payload {
	t.Helper()
	artifact, err := runtime.NewArtifactReference("artifact-integration", "s3://integration/input", runtime.ContentTypeApplicationOctetStream, 128, "sha256:integration", map[string]string{"scope": "integration"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := runtime.NewArtifactPayload(artifact, map[string]string{"scope": "integration"})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func addAsyncInputNode(t *testing.T, ctx context.Context, store *Store, scope asyncPersistenceScope, suffix, nodeID string) execution.NodeExecutionID {
	t.Helper()
	id := execution.NodeExecutionID("node-" + suffix + "-" + scope.prefix)
	now := time.Now().UTC()
	_, err := store.pool.Exec(ctx, `INSERT INTO workflow_runtime.node_executions(node_execution_id,workflow_execution_id,company_id,node_id,plugin_type,plugin_version,status,attempt,created_at,updated_at) VALUES($1,$2,$3,$4,'core.test','1','PENDING',1,$5,$5)`, id.String(), scope.executionID.String(), scope.companyID.String(), nodeID, now)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func registerAsyncInputCleanup(t *testing.T, store *Store, executionID execution.WorkflowExecutionID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = store.pool.Exec(ctx, `DELETE FROM workflow_runtime.async_node_inputs WHERE workflow_execution_id=$1`, executionID.String())
	})
}

func asyncInputEdges(inputs []repository.AsyncNodeInput) []string {
	edges := make([]string, len(inputs))
	for index, input := range inputs {
		edges[index] = input.EdgeID().String()
	}
	return edges
}

func TestAsyncInputStoreContractAndSQLMapping(t *testing.T) {
	var implementation repository.AsyncInputStore = &Store{}
	if implementation == nil {
		t.Fatal("async input contract unavailable")
	}
	upperInsert := strings.ToUpper(insertAsyncInputSQL)
	upperList := strings.ToUpper(listAsyncInputsSQL)
	if strings.Contains(upperInsert, "SELECT *") || strings.Contains(upperList, "SELECT *") {
		t.Fatal("async input SQL uses SELECT *")
	}
	for _, fragment := range []string{
		"async_input_id",
		"target_node_execution_id",
		"source_node_execution_id",
		"edge_id",
		"source_output_port",
		"target_input_port",
		"source_attempt",
		"payload_json",
		"target.node_id=$6",
		"source.node_id=$7",
	} {
		if !strings.Contains(insertAsyncInputSQL, fragment) {
			t.Fatalf("insert SQL missing %q", fragment)
		}
	}
	if !strings.Contains(listAsyncInputsSQL, "WHERE company_id=$1 AND workflow_execution_id=$2 AND target_node_execution_id=$3") {
		t.Fatal("list SQL is not tenant/execution/target scoped")
	}
	if !strings.Contains(listAsyncInputsSQL, "ORDER BY edge_id, source_attempt, source_node_execution_id, async_input_id") {
		t.Fatal("list SQL ordering is not deterministic")
	}
}

func TestCreateAsyncNodeInputMapsIdentityPortsAttemptAndInlinePayload(t *testing.T) {
	input := unitAsyncNodeInput(t, unitInlinePayload(t), "edge-1", 3)
	db := &captureAsyncInputDatabase{tag: pgconn.NewCommandTag("INSERT 0 1")}
	if err := createAsyncNodeInput(context.Background(), db, input); err != nil {
		t.Fatal(err)
	}
	if db.sql != insertAsyncInputSQL || len(db.args) != 13 {
		t.Fatalf("captured SQL/args=%q/%d", db.sql, len(db.args))
	}
	want := []any{
		input.IdentityKey(),
		"company-1",
		"execution-1",
		"target-execution-1",
		"source-execution-1",
		"target-node",
		"source-node",
		"edge-1",
		"result",
		"input",
		int16(3),
	}
	for index, expected := range want {
		if db.args[index] != expected {
			t.Fatalf("arg[%d]=%v, want %v", index, db.args[index], expected)
		}
	}
	encoded, ok := db.args[11].(string)
	if !ok {
		t.Fatalf("payload argument type=%T", db.args[11])
	}
	payload, err := decodeRuntimePayload([]byte(encoded))
	if err != nil {
		t.Fatal(err)
	}
	data, inline := payload.InlineData()
	if !inline || string(data) != `{"value":1}` || payload.Metadata()["source"] != "unit" {
		t.Fatalf("inline payload=%q/%t/%v", data, inline, payload.Metadata())
	}
}

func TestScanAsyncNodeInputHydratesInlineAndArtifactPayloads(t *testing.T) {
	for name, payload := range map[string]runtime.Payload{
		"inline":   unitInlinePayload(t),
		"artifact": unitArtifactPayload(t),
	} {
		t.Run(name, func(t *testing.T) {
			input := unitAsyncNodeInput(t, payload, "edge-"+name, 1)
			encoded, err := encodeRuntimePayload(payload)
			if err != nil {
				t.Fatal(err)
			}
			row := asyncInputTestRow{values: asyncInputRowValues(input, encoded)}
			hydrated, err := scanAsyncNodeInput(row)
			if err != nil {
				t.Fatal(err)
			}
			if hydrated.IdentityKey() != input.IdentityKey() || hydrated.SourceAttempt() != 1 || hydrated.EdgeID() != input.EdgeID() {
				t.Fatalf("hydrated identity=%q/%s/%d", hydrated.IdentityKey(), hydrated.EdgeID(), hydrated.SourceAttempt())
			}
			if name == "artifact" {
				artifact, exists := hydrated.Payload().Artifact()
				if !exists || artifact.ID() != "artifact-1" || artifact.Metadata()["region"] != "unit" {
					t.Fatalf("artifact=%v/%t", artifact, exists)
				}
			}
		})
	}
}

func TestScanAsyncNodeInputRejectsMalformedPayloadAndMismatchedIdentity(t *testing.T) {
	input := unitAsyncNodeInput(t, unitInlinePayload(t), "edge-1", 1)
	values := asyncInputRowValues(input, []byte(`{"version":1,"source":"INLINE","unknown":true}`))
	if _, err := scanAsyncNodeInput(asyncInputTestRow{values: values}); err == nil || strings.Contains(err.Error(), "unknown") {
		t.Fatalf("malformed payload error=%v", err)
	}
	encoded, err := encodeRuntimePayload(input.Payload())
	if err != nil {
		t.Fatal(err)
	}
	values = asyncInputRowValues(input, encoded)
	values.asyncInputID = "wrong-id"
	if _, err := scanAsyncNodeInput(asyncInputTestRow{values: values}); err == nil || !strings.Contains(err.Error(), "operation failed") {
		t.Fatalf("identity error=%v", err)
	}
}

func TestAsyncInputConstraintErrorsAndGraphMismatchAreConflicts(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		resource   string
	}{
		{"duplicate ID", "async_node_inputs_pk", "input ID"},
		{"duplicate logical", "async_node_inputs_logical_uk", "logical identity"},
		{"foreign key", "async_node_inputs_target_fk", "async node input"},
		{"check", "async_node_inputs_attempt_positive", "async node input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code := postgreSQLUniqueViolationCode
			if strings.Contains(test.constraint, "_fk") {
				code = postgreSQLForeignKeyViolationCode
			}
			if strings.Contains(test.constraint, "attempt") {
				code = postgreSQLCheckViolationCode
			}
			cause := &pgconn.PgError{Code: code, ConstraintName: test.constraint}
			err := mapAsyncInputWriteError("create", cause)
			if !repository.IsConflict(err) || !errors.Is(err, cause) || !strings.Contains(err.Error(), test.resource) {
				t.Fatalf("mapping=%v", err)
			}
		})
	}
	db := &captureAsyncInputDatabase{tag: pgconn.NewCommandTag("INSERT 0 0")}
	err := createAsyncNodeInput(context.Background(), db, unitAsyncNodeInput(t, unitInlinePayload(t), "edge-1", 1))
	if !repository.IsConflict(err) || !strings.Contains(err.Error(), "graph identity") {
		t.Fatalf("graph mismatch=%v", err)
	}
}

func unitAsyncNodeInput(t *testing.T, payload runtime.Payload, edge string, attempt int16) repository.AsyncNodeInput {
	t.Helper()
	input, err := repository.NewAsyncNodeInput(repository.AsyncNodeInputParams{
		CompanyID:             workflow.CompanyID("company-1"),
		WorkflowExecutionID:   execution.WorkflowExecutionID("execution-1"),
		TargetNodeExecutionID: execution.NodeExecutionID("target-execution-1"),
		SourceNodeExecutionID: execution.NodeExecutionID("source-execution-1"),
		TargetNodeID:          workflow.NodeID("target-node"),
		SourceNodeID:          workflow.NodeID("source-node"),
		EdgeID:                workflow.EdgeID(edge),
		SourceOutputPort:      "result",
		TargetInputPort:       "input",
		SourceAttempt:         attempt,
		Payload:               payload,
		CreatedAt:             time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func unitInlinePayload(t *testing.T) runtime.Payload {
	t.Helper()
	payload, err := runtime.NewInlinePayload(runtime.ContentTypeApplicationJSON, []byte(`{"value":1}`), map[string]string{"source": "unit"}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func unitArtifactPayload(t *testing.T) runtime.Payload {
	t.Helper()
	artifact, err := runtime.NewArtifactReference("artifact-1", "s3://unit/input", runtime.ContentTypeApplicationOctetStream, 42, "sha256:unit", map[string]string{"region": "unit"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := runtime.NewArtifactPayload(artifact, map[string]string{"source": "artifact"})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type captureAsyncInputDatabase struct {
	sql  string
	args []any
	tag  pgconn.CommandTag
	err  error
}

func (db *captureAsyncInputDatabase) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.sql, db.args = sql, args
	return db.tag, db.err
}

func (*captureAsyncInputDatabase) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query")
}

func (*captureAsyncInputDatabase) QueryRow(context.Context, string, ...any) pgx.Row {
	return asyncInputTestRow{err: errors.New("unexpected QueryRow")}
}

type asyncInputTestRowValues struct {
	asyncInputID, companyID, workflowExecutionID string
	targetNodeExecutionID, sourceNodeExecutionID string
	targetNodeID, sourceNodeID, edgeID           string
	sourceOutputPort, targetInputPort            string
	sourceAttempt                                int16
	payload                                      []byte
	createdAt                                    time.Time
}

func asyncInputRowValues(input repository.AsyncNodeInput, payload []byte) asyncInputTestRowValues {
	return asyncInputTestRowValues{
		asyncInputID: input.IdentityKey(), companyID: input.CompanyID().String(), workflowExecutionID: input.WorkflowExecutionID().String(),
		targetNodeExecutionID: input.TargetNodeExecutionID().String(), sourceNodeExecutionID: input.SourceNodeExecutionID().String(),
		targetNodeID: input.TargetNodeID().String(), sourceNodeID: input.SourceNodeID().String(), edgeID: input.EdgeID().String(),
		sourceOutputPort: input.SourceOutputPort(), targetInputPort: input.TargetInputPort(), sourceAttempt: input.SourceAttempt(),
		payload: append([]byte(nil), payload...), createdAt: input.CreatedAt(),
	}
}

type asyncInputTestRow struct {
	values asyncInputTestRowValues
	err    error
}

func (row asyncInputTestRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 13 {
		return errors.New("unexpected async input scan destination count")
	}
	*dest[0].(*string) = row.values.asyncInputID
	*dest[1].(*string) = row.values.companyID
	*dest[2].(*string) = row.values.workflowExecutionID
	*dest[3].(*string) = row.values.targetNodeExecutionID
	*dest[4].(*string) = row.values.sourceNodeExecutionID
	*dest[5].(*string) = row.values.targetNodeID
	*dest[6].(*string) = row.values.sourceNodeID
	*dest[7].(*string) = row.values.edgeID
	*dest[8].(*string) = row.values.sourceOutputPort
	*dest[9].(*string) = row.values.targetInputPort
	*dest[10].(*int16) = row.values.sourceAttempt
	*dest[11].(*[]byte) = append([]byte(nil), row.values.payload...)
	*dest[12].(*time.Time) = row.values.createdAt
	return nil
}

type asyncPersistenceScope struct {
	companyID     workflow.CompanyID
	executionID   execution.WorkflowExecutionID
	commandNodeID execution.NodeExecutionID
	resultNodeID  execution.NodeExecutionID
	prefix        string
}

func requireAsyncPersistenceStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	ctx, store := requirePostgreSQLIntegrationStore(t)
	requirePostgreSQLIntegrationTable(t, ctx, store, "outbox_messages")
	requirePostgreSQLIntegrationTable(t, ctx, store, "inbox_messages")
	var version int64
	if err := store.pool.QueryRow(ctx, `SELECT version_id FROM public.workflow_runtime_goose_db_version WHERE is_applied ORDER BY id DESC LIMIT 1`).Scan(&version); err != nil || version != 6 {
		t.Fatalf("migration version=%d error=%v, want 6", version, err)
	}
	return ctx, store
}

func createAsyncPersistenceScope(t *testing.T, ctx context.Context, store *Store, prefix string) asyncPersistenceScope {
	t.Helper()
	now := time.Now().UTC()
	company := "company-" + prefix
	snapshot := "snapshot-" + prefix
	workflowID := "workflow-" + prefix
	executionID := "execution-" + prefix
	commandNode := "node-command-" + prefix
	resultNode := "node-result-" + prefix
	_, err := store.pool.Exec(ctx, `INSERT INTO workflow_runtime.workflow_definition_snapshots(snapshot_id,company_id,workflow_id,workflow_revision,workflow_name,definition_json,created_at) VALUES($1,$2,$3,1,$4,'{}'::jsonb,$5)`, snapshot, company, workflowID, "Async "+prefix, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.pool.Exec(ctx, `INSERT INTO workflow_runtime.workflow_executions(workflow_execution_id,company_id,workflow_id,workflow_revision,snapshot_id,mode,status,created_at,updated_at) VALUES($1,$2,$3,1,$4,'ASYNC','CREATED',$5,$5)`, executionID, company, workflowID, snapshot, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.pool.Exec(ctx, `INSERT INTO workflow_runtime.node_executions(node_execution_id,workflow_execution_id,company_id,node_id,plugin_type,plugin_version,status,attempt,created_at,updated_at) VALUES($1,$2,$3,$4,'core.test','1','PENDING',1,$6,$6),($5,$2,$3,$7,'core.test','1','PENDING',1,$6,$6)`, commandNode, executionID, company, "command", resultNode, now, "result")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.outbox_messages WHERE workflow_execution_id=$1`, executionID)
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.inbox_messages WHERE workflow_execution_id=$1`, executionID)
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.async_node_inputs WHERE workflow_execution_id=$1`, executionID)
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.async_worker_results WHERE workflow_execution_id=$1`, executionID)
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.async_context_variables WHERE workflow_execution_id=$1`, executionID)
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.node_executions WHERE workflow_execution_id=$1`, executionID)
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.workflow_executions WHERE workflow_execution_id=$1`, executionID)
		_, _ = store.pool.Exec(cleanupCtx, `DELETE FROM workflow_runtime.workflow_definition_snapshots WHERE snapshot_id=$1`, snapshot)
	})
	return asyncPersistenceScope{workflow.CompanyID(company), execution.WorkflowExecutionID(executionID), execution.NodeExecutionID(commandNode), execution.NodeExecutionID(resultNode), prefix}
}

func uniqueAsyncPrefix(t *testing.T) string { return fmt.Sprintf("%d", time.Now().UnixNano()) }

func TestAsyncPersistenceTransactionIntegrationWorkerResultAndOutboxCommitRollback(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	now := time.Now().UTC()
	committedResult := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.resultNodeID.String(), "result", 30, repository.DurableWorkerResultSucceeded, false)
	committedOutbox := newIntegrationOutboxMessage(t, scope, "tx-result-commit", repository.OutboxOperationNodeResult, now)
	if err := store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		if err := tx.CreateDurableWorkerResult(ctx, committedResult); err != nil {
			return err
		}
		return tx.CreateOutboxMessage(ctx, committedOutbox)
	}); err != nil {
		t.Fatal(err)
	}
	assertWorkerResultAndOutboxVisibility(t, ctx, store, scope, committedResult, committedOutbox, true)

	rollbackResult := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.resultNodeID.String(), "result", 31, repository.DurableWorkerResultFailed, false)
	rollbackOutbox := newIntegrationOutboxMessage(t, scope, "tx-result-rollback", repository.OutboxOperationNodeCommand, now.Add(time.Second))
	rollbackCause := errors.New("force worker result rollback")
	err := store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		if err := tx.CreateDurableWorkerResult(ctx, rollbackResult); err != nil {
			return err
		}
		if err := tx.CreateOutboxMessage(ctx, rollbackOutbox); err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("rollback error=%v", err)
	}
	assertWorkerResultAndOutboxVisibility(t, ctx, store, scope, rollbackResult, rollbackOutbox, false)
}

func TestAsyncPersistenceTransactionIntegrationEngineEffectsCommitRollback(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	now := time.Now().UTC()
	commitInbox := newIntegrationInboxMessage(t, scope, "tx-engine", "commit", repository.InboxProcessingApplied, nil, now)
	commitInput := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-tx-commit", 1, integrationInlinePayload(t, `{"tx":"commit"}`))
	commitContext := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "tx-commit", `{"committed":true}`, 1, now, now)
	commitWrite, _ := repository.NewAsyncContextWrite(commitContext, 0)
	commitOutbox := newIntegrationOutboxMessage(t, scope, "tx-engine-commit", repository.OutboxOperationNodeCommand, now)
	if err := store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		if err := tx.RecordInboxMessage(ctx, commitInbox); err != nil {
			return err
		}
		if err := tx.CreateAsyncNodeInput(ctx, commitInput); err != nil {
			return err
		}
		if err := tx.CompareAndSwapAsyncContextVariable(ctx, commitWrite); err != nil {
			return err
		}
		return tx.CreateOutboxMessage(ctx, commitOutbox)
	}); err != nil {
		t.Fatal(err)
	}
	assertEngineEffectsVisibility(t, ctx, store, scope, commitInbox, commitInput, commitContext.Key(), commitOutbox, true)

	rollbackInbox := newIntegrationInboxMessage(t, scope, "tx-engine", "rollback", repository.InboxProcessingIgnored, nil, now)
	rollbackInput := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-tx-rollback", 1, integrationInlinePayload(t, `{"tx":"rollback"}`))
	rollbackContext := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "tx-rollback", `{"committed":false}`, 1, now, now)
	rollbackWrite, _ := repository.NewAsyncContextWrite(rollbackContext, 0)
	rollbackOutbox := newIntegrationOutboxMessage(t, scope, "tx-engine-rollback", repository.OutboxOperationNodeResult, now.Add(time.Second))
	rollbackCause := errors.New("force engine effects rollback")
	err := store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		if err := tx.RecordInboxMessage(ctx, rollbackInbox); err != nil {
			return err
		}
		if err := tx.CreateAsyncNodeInput(ctx, rollbackInput); err != nil {
			return err
		}
		if err := tx.CompareAndSwapAsyncContextVariable(ctx, rollbackWrite); err != nil {
			return err
		}
		if err := tx.CreateOutboxMessage(ctx, rollbackOutbox); err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("rollback error=%v", err)
	}
	assertEngineEffectsVisibility(t, ctx, store, scope, rollbackInbox, rollbackInput, rollbackContext.Key(), rollbackOutbox, false)
}

func TestAsyncPersistenceTransactionIntegrationConcurrentDuplicateProtection(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	now := time.Now().UTC()
	result := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.resultNodeID.String(), "result", 40, repository.DurableWorkerResultSucceeded, false)
	outbox := newIntegrationOutboxMessage(t, scope, "tx-concurrent", repository.OutboxOperationNodeResult, now)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
				if err := tx.CreateDurableWorkerResult(ctx, result); err != nil {
					return err
				}
				return tx.CreateOutboxMessage(ctx, outbox)
			})
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if repository.IsConflict(err) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent transaction success=%d conflict=%d", successes, conflicts)
	}
	assertWorkerResultAndOutboxVisibility(t, ctx, store, scope, result, outbox, true)
}

func TestAsyncPersistenceTransactionIntegrationContextConflictRollsBackAllEffects(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	now := time.Now().UTC()
	base := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "conflict", `1`, 1, now, now)
	baseWrite, _ := repository.NewAsyncContextWrite(base, 0)
	if err := store.CompareAndSwapAsyncContextVariable(ctx, baseWrite); err != nil {
		t.Fatal(err)
	}
	current := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "conflict", `2`, 2, now, now.Add(time.Second))
	currentWrite, _ := repository.NewAsyncContextWrite(current, 1)
	if err := store.CompareAndSwapAsyncContextVariable(ctx, currentWrite); err != nil {
		t.Fatal(err)
	}

	result := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.resultNodeID.String(), "result", 50, repository.DurableWorkerResultSucceeded, false)
	inbox := newIntegrationInboxMessage(t, scope, "tx-conflict", "message", repository.InboxProcessingApplied, nil, now)
	input := integrationAsyncInput(t, scope, scope.commandNodeID, "command", scope.resultNodeID, "result", "edge-tx-conflict", 1, integrationInlinePayload(t, `{"conflict":true}`))
	outbox := newIntegrationOutboxMessage(t, scope, "tx-conflict", repository.OutboxOperationNodeCommand, now)
	staleVariable := asyncContextVariableFixture(t, scope.companyID.String(), scope.executionID.String(), "conflict", `3`, 2, now, now.Add(2*time.Second))
	staleWrite, _ := repository.NewAsyncContextWrite(staleVariable, 1)
	err := store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		if err := tx.CreateDurableWorkerResult(ctx, result); err != nil {
			return err
		}
		if err := tx.RecordInboxMessage(ctx, inbox); err != nil {
			return err
		}
		if err := tx.CreateAsyncNodeInput(ctx, input); err != nil {
			return err
		}
		if err := tx.CreateOutboxMessage(ctx, outbox); err != nil {
			return err
		}
		return tx.CompareAndSwapAsyncContextVariable(ctx, staleWrite)
	})
	if !repository.IsStaleWrite(err) {
		t.Fatalf("context conflict=%v", err)
	}
	assertWorkerResultAndOutboxVisibility(t, ctx, store, scope, result, outbox, false)
	processed, checkErr := store.HasProcessedInboxMessage(ctx, inbox.ConsumerIdentity(), inbox.MessageID())
	if checkErr != nil || processed {
		t.Fatalf("conflict inbox=%t/%v", processed, checkErr)
	}
	inputs, checkErr := store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, scope.resultNodeID)
	if checkErr != nil || len(inputs) != 0 {
		t.Fatalf("conflict inputs=%d/%v", len(inputs), checkErr)
	}
	persisted, checkErr := store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, "conflict")
	if checkErr != nil || persisted.Version() != 2 {
		t.Fatalf("context after conflict=%d/%v", persisted.Version(), checkErr)
	}
}

func assertWorkerResultAndOutboxVisibility(t *testing.T, ctx context.Context, store *Store, scope asyncPersistenceScope, result repository.DurableWorkerResult, outbox repository.OutboxMessage, visible bool) {
	t.Helper()
	_, resultErr := store.GetDurableWorkerResult(ctx, scope.companyID, scope.executionID, result.NodeExecutionID(), result.Attempt())
	var outboxCount int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_runtime.outbox_messages WHERE message_id=$1`, outbox.MessageID().String()).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if visible && (resultErr != nil || outboxCount != 1) {
		t.Fatalf("visible result/outbox=%v/%d", resultErr, outboxCount)
	}
	if !visible && (!repository.IsNotFound(resultErr) || outboxCount != 0) {
		t.Fatalf("rolled back result/outbox=%v/%d", resultErr, outboxCount)
	}
}

func assertEngineEffectsVisibility(t *testing.T, ctx context.Context, store *Store, scope asyncPersistenceScope, inbox repository.InboxMessage, input repository.AsyncNodeInput, contextKey string, outbox repository.OutboxMessage, visible bool) {
	t.Helper()
	processed, err := store.HasProcessedInboxMessage(ctx, inbox.ConsumerIdentity(), inbox.MessageID())
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := store.ListAsyncNodeInputs(ctx, scope.companyID, scope.executionID, input.TargetNodeExecutionID())
	if err != nil {
		t.Fatal(err)
	}
	_, contextErr := store.GetAsyncContextVariable(ctx, scope.companyID, scope.executionID, contextKey)
	var outboxCount int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_runtime.outbox_messages WHERE message_id=$1`, outbox.MessageID().String()).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	inputPresent := false
	for _, candidate := range inputs {
		if candidate.IdentityKey() == input.IdentityKey() {
			inputPresent = true
			break
		}
	}
	if visible && (!processed || !inputPresent || contextErr != nil || outboxCount != 1) {
		t.Fatalf("visible engine effects inbox=%t inputs=%d inputPresent=%t context=%v outbox=%d", processed, len(inputs), inputPresent, contextErr, outboxCount)
	}
	if !visible && (processed || inputPresent || !repository.IsNotFound(contextErr) || outboxCount != 0) {
		t.Fatalf("rolled back engine effects inbox=%t inputs=%d inputPresent=%t context=%v outbox=%d", processed, len(inputs), inputPresent, contextErr, outboxCount)
	}
}

func TestAsyncPersistenceTransactionContractsAreImplemented(t *testing.T) {
	var transactor repository.AsyncPersistenceTransactor = &Store{}
	var transaction repository.AsyncPersistenceTransaction = &asyncPersistenceTransaction{}
	if transactor == nil || transaction == nil {
		t.Fatal("async persistence transaction contracts unavailable")
	}
	if err := (&Store{}).WithinAsyncPersistenceTransaction(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "work") {
		t.Fatalf("nil work validation=%v", err)
	}
}

func TestAsyncRuntimeCodecPayloadAndArtifactRoundTrips(t *testing.T) {
	inline, err := runtime.NewInlinePayload(runtime.ContentTypeApplicationJSON, []byte(`{"amount":7}`), map[string]string{"source": "test"}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeRuntimePayload(inline)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeRuntimePayload(encoded)
	if err != nil {
		t.Fatal(err)
	}
	data, ok := decoded.InlineData()
	if !ok || string(data) != `{"amount":7}` || decoded.Metadata()["source"] != "test" {
		t.Fatal("inline payload round trip mismatch")
	}
	encoded[0] = 'X'
	data[0] = 'X'
	second, _ := decoded.InlineData()
	if string(second) != `{"amount":7}` {
		t.Fatal("payload did not defensively copy data")
	}

	artifact, err := runtime.NewArtifactReference("artifact-1", "s3://bucket/key", runtime.ContentTypeApplicationOctetStream, 42, "sha256:value", map[string]string{"region": "local"})
	if err != nil {
		t.Fatal(err)
	}
	artifactPayload, err := runtime.NewArtifactPayload(artifact, map[string]string{"purpose": "test"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = encodeRuntimePayload(artifactPayload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = decodeRuntimePayload(encoded)
	if err != nil {
		t.Fatal(err)
	}
	actualArtifact, ok := decoded.Artifact()
	if !ok || actualArtifact.ID() != "artifact-1" || actualArtifact.Location() != "s3://bucket/key" || actualArtifact.SizeBytes() != 42 {
		t.Fatal("artifact round trip mismatch")
	}
}

func TestAsyncRuntimeCodecNodeContentRoundTrips(t *testing.T) {
	payload, _ := runtime.NewInlinePayload(runtime.ContentTypeTextPlain, []byte("value"), nil, 1024)
	value, _ := runtime.NewRuntimeValue([]byte(`{"enabled":true}`))
	changes, _ := runtime.NewContextChanges(map[string]runtime.RuntimeValue{"state": value}, []string{"obsolete"})
	outputs := map[string][]runtime.Payload{"success": {payload}}
	encodedOutputs, err := encodeRoutedOutputs(outputs)
	if err != nil {
		t.Fatal(err)
	}
	decodedOutputs, err := decodeRoutedOutputs(encodedOutputs)
	if err != nil || len(decodedOutputs["success"]) != 1 {
		t.Fatalf("routed outputs round trip: %v", err)
	}
	encodedChanges, err := encodeContextChanges(changes)
	if err != nil {
		t.Fatal(err)
	}
	decodedChanges, err := decodeContextChanges(encodedChanges)
	if err != nil || decodedChanges.IsEmpty() {
		t.Fatalf("context changes round trip: %v", err)
	}

	failure, _ := runtime.NewRuntimeFailure(runtime.FailureCategoryDependency, "DEPENDENCY_DOWN", "dependency unavailable", true, map[string]string{"safe": "detail"})
	encodedFailure, err := encodeRuntimeFailure(failure)
	if err != nil {
		t.Fatal(err)
	}
	decodedFailure, err := decodeRuntimeFailure(encodedFailure)
	if err != nil || decodedFailure.Category() != failure.Category() || decodedFailure.Code() != failure.Code() {
		t.Fatalf("failure round trip: %v", err)
	}

	routed, _ := runtime.NewNodeSuccessResult(outputs, changes)
	encodedResult, err := encodeNodeResult(routed)
	if err != nil {
		t.Fatal(err)
	}
	decodedResult, err := decodeNodeResult(encodedResult)
	if err != nil || !decodedResult.HasRoutedOutputs() {
		t.Fatalf("routed result round trip: %v", err)
	}
	terminal, _ := runtime.NewTerminalNodeSuccessResult(payload, changes)
	encodedResult, err = encodeNodeResult(terminal)
	if err != nil {
		t.Fatal(err)
	}
	decodedResult, err = decodeNodeResult(encodedResult)
	if err != nil || !decodedResult.HasTerminalOutput() {
		t.Fatalf("terminal result round trip: %v", err)
	}
	failed, _ := runtime.NewNodeFailureResult(failure)
	encodedResult, err = encodeNodeResult(failed)
	if err != nil {
		t.Fatal(err)
	}
	decodedResult, err = decodeNodeResult(encodedResult)
	if err != nil || !decodedResult.IsFailure() {
		t.Fatalf("failed result round trip: %v", err)
	}
}

func TestAsyncRuntimeCodecRejectsUnsafePersistedData(t *testing.T) {
	tests := map[string][]byte{
		"malformed":           []byte(`{"version":1`),
		"unknown field":       []byte(`{"version":1,"source":"INLINE","contentType":"text/plain","unknown":true}`),
		"unsupported version": []byte(`{"version":2,"source":"INLINE","contentType":"text/plain"}`),
		"zero":                nil,
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeRuntimePayload(encoded); err == nil {
				t.Fatal("unsafe persisted payload accepted")
			} else {
				var safe *databaseError
				if !errors.As(err, &safe) {
					t.Fatalf("error type %T, want safe databaseError", err)
				}
			}
		})
	}
	oversized := bytes.Repeat([]byte{'x'}, repository.MaximumAsyncEncodedPayloadBytes+1)
	if _, err := decodeRuntimePayload(oversized); err == nil {
		t.Fatal("oversized payload accepted")
	}
	if _, err := encodeRuntimePayload(runtime.Payload{}); err == nil {
		t.Fatal("zero payload encoded")
	}
	if _, err := encodeNodeResult(runtime.NodeResult{}); err == nil {
		t.Fatal("zero result encoded")
	}
	if _, err := decodeNodeResult([]byte(`{"version":1,"status":"UNKNOWN"}`)); err == nil {
		t.Fatal("unsupported result status accepted")
	}
	if _, err := decodeRuntimeFailure([]byte(`{"version":1,"category":"UNKNOWN","code":"x","message":"x","retryable":false}`)); err == nil {
		t.Fatal("unsupported failure category accepted")
	}
}

func TestAsyncRuntimeCodecErrorsDoNotExposePersistedContent(t *testing.T) {
	secret := "secret-database-content"
	_, err := decodeRuntimePayload([]byte(`{"version":1,"` + secret + `":true}`))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("safe error exposed persisted content: %v", err)
	}
}

func TestAsyncWorkerFinalizationCheckpointSurvivesRollbackAndReplay(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	now := time.Now().UTC()
	if _, err := store.pool.Exec(ctx, `UPDATE workflow_runtime.node_executions SET status='QUEUED', ready_at=$2, queued_at=$2, updated_at=$2 WHERE node_execution_id=$1`, scope.commandNodeID.String(), now); err != nil {
		t.Fatal(err)
	}
	var runningVersion int64
	if err := store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		var err error
		runningVersion, err = tx.ClaimAsyncNodeExecution(ctx, repository.AsyncNodeClaim{CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID, NodeExecutionID: scope.commandNodeID, ExpectedLockVersion: 0, StartedAt: now})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	checkpoint := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 1, repository.DurableWorkerResultSucceeded, false)
	checkpointResult, _ := checkpoint.Result()
	checkpoint, err := repository.NewDurableWorkerResult(repository.DurableWorkerResultParams{CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID, NodeExecutionID: scope.commandNodeID, NodeID: "command", Attempt: 1, Status: repository.DurableWorkerResultSucceeded, Result: checkpointResult, StartedAt: now, FinishedAt: now.Add(time.Second), CreatedAt: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDurableWorkerResult(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	outbox := newIntegrationOutboxMessage(t, scope, "continuation-result", repository.OutboxOperationNodeResult, checkpoint.FinishedAt())
	inbox := newIntegrationInboxMessage(t, scope, "worker-continuation", "continuation-command", repository.InboxProcessingApplied, nil, checkpoint.FinishedAt())
	completion := repository.AsyncNodeCompletion{CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID, NodeExecutionID: scope.commandNodeID, ExpectedLockVersion: runningVersion, Status: execution.NodeExecutionStatusSucceeded, FinishedAt: checkpoint.FinishedAt(), OutputSummary: []byte(`{}`), FailureSummary: []byte(`{}`), Result: checkpoint, OutboxMessage: outbox}
	finalization := repository.AsyncNodeFinalization{Completion: completion, InboxMessage: inbox}
	rollback := errors.New("force finalization rollback")
	err = store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		if err := tx.FinalizeAsyncNodeExecution(ctx, finalization); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error=%v", err)
	}
	assertContinuationState(t, ctx, store, scope, checkpoint, outbox, inbox, "RUNNING", 0, false)
	if err := store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		return tx.FinalizeAsyncNodeExecution(ctx, finalization)
	}); err != nil {
		t.Fatal(err)
	}
	assertContinuationState(t, ctx, store, scope, checkpoint, outbox, inbox, "SUCCEEDED", 1, true)
}

func TestAsyncNodeExecutionSessionLockSerializesLogicalAttempt(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	lock := repository.AsyncNodeExecutionLock{CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID, NodeExecutionID: scope.commandNodeID, Attempt: 1}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{})
	errorsCh := make(chan error, 2)
	go func() {
		errorsCh <- store.WithAsyncNodeExecutionLock(ctx, lock, func(context.Context) error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered
	go func() {
		errorsCh <- store.WithAsyncNodeExecutionLock(ctx, lock, func(context.Context) error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("duplicate logical attempt entered while first session lock was held")
	case <-time.After(200 * time.Millisecond):
	}
	close(releaseFirst)
	select {
	case <-secondEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("second logical attempt did not continue after lock release")
	}
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		if err := <-errorsCh; err != nil {
			t.Errorf("lock error=%v", err)
		}
	}()
	go func() {
		defer wait.Done()
		if err := <-errorsCh; err != nil {
			t.Errorf("lock error=%v", err)
		}
	}()
	wait.Wait()
}

func assertContinuationState(t *testing.T, ctx context.Context, store *Store, scope asyncPersistenceScope, checkpoint repository.DurableWorkerResult, outbox repository.OutboxMessage, inbox repository.InboxMessage, wantStatus string, wantOutbox int, wantInbox bool) {
	t.Helper()
	var status string
	if err := store.pool.QueryRow(ctx, `SELECT status FROM workflow_runtime.node_executions WHERE node_execution_id=$1`, scope.commandNodeID.String()).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus {
		t.Fatalf("node status=%s want=%s", status, wantStatus)
	}
	if _, err := store.GetDurableWorkerResult(ctx, scope.companyID, scope.executionID, checkpoint.NodeExecutionID(), checkpoint.Attempt()); err != nil {
		t.Fatalf("checkpoint missing: %v", err)
	}
	var outboxCount int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_runtime.outbox_messages WHERE message_id=$1`, outbox.MessageID().String()).Scan(&outboxCount); err != nil || outboxCount != wantOutbox {
		t.Fatalf("outbox=%d/%v want=%d", outboxCount, err, wantOutbox)
	}
	processed, err := store.HasProcessedInboxMessage(ctx, inbox.ConsumerIdentity(), inbox.MessageID())
	if err != nil || processed != wantInbox {
		t.Fatalf("inbox=%t/%v want=%t", processed, err, wantInbox)
	}
}

func TestAsyncWorkerExecutionStoreIntegrationConcurrentClaim(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	now := time.Now().UTC()
	_, err := store.pool.Exec(ctx, `UPDATE workflow_runtime.node_executions SET status='QUEUED', ready_at=$1, queued_at=$1 WHERE node_execution_id=$2`, now, scope.commandNodeID.String())
	if err != nil {
		t.Fatal(err)
	}
	results := make([]error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index] = store.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
				_, err := tx.ClaimAsyncNodeExecution(ctx, repository.AsyncNodeClaim{CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID, NodeExecutionID: scope.commandNodeID, ExpectedLockVersion: 0, StartedAt: now})
				return err
			})
		}(i)
	}
	wait.Wait()
	winners := 0
	for _, err := range results {
		if err == nil {
			winners++
		} else if !repository.IsStaleWrite(err) {
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners=%d, want 1", winners)
	}
}

func TestCreateExecutionIntegration(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(t)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"workflow_executions",
	)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"execution_events",
	)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"execution_logs",
	)

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	successFixture :=
		newCreateExecutionIntegrationFixture(
			t,
			"success-"+suffix,
			"",
		)

	rollbackHolder :=
		newCreateExecutionIntegrationFixture(
			t,
			"rollback-holder-"+suffix,
			"",
		)

	duplicateLogID :=
		repository.ExecutionLogID(
			"shared-rollback-log-" + suffix,
		)

	rollbackTarget :=
		newCreateExecutionIntegrationFixture(
			t,
			"rollback-target-"+suffix,
			duplicateLogID.String(),
		)

	cleanupCompanies := []workflow.CompanyID{
		successFixture.companyID,
		rollbackHolder.companyID,
		rollbackTarget.companyID,
	}

	t.Cleanup(func() {
		cleanupContext, cancelCleanup :=
			context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
		defer cancelCleanup()

		for _, companyID := range cleanupCompanies {
			cleanupCreateExecutionIntegrationCompany(
				t,
				cleanupContext,
				store,
				companyID,
			)
		}
	})

	t.Run(
		"commits snapshot workflow event and logs atomically",
		func(t *testing.T) {
			err := store.CreateExecution(
				ctx,
				successFixture.command,
			)
			if err != nil {
				t.Fatalf(
					"CreateExecution() returned an error: %v",
					err,
				)
			}

			actualSnapshot, err :=
				store.GetByID(
					ctx,
					successFixture.companyID,
					successFixture.snapshotID,
				)
			if err != nil {
				t.Fatalf(
					"GetByID() returned an error: %v",
					err,
				)
			}

			if actualSnapshot.ID() !=
				successFixture.snapshotID {
				t.Fatalf(
					"snapshot ID = %q, want %q",
					actualSnapshot.ID(),
					successFixture.snapshotID,
				)
			}

			actualWorkflow, err :=
				store.GetWorkflowExecution(
					ctx,
					successFixture.companyID,
					successFixture.
						workflowExecutionID,
				)
			if err != nil {
				t.Fatalf(
					"GetWorkflowExecution() returned an error: %v",
					err,
				)
			}

			if actualWorkflow.Status() !=
				execution.
					WorkflowExecutionStatusCreated {
				t.Fatalf(
					"workflow status = %q, want CREATED",
					actualWorkflow.Status(),
				)
			}

			if actualWorkflow.NextSequenceNumber() !=
				repository.SequenceNumber(4) {
				t.Fatalf(
					"next sequence number = %d, want 4",
					actualWorkflow.
						NextSequenceNumber(),
				)
			}

			if actualWorkflow.LockVersion() != 0 {
				t.Fatalf(
					"workflow lock version = %d, want 0",
					actualWorkflow.LockVersion(),
				)
			}

			pageRequest, err :=
				repository.NewPageRequest(
					10,
					"",
				)
			if err != nil {
				t.Fatalf(
					"NewPageRequest() returned an error: %v",
					err,
				)
			}

			eventPage, err :=
				store.ListExecutionEvents(
					ctx,
					successFixture.companyID,
					successFixture.
						workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionEvents() returned an error: %v",
					err,
				)
			}

			if len(eventPage.Items()) != 1 {
				t.Fatalf(
					"event count = %d, want 1",
					len(eventPage.Items()),
				)
			}

			if eventPage.Items()[0].
				SequenceNumber() !=
				repository.SequenceNumber(1) {
				t.Fatalf(
					"event sequence = %d, want 1",
					eventPage.Items()[0].
						SequenceNumber(),
				)
			}

			logPage, err :=
				store.ListExecutionLogs(
					ctx,
					successFixture.companyID,
					successFixture.
						workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionLogs() returned an error: %v",
					err,
				)
			}

			if len(logPage.Items()) != 2 {
				t.Fatalf(
					"log count = %d, want 2",
					len(logPage.Items()),
				)
			}

			if logPage.Items()[0].
				SequenceNumber() !=
				repository.SequenceNumber(2) {
				t.Fatalf(
					"first log sequence = %d, want 2",
					logPage.Items()[0].
						SequenceNumber(),
				)
			}

			if logPage.Items()[1].
				SequenceNumber() !=
				repository.SequenceNumber(3) {
				t.Fatalf(
					"second log sequence = %d, want 3",
					logPage.Items()[1].
						SequenceNumber(),
				)
			}

			nodePage, err :=
				store.ListNodeExecutions(
					ctx,
					successFixture.companyID,
					successFixture.
						workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListNodeExecutions() returned an error: %v",
					err,
				)
			}

			if len(nodePage.Items()) != 0 {
				t.Fatalf(
					"node execution count = %d, want 0",
					len(nodePage.Items()),
				)
			}

			duplicateErr :=
				store.CreateExecution(
					ctx,
					successFixture.command,
				)
			if !repository.IsConflict(
				duplicateErr,
			) {
				t.Fatalf(
					"duplicate CreateExecution() error = %v, want CONFLICT",
					duplicateErr,
				)
			}
		},
	)

	t.Run(
		"rolls back every new row when a timeline insert fails",
		func(t *testing.T) {
			if err := store.Create(
				ctx,
				rollbackHolder.command.
					Snapshot(),
			); err != nil {
				t.Fatalf(
					"create rollback holder snapshot: %v",
					err,
				)
			}

			insertWorkflowExecutionReadFixture(
				t,
				ctx,
				store,
				rollbackHolder.command.
					WorkflowExecution(),
			)

			_, err := store.pool.Exec(
				ctx,
				`
INSERT INTO workflow_runtime.execution_logs (
	log_id,
	workflow_execution_id,
	company_id,
	sequence_number,
	level,
	message,
	metadata,
	created_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7::jsonb,
	$8
)
`,
				duplicateLogID.String(),
				rollbackHolder.
					workflowExecutionID.String(),
				rollbackHolder.
					companyID.String(),
				int64(1),
				repository.
					ExecutionLogLevelInfo.String(),
				"Existing log used to force a rollback",
				`{"fixture":"rollback-holder"}`,
				rollbackHolder.createdAt,
			)
			if err != nil {
				t.Fatalf(
					"insert rollback holder log: %v",
					err,
				)
			}

			createErr :=
				store.CreateExecution(
					ctx,
					rollbackTarget.command,
				)
			if !repository.IsConflict(
				createErr,
			) {
				t.Fatalf(
					"CreateExecution() rollback error = %v, want CONFLICT",
					createErr,
				)
			}

			assertCreateExecutionIntegrationRowCount(
				t,
				ctx,
				store,
				"workflow_definition_snapshots",
				"snapshot_id",
				rollbackTarget.
					snapshotID.String(),
				0,
			)

			assertCreateExecutionIntegrationRowCount(
				t,
				ctx,
				store,
				"workflow_executions",
				"workflow_execution_id",
				rollbackTarget.
					workflowExecutionID.String(),
				0,
			)

			assertCreateExecutionIntegrationRowCount(
				t,
				ctx,
				store,
				"execution_events",
				"event_id",
				rollbackTarget.
					eventID.String(),
				0,
			)

			assertCreateExecutionIntegrationRowCount(
				t,
				ctx,
				store,
				"execution_logs",
				"log_id",
				duplicateLogID.String(),
				1,
			)
		},
	)
}

type createExecutionIntegrationFixture struct {
	command repository.CreateExecutionCommand

	companyID workflow.CompanyID

	snapshotID repository.DefinitionSnapshotID

	workflowExecutionID execution.WorkflowExecutionID

	eventID repository.ExecutionEventID

	createdAt time.Time
}

func newCreateExecutionIntegrationFixture(
	t *testing.T,
	suffix string,
	firstLogID string,
) createExecutionIntegrationFixture {
	t.Helper()

	companyID := workflow.CompanyID(
		"company-create-execution-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-create-execution-" + suffix,
	)

	snapshotID :=
		repository.DefinitionSnapshotID(
			"snapshot-create-execution-" +
				suffix,
		)

	workflowExecutionID :=
		execution.WorkflowExecutionID(
			"execution-create-execution-" +
				suffix,
		)

	createdAt := time.Date(
		2026,
		time.July,
		17,
		19,
		0,
		0,
		0,
		time.UTC,
	).Add(
		time.Duration(len(suffix)) *
			time.Second,
	)

	snapshot, err :=
		repository.NewDefinitionSnapshot(
			snapshotID,
			companyID,
			workflowID,
			3,
			"Create Execution Integration",
			[]byte(
				`{"nodes":[],"edges":[]}`,
			),
			createdAt.Add(-time.Minute),
		)
	if err != nil {
		t.Fatalf(
			"NewDefinitionSnapshot() returned an error: %v",
			err,
		)
	}

	workflowExecution, err :=
		repository.NewWorkflowExecutionRecord(
			repository.
				WorkflowExecutionRecordParams{
				ID:               workflowExecutionID,
				CompanyID:        companyID,
				WorkflowID:       workflowID,
				WorkflowRevision: 3,
				SnapshotID:       snapshotID,
				Mode:             execution.ExecutionModeSync,
				CorrelationID:    "correlation-" + suffix,
				Status: execution.
					WorkflowExecutionStatusCreated,
				CreatedAt:          createdAt,
				UpdatedAt:          createdAt,
				TerminalOutputs:    []byte(`{}`),
				NextSequenceNumber: repository.SequenceNumber(1),
				LockVersion:        0,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an error: %v",
			err,
		)
	}

	eventID := repository.ExecutionEventID(
		"event-workflow-created-" + suffix,
	)

	eventDraft, err :=
		repository.NewExecutionEventDraft(
			repository.ExecutionEventDraftParams{
				ID:                  eventID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				Type: repository.
					ExecutionEventTypeWorkflowCreated,
				NewStatus: execution.
					WorkflowExecutionStatusCreated.
					String(),
				CorrelationID: "correlation-" + suffix,
				CausationID:   "request-" + suffix,
				SafeMessage:   "Workflow execution created",
				Metadata: []byte(
					`{"source":"integration-test"}`,
				),
				CreatedAt: createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventDraft() returned an error: %v",
			err,
		)
	}

	eventEntry, err :=
		repository.NewEventTimelineEntry(
			eventDraft,
		)
	if err != nil {
		t.Fatalf(
			"NewEventTimelineEntry() returned an error: %v",
			err,
		)
	}

	if firstLogID == "" {
		firstLogID =
			"log-workflow-created-" + suffix
	}

	firstLogEntry :=
		newCreateExecutionIntegrationLogEntry(
			t,
			repository.ExecutionLogID(
				firstLogID,
			),
			workflowExecutionID,
			companyID,
			"Workflow execution was created",
			createdAt.Add(time.Second),
		)

	secondLogEntry :=
		newCreateExecutionIntegrationLogEntry(
			t,
			repository.ExecutionLogID(
				"log-workflow-prepared-"+
					suffix,
			),
			workflowExecutionID,
			companyID,
			"Workflow execution persistence was prepared",
			createdAt.Add(
				2*time.Second,
			),
		)

	command, err :=
		repository.NewCreateExecutionCommand(
			repository.CreateExecutionCommandParams{
				Snapshot:          snapshot,
				WorkflowExecution: workflowExecution,
				Timeline: []repository.TimelineEntry{
					eventEntry,
					firstLogEntry,
					secondLogEntry,
				},
			},
		)
	if err != nil {
		t.Fatalf(
			"NewCreateExecutionCommand() returned an error: %v",
			err,
		)
	}

	return createExecutionIntegrationFixture{
		command:             command,
		companyID:           companyID,
		snapshotID:          snapshotID,
		workflowExecutionID: workflowExecutionID,
		eventID:             eventID,
		createdAt:           createdAt,
	}
}

func newCreateExecutionIntegrationLogEntry(
	t *testing.T,
	logID repository.ExecutionLogID,
	workflowExecutionID execution.WorkflowExecutionID,
	companyID workflow.CompanyID,
	message string,
	createdAt time.Time,
) repository.TimelineEntry {
	t.Helper()

	draft, err :=
		repository.NewExecutionLogDraft(
			repository.ExecutionLogDraftParams{
				ID:                  logID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				Level: repository.
					ExecutionLogLevelInfo,
				Message: message,
				Metadata: []byte(
					`{"source":"integration-test"}`,
				),
				CreatedAt: createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an error: %v",
			err,
		)
	}

	entry, err :=
		repository.NewLogTimelineEntry(
			draft,
		)
	if err != nil {
		t.Fatalf(
			"NewLogTimelineEntry() returned an error: %v",
			err,
		)
	}

	return entry
}

func cleanupCreateExecutionIntegrationCompany(
	t *testing.T,
	ctx context.Context,
	store *Store,
	companyID workflow.CompanyID,
) {
	t.Helper()

	tables := []string{
		"execution_errors",
		"execution_logs",
		"execution_events",
		"node_executions",
		"workflow_executions",
		"workflow_definition_snapshots",
	}

	for _, table := range tables {
		query := fmt.Sprintf(
			"DELETE FROM workflow_runtime.%s WHERE company_id = $1",
			table,
		)

		if _, err := store.pool.Exec(
			ctx,
			query,
			companyID.String(),
		); err != nil {
			t.Errorf(
				"cleanup %s for company %q: %v",
				table,
				companyID,
				err,
			)
		}
	}
}

func assertCreateExecutionIntegrationRowCount(
	t *testing.T,
	ctx context.Context,
	store *Store,
	table string,
	column string,
	value string,
	expected int,
) {
	t.Helper()

	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM workflow_runtime.%s WHERE %s = $1",
		table,
		column,
	)

	var actual int

	if err := store.pool.QueryRow(
		ctx,
		query,
		value,
	).Scan(&actual); err != nil {
		t.Fatalf(
			"count %s by %s: %v",
			table,
			column,
			err,
		)
	}

	if actual != expected {
		t.Fatalf(
			"%s row count = %d, want %d",
			table,
			actual,
			expected,
		)
	}
}

func TestCreateExecutionStoreContractIsImplemented(
	t *testing.T,
) {
	var implementation createExecutionStore = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement createExecutionStore",
		)
	}
}

func TestCreateExecutionRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	err := store.CreateExecution(
		context.Background(),
		repository.CreateExecutionCommand{},
	)
	if err == nil {
		t.Fatal(
			"CreateExecution() returned nil error for nil store",
		)
	}
}

func TestCreateExecutionRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedCreateExecutionStore(t)

	err := store.CreateExecution(
		nil,
		repository.CreateExecutionCommand{},
	)
	if err == nil {
		t.Fatal(
			"CreateExecution() accepted nil context",
		)
	}
}

func TestCreateExecutionPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedCreateExecutionStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err := store.CreateExecution(
		ctx,
		repository.CreateExecutionCommand{},
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"CreateExecution() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestCreateExecutionRejectsInvalidCommand(
	t *testing.T,
) {
	store := newUnconnectedCreateExecutionStore(t)

	err := store.CreateExecution(
		context.Background(),
		repository.CreateExecutionCommand{},
	)
	if err == nil {
		t.Fatal(
			"CreateExecution() accepted an invalid command",
		)
	}
}

func TestNextSequenceAfterTimeline(
	t *testing.T,
) {
	tests := []struct {
		name           string
		start          repository.SequenceNumber
		timelineLength int
		expected       repository.SequenceNumber
		wantError      bool
	}{
		{
			name:           "allocates timeline",
			start:          repository.SequenceNumber(1),
			timelineLength: 3,
			expected:       repository.SequenceNumber(4),
		},
		{
			name:           "rejects invalid start",
			start:          0,
			timelineLength: 1,
			wantError:      true,
		},
		{
			name:           "rejects empty timeline",
			start:          repository.SequenceNumber(1),
			timelineLength: 0,
			wantError:      true,
		},
		{
			name: "rejects overflow",
			start: repository.SequenceNumber(
				math.MaxInt64,
			),
			timelineLength: 1,
			wantError:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := nextSequenceAfterTimeline(
				test.start,
				test.timelineLength,
			)

			if test.wantError {
				if err == nil {
					t.Fatal(
						"nextSequenceAfterTimeline() returned nil error",
					)
				}

				return
			}

			if err != nil {
				t.Fatalf(
					"nextSequenceAfterTimeline() returned an unexpected error: %v",
					err,
				)
			}

			if actual != test.expected {
				t.Fatalf(
					"next sequence = %d, want %d",
					actual,
					test.expected,
				)
			}
		})
	}
}

func newUnconnectedCreateExecutionStore(
	t *testing.T,
) *Store {
	t.Helper()

	store, err := NewStore(
		new(pgxpool.Pool),
	)
	if err != nil {
		t.Fatalf(
			"NewStore() returned an unexpected error: %v",
			err,
		)
	}

	return store
}

const (
	createNodeExecutionsExpectedLockVersion int64 = 4
)

var createNodeExecutionsExpectedSequence = repository.SequenceNumber(7)

func TestCreateNodeExecutionsIntegration(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(t)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"workflow_executions",
	)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"node_executions",
	)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"execution_events",
	)

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	successFixture :=
		newCreateNodeExecutionsIntegrationFixture(
			t,
			"success-"+suffix,
			"",
		)

	sharedEventID := repository.ExecutionEventID(
		"shared-node-created-event-" + suffix,
	)

	rollbackHolder :=
		newCreateNodeExecutionsIntegrationFixture(
			t,
			"rollback-holder-"+suffix,
			sharedEventID.String(),
		)

	rollbackTarget :=
		newCreateNodeExecutionsIntegrationFixture(
			t,
			"rollback-target-"+suffix,
			sharedEventID.String(),
		)

	fixtures := []createNodeExecutionsIntegrationFixture{
		successFixture,
		rollbackHolder,
		rollbackTarget,
	}

	t.Cleanup(func() {
		cleanupContext, cancelCleanup :=
			context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
		defer cancelCleanup()

		for _, fixture := range fixtures {
			cleanupCreateExecutionIntegrationCompany(
				t,
				cleanupContext,
				store,
				fixture.companyID,
			)
		}
	})

	t.Run(
		"creates pending nodes and node created events atomically",
		func(t *testing.T) {
			prepareCreateNodeExecutionsIntegrationFixture(
				t,
				ctx,
				store,
				successFixture,
			)

			err := store.CreateNodeExecutions(
				ctx,
				successFixture.command,
			)
			if err != nil {
				t.Fatalf(
					"CreateNodeExecutions() returned an error: %v",
					err,
				)
			}

			actualWorkflow, err :=
				store.GetWorkflowExecution(
					ctx,
					successFixture.companyID,
					successFixture.workflowExecutionID,
				)
			if err != nil {
				t.Fatalf(
					"GetWorkflowExecution() returned an error: %v",
					err,
				)
			}

			if actualWorkflow.Status() !=
				execution.WorkflowExecutionStatusValidating {
				t.Fatalf(
					"workflow status = %q, want VALIDATING",
					actualWorkflow.Status(),
				)
			}

			if actualWorkflow.LockVersion() !=
				createNodeExecutionsExpectedLockVersion+1 {
				t.Fatalf(
					"workflow lock version = %d, want %d",
					actualWorkflow.LockVersion(),
					createNodeExecutionsExpectedLockVersion+1,
				)
			}

			expectedNextSequence :=
				repository.SequenceNumber(
					createNodeExecutionsExpectedSequence.
						Int64() +
						int64(
							len(
								successFixture.
									nodeExecutions,
							),
						),
				)

			if actualWorkflow.NextSequenceNumber() !=
				expectedNextSequence {
				t.Fatalf(
					"workflow next sequence = %d, want %d",
					actualWorkflow.NextSequenceNumber(),
					expectedNextSequence,
				)
			}

			pageRequest, err :=
				repository.NewPageRequest(
					10,
					"",
				)
			if err != nil {
				t.Fatalf(
					"NewPageRequest() returned an error: %v",
					err,
				)
			}

			nodePage, err :=
				store.ListNodeExecutions(
					ctx,
					successFixture.companyID,
					successFixture.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListNodeExecutions() returned an error: %v",
					err,
				)
			}

			if len(nodePage.Items()) !=
				len(successFixture.nodeExecutions) {
				t.Fatalf(
					"node count = %d, want %d",
					len(nodePage.Items()),
					len(successFixture.nodeExecutions),
				)
			}

			for index, expectedNode := range successFixture.nodeExecutions {
				actualNode :=
					nodePage.Items()[index]

				if actualNode.ID() !=
					expectedNode.ID() {
					t.Fatalf(
						"node[%d] ID = %q, want %q",
						index,
						actualNode.ID(),
						expectedNode.ID(),
					)
				}

				if actualNode.Status() !=
					execution.NodeExecutionStatusPending {
					t.Fatalf(
						"node[%d] status = %q, want PENDING",
						index,
						actualNode.Status(),
					)
				}

				if actualNode.LockVersion() != 0 {
					t.Fatalf(
						"node[%d] lock version = %d, want 0",
						index,
						actualNode.LockVersion(),
					)
				}
			}

			eventPage, err :=
				store.ListExecutionEvents(
					ctx,
					successFixture.companyID,
					successFixture.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionEvents() returned an error: %v",
					err,
				)
			}

			if len(eventPage.Items()) !=
				len(successFixture.eventIDs) {
				t.Fatalf(
					"event count = %d, want %d",
					len(eventPage.Items()),
					len(successFixture.eventIDs),
				)
			}

			for index, expectedEventID := range successFixture.eventIDs {
				actualEvent :=
					eventPage.Items()[index]

				if actualEvent.ID() != expectedEventID {
					t.Fatalf(
						"event[%d] ID = %q, want %q",
						index,
						actualEvent.ID(),
						expectedEventID,
					)
				}

				expectedSequence :=
					repository.SequenceNumber(
						createNodeExecutionsExpectedSequence.
							Int64() +
							int64(index),
					)

				if actualEvent.SequenceNumber() !=
					expectedSequence {
					t.Fatalf(
						"event[%d] sequence = %d, want %d",
						index,
						actualEvent.SequenceNumber(),
						expectedSequence,
					)
				}

				if actualEvent.Type() !=
					repository.ExecutionEventTypeNodeCreated {
					t.Fatalf(
						"event[%d] type = %q, want NODE_CREATED",
						index,
						actualEvent.Type(),
					)
				}
			}

			staleErr := store.CreateNodeExecutions(
				ctx,
				successFixture.command,
			)
			if !repository.IsStaleWrite(
				staleErr,
			) {
				t.Fatalf(
					"repeated CreateNodeExecutions() error = %v, want STALE_WRITE",
					staleErr,
				)
			}
		},
	)

	t.Run(
		"rolls back workflow and nodes when timeline insert fails",
		func(t *testing.T) {
			prepareCreateNodeExecutionsIntegrationFixture(
				t,
				ctx,
				store,
				rollbackHolder,
			)

			prepareCreateNodeExecutionsIntegrationFixture(
				t,
				ctx,
				store,
				rollbackTarget,
			)

			if err := store.CreateNodeExecutions(
				ctx,
				rollbackHolder.command,
			); err != nil {
				t.Fatalf(
					"create rollback holder nodes: %v",
					err,
				)
			}

			createErr := store.CreateNodeExecutions(
				ctx,
				rollbackTarget.command,
			)
			if !repository.IsConflict(
				createErr,
			) {
				t.Fatalf(
					"CreateNodeExecutions() rollback error = %v, want CONFLICT",
					createErr,
				)
			}

			actualWorkflow, err :=
				store.GetWorkflowExecution(
					ctx,
					rollbackTarget.companyID,
					rollbackTarget.workflowExecutionID,
				)
			if err != nil {
				t.Fatalf(
					"GetWorkflowExecution() after rollback: %v",
					err,
				)
			}

			if actualWorkflow.LockVersion() !=
				createNodeExecutionsExpectedLockVersion {
				t.Fatalf(
					"workflow lock version after rollback = %d, want %d",
					actualWorkflow.LockVersion(),
					createNodeExecutionsExpectedLockVersion,
				)
			}

			if actualWorkflow.NextSequenceNumber() !=
				createNodeExecutionsExpectedSequence {
				t.Fatalf(
					"workflow next sequence after rollback = %d, want %d",
					actualWorkflow.NextSequenceNumber(),
					createNodeExecutionsExpectedSequence,
				)
			}

			pageRequest, err :=
				repository.NewPageRequest(
					10,
					"",
				)
			if err != nil {
				t.Fatalf(
					"NewPageRequest() returned an error: %v",
					err,
				)
			}

			nodePage, err :=
				store.ListNodeExecutions(
					ctx,
					rollbackTarget.companyID,
					rollbackTarget.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListNodeExecutions() after rollback: %v",
					err,
				)
			}

			if len(nodePage.Items()) != 0 {
				t.Fatalf(
					"node count after rollback = %d, want 0",
					len(nodePage.Items()),
				)
			}

			eventPage, err :=
				store.ListExecutionEvents(
					ctx,
					rollbackTarget.companyID,
					rollbackTarget.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionEvents() after rollback: %v",
					err,
				)
			}

			if len(eventPage.Items()) != 0 {
				t.Fatalf(
					"event count after rollback = %d, want 0",
					len(eventPage.Items()),
				)
			}
		},
	)
}

type createNodeExecutionsIntegrationFixture struct {
	command repository.CreateNodeExecutionsCommand

	snapshot repository.DefinitionSnapshot

	workflowExecution repository.WorkflowExecutionRecord

	companyID workflow.CompanyID

	workflowExecutionID execution.WorkflowExecutionID

	nodeExecutions []repository.NodeExecutionRecord

	eventIDs []repository.ExecutionEventID
}

func newCreateNodeExecutionsIntegrationFixture(
	t *testing.T,
	suffix string,
	firstEventID string,
) createNodeExecutionsIntegrationFixture {
	t.Helper()

	companyID := workflow.CompanyID(
		"company-create-nodes-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-create-nodes-" + suffix,
	)

	snapshotID :=
		repository.DefinitionSnapshotID(
			"snapshot-create-nodes-" + suffix,
		)

	workflowExecutionID :=
		execution.WorkflowExecutionID(
			"execution-create-nodes-" + suffix,
		)

	createdAt := time.Date(
		2026,
		time.July,
		17,
		20,
		0,
		0,
		0,
		time.UTC,
	).Add(
		time.Duration(len(suffix)) *
			time.Millisecond,
	)

	snapshot, err :=
		repository.NewDefinitionSnapshot(
			snapshotID,
			companyID,
			workflowID,
			5,
			"Create Node Executions Integration",
			[]byte(
				`{"nodes":[],"edges":[]}`,
			),
			createdAt.Add(-time.Minute),
		)
	if err != nil {
		t.Fatalf(
			"NewDefinitionSnapshot() returned an error: %v",
			err,
		)
	}

	workflowExecution, err :=
		repository.NewWorkflowExecutionRecord(
			repository.WorkflowExecutionRecordParams{
				ID:                 workflowExecutionID,
				CompanyID:          companyID,
				WorkflowID:         workflowID,
				WorkflowRevision:   5,
				SnapshotID:         snapshotID,
				Mode:               execution.ExecutionModeSync,
				CorrelationID:      "correlation-" + suffix,
				Status:             execution.WorkflowExecutionStatusValidating,
				CreatedAt:          createdAt,
				ValidatingAt:       createdAt.Add(time.Second),
				UpdatedAt:          createdAt.Add(time.Second),
				TerminalOutputs:    []byte(`{}`),
				NextSequenceNumber: createNodeExecutionsExpectedSequence,
				LockVersion:        createNodeExecutionsExpectedLockVersion,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an error: %v",
			err,
		)
	}

	nodeExecutions := make(
		[]repository.NodeExecutionRecord,
		2,
	)

	timeline := make(
		[]repository.TimelineEntry,
		2,
	)

	eventIDs := make(
		[]repository.ExecutionEventID,
		2,
	)

	for index := range nodeExecutions {
		nodeCreatedAt :=
			createdAt.Add(
				time.Duration(index+2) *
					time.Second,
			)

		nodeExecutionID :=
			execution.NodeExecutionID(
				fmt.Sprintf(
					"node-execution-%d-%s",
					index+1,
					suffix,
				),
			)

		nodeExecution, err :=
			repository.NewNodeExecutionRecord(
				repository.NodeExecutionRecordParams{
					ID:                  nodeExecutionID,
					WorkflowExecutionID: workflowExecutionID,
					CompanyID:           companyID,
					NodeID: workflow.NodeID(
						fmt.Sprintf(
							"node-%d",
							index+1,
						),
					),
					PluginType: workflow.PluginType(
						"core.pass-through",
					),
					PluginVersion: workflow.PluginVersion(
						"v1",
					),
					Status:      execution.NodeExecutionStatusPending,
					Attempt:     1,
					CreatedAt:   nodeCreatedAt,
					UpdatedAt:   nodeCreatedAt,
					LockVersion: 0,
				},
			)
		if err != nil {
			t.Fatalf(
				"NewNodeExecutionRecord() returned an error: %v",
				err,
			)
		}

		eventID := repository.ExecutionEventID(
			fmt.Sprintf(
				"node-created-event-%d-%s",
				index+1,
				suffix,
			),
		)

		if index == 0 &&
			firstEventID != "" {
			eventID =
				repository.ExecutionEventID(
					firstEventID,
				)
		}

		eventDraft, err :=
			repository.NewExecutionEventDraft(
				repository.ExecutionEventDraftParams{
					ID:                  eventID,
					WorkflowExecutionID: workflowExecutionID,
					CompanyID:           companyID,
					NodeExecutionID:     nodeExecutionID,
					Type:                repository.ExecutionEventTypeNodeCreated,
					NewStatus: execution.NodeExecutionStatusPending.
						String(),
					CorrelationID: "correlation-" + suffix,
					CausationID:   "validation-" + suffix,
					SafeMessage:   "Node execution created",
					Metadata: []byte(
						fmt.Sprintf(
							`{"nodeIndex":%d}`,
							index+1,
						),
					),
					CreatedAt: nodeCreatedAt,
				},
			)
		if err != nil {
			t.Fatalf(
				"NewExecutionEventDraft() returned an error: %v",
				err,
			)
		}

		entry, err :=
			repository.NewEventTimelineEntry(
				eventDraft,
			)
		if err != nil {
			t.Fatalf(
				"NewEventTimelineEntry() returned an error: %v",
				err,
			)
		}

		nodeExecutions[index] = nodeExecution
		timeline[index] = entry
		eventIDs[index] = eventID
	}

	command, err :=
		repository.NewCreateNodeExecutionsCommand(
			repository.CreateNodeExecutionsCommandParams{
				CompanyID:                   companyID,
				WorkflowExecutionID:         workflowExecutionID,
				ExpectedWorkflowStatus:      execution.WorkflowExecutionStatusValidating,
				ExpectedWorkflowLockVersion: createNodeExecutionsExpectedLockVersion,
				ExpectedNextSequenceNumber:  createNodeExecutionsExpectedSequence,
				NodeExecutions:              nodeExecutions,
				Timeline:                    timeline,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewCreateNodeExecutionsCommand() returned an error: %v",
			err,
		)
	}

	return createNodeExecutionsIntegrationFixture{
		command:             command,
		snapshot:            snapshot,
		workflowExecution:   workflowExecution,
		companyID:           companyID,
		workflowExecutionID: workflowExecutionID,
		nodeExecutions:      nodeExecutions,
		eventIDs:            eventIDs,
	}
}

func prepareCreateNodeExecutionsIntegrationFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	fixture createNodeExecutionsIntegrationFixture,
) {
	t.Helper()

	if err := store.Create(
		ctx,
		fixture.snapshot,
	); err != nil {
		t.Fatalf(
			"create snapshot fixture: %v",
			err,
		)
	}

	insertWorkflowExecutionReadFixture(
		t,
		ctx,
		store,
		fixture.workflowExecution,
	)
}

func TestCreateNodeExecutionsStoreContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.ExecutionCreationStore = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement ExecutionCreationStore",
		)
	}
}

func TestCreateNodeExecutionsRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	err := store.CreateNodeExecutions(
		context.Background(),
		repository.CreateNodeExecutionsCommand{},
	)
	if err == nil {
		t.Fatal(
			"CreateNodeExecutions() returned nil error for nil store",
		)
	}
}

func TestCreateNodeExecutionsRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedCreateNodeExecutionsStore(t)

	err := store.CreateNodeExecutions(
		nil,
		repository.CreateNodeExecutionsCommand{},
	)
	if err == nil {
		t.Fatal(
			"CreateNodeExecutions() accepted nil context",
		)
	}
}

func TestCreateNodeExecutionsPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedCreateNodeExecutionsStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err := store.CreateNodeExecutions(
		ctx,
		repository.CreateNodeExecutionsCommand{},
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"CreateNodeExecutions() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestCreateNodeExecutionsRejectsInvalidCommand(
	t *testing.T,
) {
	store := newUnconnectedCreateNodeExecutionsStore(t)

	err := store.CreateNodeExecutions(
		context.Background(),
		repository.CreateNodeExecutionsCommand{},
	)
	if err == nil {
		t.Fatal(
			"CreateNodeExecutions() accepted an invalid command",
		)
	}
}

func newUnconnectedCreateNodeExecutionsStore(
	t *testing.T,
) *Store {
	t.Helper()

	store, err := NewStore(
		new(pgxpool.Pool),
	)
	if err != nil {
		t.Fatalf(
			"NewStore() returned an unexpected error: %v",
			err,
		)
	}

	return store
}

func TestTimestampCursorRoundTrip(
	t *testing.T,
) {
	expectedTime := time.Date(
		2026,
		time.July,
		17,
		18,
		30,
		45,
		123456789,
		time.FixedZone(
			"test-offset",
			3*60*60,
		),
	)

	token, err := encodeTimestampCursor(
		cursorKindWorkflowExecutions,
		expectedTime,
		" execution-1 ",
	)
	if err != nil {
		t.Fatalf(
			"encodeTimestampCursor() returned an unexpected error: %v",
			err,
		)
	}

	actualTime, actualIdentifier, err :=
		decodeTimestampCursor(
			cursorKindWorkflowExecutions,
			token,
		)
	if err != nil {
		t.Fatalf(
			"decodeTimestampCursor() returned an unexpected error: %v",
			err,
		)
	}

	if !actualTime.Equal(
		expectedTime.UTC(),
	) {
		t.Fatalf(
			"decoded time = %v, want %v",
			actualTime,
			expectedTime.UTC(),
		)
	}

	if actualTime.Location() !=
		time.UTC {
		t.Fatalf(
			"decoded time location = %v, want UTC",
			actualTime.Location(),
		)
	}

	if actualIdentifier !=
		"execution-1" {
		t.Fatalf(
			"decoded identifier = %q, want %q",
			actualIdentifier,
			"execution-1",
		)
	}
}

func TestSequenceCursorRoundTrip(
	t *testing.T,
) {
	expected :=
		repository.SequenceNumber(42)

	token, err := encodeSequenceCursor(
		cursorKindExecutionEvents,
		expected,
	)
	if err != nil {
		t.Fatalf(
			"encodeSequenceCursor() returned an unexpected error: %v",
			err,
		)
	}

	actual, err := decodeSequenceCursor(
		cursorKindExecutionEvents,
		token,
	)
	if err != nil {
		t.Fatalf(
			"decodeSequenceCursor() returned an unexpected error: %v",
			err,
		)
	}

	if actual != expected {
		t.Fatalf(
			"decoded sequence = %d, want %d",
			actual,
			expected,
		)
	}
}

func TestCursorEncodingIsDeterministic(
	t *testing.T,
) {
	timestamp := time.Date(
		2026,
		time.July,
		17,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	first, err := encodeTimestampCursor(
		cursorKindNodeExecutions,
		timestamp,
		"node-execution-1",
	)
	if err != nil {
		t.Fatalf(
			"first encodeTimestampCursor() error: %v",
			err,
		)
	}

	second, err := encodeTimestampCursor(
		cursorKindNodeExecutions,
		timestamp,
		"node-execution-1",
	)
	if err != nil {
		t.Fatalf(
			"second encodeTimestampCursor() error: %v",
			err,
		)
	}

	if first != second {
		t.Fatalf(
			"cursor encoding is not deterministic: %q != %q",
			first,
			second,
		)
	}
}

func TestCursorDecodingRejectsDifferentQueryKind(
	t *testing.T,
) {
	token, err := encodeSequenceCursor(
		cursorKindExecutionEvents,
		repository.SequenceNumber(10),
	)
	if err != nil {
		t.Fatalf(
			"encodeSequenceCursor() error: %v",
			err,
		)
	}

	_, err = decodeSequenceCursor(
		cursorKindExecutionLogs,
		token,
	)
	if err == nil {
		t.Fatal(
			"decodeSequenceCursor() accepted a token for another query kind",
		)
	}
}

func TestCursorEncodersRejectInvalidValues(
	t *testing.T,
) {
	timestamp := time.Date(
		2026,
		time.July,
		17,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	tests := []struct {
		name   string
		encode func() error
	}{
		{
			name: "invalid timestamp cursor kind",
			encode: func() error {
				_, err := encodeTimestampCursor(
					cursorKind("unknown"),
					timestamp,
					"execution-1",
				)
				return err
			},
		},
		{
			name: "zero timestamp",
			encode: func() error {
				_, err := encodeTimestampCursor(
					cursorKindWorkflowExecutions,
					time.Time{},
					"execution-1",
				)
				return err
			},
		},
		{
			name: "blank identifier",
			encode: func() error {
				_, err := encodeTimestampCursor(
					cursorKindWorkflowExecutions,
					timestamp,
					" ",
				)
				return err
			},
		},
		{
			name: "invalid sequence",
			encode: func() error {
				_, err := encodeSequenceCursor(
					cursorKindExecutionEvents,
					0,
				)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.encode(); err == nil {
				t.Fatal(
					"cursor encoder returned nil error",
				)
			}
		})
	}
}

func TestDecodeCursorPayloadRejectsMalformedTokens(
	t *testing.T,
) {
	tests := []struct {
		name  string
		token repository.PageToken
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "invalid base64",
			token: "***",
		},
		{
			name: "invalid json",
			token: encodeRawCursorTestContent(
				`not-json`,
			),
		},
		{
			name: "unknown json field",
			token: encodeRawCursorTestContent(
				`{"v":1,"k":"execution_events","s":1,"unknown":true}`,
			),
		},
		{
			name: "unsupported version",
			token: encodeRawCursorTestContent(
				`{"v":2,"k":"execution_events","s":1}`,
			),
		},
		{
			name: "invalid kind",
			token: encodeRawCursorTestContent(
				`{"v":1,"k":"unknown","s":1}`,
			),
		},
		{
			name: "trailing json",
			token: encodeRawCursorTestContent(
				`{"v":1,"k":"execution_events","s":1} {}`,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeSequenceCursor(
				cursorKindExecutionEvents,
				test.token,
			)

			if err == nil {
				t.Fatal(
					"decodeSequenceCursor() returned nil error",
				)
			}
		})
	}
}

func TestDecodeTimestampCursorRejectsSequencePayload(
	t *testing.T,
) {
	token, err := encodeSequenceCursor(
		cursorKindWorkflowExecutions,
		repository.SequenceNumber(5),
	)
	if err != nil {
		t.Fatalf(
			"encodeSequenceCursor() error: %v",
			err,
		)
	}

	_, _, err = decodeTimestampCursor(
		cursorKindWorkflowExecutions,
		token,
	)
	if err == nil {
		t.Fatal(
			"decodeTimestampCursor() accepted a sequence cursor",
		)
	}
}

func TestDecodeSequenceCursorRejectsTimestampPayload(
	t *testing.T,
) {
	token, err := encodeTimestampCursor(
		cursorKindExecutionLogs,
		time.Date(
			2026,
			time.July,
			17,
			12,
			0,
			0,
			0,
			time.UTC,
		),
		"log-1",
	)
	if err != nil {
		t.Fatalf(
			"encodeTimestampCursor() error: %v",
			err,
		)
	}

	_, err = decodeSequenceCursor(
		cursorKindExecutionLogs,
		token,
	)
	if err == nil {
		t.Fatal(
			"decodeSequenceCursor() accepted a timestamp cursor",
		)
	}
}

func TestCursorTokenIsURLSafe(
	t *testing.T,
) {
	token, err := encodeTimestampCursor(
		cursorKindExecutionErrors,
		time.Date(
			2026,
			time.July,
			17,
			12,
			0,
			0,
			0,
			time.UTC,
		),
		"error-+/=",
	)
	if err != nil {
		t.Fatalf(
			"encodeTimestampCursor() error: %v",
			err,
		)
	}

	if strings.ContainsAny(
		token.String(),
		"+/=",
	) {
		t.Fatalf(
			"cursor token is not URL-safe: %q",
			token,
		)
	}
}

func encodeRawCursorTestContent(
	value string,
) repository.PageToken {
	return repository.PageToken(
		base64.RawURLEncoding.
			EncodeToString(
				[]byte(value),
			),
	)
}

func TestDurableWorkerResultStoreIntegrationStatusesAndHydration(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerWorkerResultCleanup(t, store, scope.executionID.String())
	fixtures := []repository.DurableWorkerResult{
		workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 1, repository.DurableWorkerResultSucceeded, false),
		workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 2, repository.DurableWorkerResultSucceeded, true),
		workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 3, repository.DurableWorkerResultFailed, false),
		workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 4, repository.DurableWorkerResultCancelled, false),
		workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 5, repository.DurableWorkerResultTimedOut, false),
	}
	for _, fixture := range fixtures {
		if err := store.CreateDurableWorkerResult(ctx, fixture); err != nil {
			t.Fatal(err)
		}
		hydrated, err := store.GetDurableWorkerResult(ctx, scope.companyID, scope.executionID, scope.commandNodeID, fixture.Attempt())
		if err != nil || hydrated.IdentityKey() != fixture.IdentityKey() || hydrated.Status() != fixture.Status() {
			t.Fatalf("attempt=%d hydrated=%s/%v", fixture.Attempt(), hydrated.Status(), err)
		}
		if fixture.Status() == repository.DurableWorkerResultSucceeded {
			result, exists := hydrated.Result()
			if !exists || len(result.ContextChanges().SetKeys()) != 1 {
				t.Fatalf("success shape attempt=%d", fixture.Attempt())
			}
		}
		if fixture.Status() == repository.DurableWorkerResultFailed {
			result, _ := hydrated.Result()
			failure, exists := result.Failure()
			if !exists || failure.Code() != "PLUGIN_FAILED" || failure.Details()["kind"] != "unit" {
				t.Fatalf("structured failure=%v/%t", failure, exists)
			}
		}
	}
	if err := store.CreateDurableWorkerResult(ctx, fixtures[0]); !repository.IsConflict(err) {
		t.Fatalf("duplicate result=%v", err)
	}
	if _, err := store.GetDurableWorkerResult(ctx, "other-company", scope.executionID, scope.commandNodeID, 1); !repository.IsNotFound(err) {
		t.Fatalf("cross-tenant get=%v", err)
	}
}

func TestDurableWorkerResultStoreIntegrationTenantNodeAndMalformedPersistence(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerWorkerResultCleanup(t, store, scope.executionID.String())
	for _, fixture := range []repository.DurableWorkerResult{
		workerResultFixture(t, "other-company", scope.executionID.String(), scope.commandNodeID.String(), "command", 10, repository.DurableWorkerResultSucceeded, false),
		workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "wrong-node", 11, repository.DurableWorkerResultSucceeded, false),
	} {
		if err := store.CreateDurableWorkerResult(ctx, fixture); !repository.IsConflict(err) {
			t.Fatalf("tenant/node mismatch=%v", err)
		}
	}
	failed := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 12, repository.DurableWorkerResultFailed, false)
	if err := store.CreateDurableWorkerResult(ctx, failed); err != nil {
		t.Fatal(err)
	}
	_, err := store.pool.Exec(ctx, `UPDATE workflow_runtime.async_worker_results SET failure_json='{"version":1,"category":"EXECUTION","code":"X","message":"safe","retryable":false,"unknown":true}'::jsonb WHERE worker_result_id=$1`, failed.IdentityKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDurableWorkerResult(ctx, scope.companyID, scope.executionID, scope.commandNodeID, 12); err == nil {
		t.Fatal("malformed persisted result was accepted")
	}
}

func TestDurableWorkerResultStoreIntegrationRollbackAndConcurrentDuplicate(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	registerWorkerResultCleanup(t, store, scope.executionID.String())
	rollbackResult := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 20, repository.DurableWorkerResultSucceeded, false)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createDurableWorkerResult(ctx, tx, rollbackResult); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDurableWorkerResult(ctx, scope.companyID, scope.executionID, scope.commandNodeID, 20); !repository.IsNotFound(err) {
		t.Fatalf("rollback result=%v", err)
	}
	if err := store.CreateDurableWorkerResult(ctx, rollbackResult); err != nil {
		t.Fatalf("reinsert after rollback=%v", err)
	}

	concurrent := workerResultFixture(t, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String(), "command", 21, repository.DurableWorkerResultTimedOut, false)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- store.CreateDurableWorkerResult(ctx, concurrent)
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if repository.IsConflict(err) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent result success=%d conflict=%d", successes, conflicts)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_runtime.async_worker_results WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3 AND attempt=21`, scope.companyID.String(), scope.executionID.String(), scope.commandNodeID.String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable result count=%d/%v", count, err)
	}
}

func registerWorkerResultCleanup(t *testing.T, store *Store, executionID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = store.pool.Exec(ctx, `DELETE FROM workflow_runtime.async_worker_results WHERE workflow_execution_id=$1`, executionID)
	})
}

func TestDurableWorkerResultStoreContractAndSQL(t *testing.T) {
	var implementation repository.DurableWorkerResultStore = &Store{}
	if implementation == nil || strings.Contains(strings.ToUpper(getDurableWorkerResultSQL), "SELECT *") {
		t.Fatal("durable result contract or explicit SQL unavailable")
	}
	for _, fragment := range []string{"worker_result_id", "result_status", "routed_outputs_json", "terminal_output_json", "context_changes_json", "failure_json", "node.node_id=$5"} {
		if !strings.Contains(insertDurableWorkerResultSQL, fragment) {
			t.Fatalf("insert SQL missing %q", fragment)
		}
	}
}

func TestDurableWorkerResultCodecPartsRoundTrip(t *testing.T) {
	for _, status := range []repository.DurableWorkerResultStatus{
		repository.DurableWorkerResultSucceeded,
		repository.DurableWorkerResultFailed,
		repository.DurableWorkerResultCancelled,
		repository.DurableWorkerResultTimedOut,
	} {
		result := workerResultFixture(t, "company-1", "execution-1", "node-execution-1", "node-1", 1, status, false)
		routed, terminal, changes, failure, err := encodeDurableWorkerResultParts(result)
		if err != nil {
			t.Fatal(err)
		}
		row := durableResultTestRow{result: result, routed: []byte(routed), changes: []byte(changes), terminal: anyJSONBytes(terminal), failure: anyJSONBytes(failure)}
		hydrated, err := scanDurableWorkerResult(row)
		if err != nil || hydrated.Status() != status || hydrated.IdentityKey() != result.IdentityKey() {
			t.Fatalf("status=%s hydrated=%s/%v", status, hydrated.Status(), err)
		}
	}
	terminalResult := workerResultFixture(t, "company-1", "execution-1", "node-execution-1", "node-1", 2, repository.DurableWorkerResultSucceeded, true)
	routed, terminal, changes, failure, err := encodeDurableWorkerResultParts(terminalResult)
	if err != nil {
		t.Fatal(err)
	}
	hydrated, err := scanDurableWorkerResult(durableResultTestRow{result: terminalResult, routed: []byte(routed), changes: []byte(changes), terminal: anyJSONBytes(terminal), failure: anyJSONBytes(failure)})
	if err != nil {
		t.Fatal(err)
	}
	runtimeResult, exists := hydrated.Result()
	if !exists || !runtimeResult.HasTerminalOutput() || runtimeResult.ContextChanges().SetKeys()[0] != "progress" {
		t.Fatal("terminal output or context changes were not hydrated")
	}
}

func TestDurableWorkerResultErrorsAreMapped(t *testing.T) {
	for _, test := range []struct{ constraint, resource string }{
		{"async_worker_results_pk", "result ID"},
		{"async_worker_results_node_attempt_uk", "node attempt"},
		{"async_worker_results_node_fk", "durable worker result"},
		{"async_worker_results_shape", "durable worker result"},
	} {
		code := postgreSQLUniqueViolationCode
		if strings.HasSuffix(test.constraint, "_fk") {
			code = postgreSQLForeignKeyViolationCode
		}
		if strings.HasSuffix(test.constraint, "_shape") {
			code = postgreSQLCheckViolationCode
		}
		cause := &pgconn.PgError{Code: code, ConstraintName: test.constraint}
		err := mapDurableWorkerResultWriteError("create", cause)
		if !repository.IsConflict(err) || !errors.Is(err, cause) || !strings.Contains(err.Error(), test.resource) {
			t.Fatalf("constraint=%s mapping=%v", test.constraint, err)
		}
	}
}

func workerResultFixture(t *testing.T, company, workflowExecution, nodeExecution, node string, attempt int16, status repository.DurableWorkerResultStatus, terminal bool) repository.DurableWorkerResult {
	t.Helper()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC).Add(time.Duration(attempt) * time.Minute)
	params := repository.DurableWorkerResultParams{
		CompanyID: workflow.CompanyID(company), WorkflowExecutionID: execution.WorkflowExecutionID(workflowExecution),
		NodeExecutionID: execution.NodeExecutionID(nodeExecution), NodeID: workflow.NodeID(node), Attempt: attempt, Status: status,
		StartedAt: now, FinishedAt: now.Add(time.Second), CreatedAt: now.Add(2 * time.Second),
	}
	switch status {
	case repository.DurableWorkerResultSucceeded:
		value, err := runtime.NewRuntimeValue([]byte(`{"step":1}`))
		if err != nil {
			t.Fatal(err)
		}
		changes, err := runtime.NewContextChanges(map[string]runtime.RuntimeValue{"progress": value}, nil)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := runtime.NewInlinePayload(runtime.ContentTypeApplicationJSON, []byte(`{"ok":true}`), map[string]string{"scope": "worker"}, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if terminal {
			params.Result, err = runtime.NewTerminalNodeSuccessResult(payload, changes)
		} else {
			params.Result, err = runtime.NewNodeSuccessResult(map[string][]runtime.Payload{"result": {payload}}, changes)
		}
		if err != nil {
			t.Fatal(err)
		}
	case repository.DurableWorkerResultFailed:
		failure, err := runtime.NewRuntimeFailure(runtime.FailureCategoryExecution, "PLUGIN_FAILED", "safe failure", false, map[string]string{"kind": "unit"})
		if err != nil {
			t.Fatal(err)
		}
		params.Result, err = runtime.NewNodeFailureResult(failure)
		if err != nil {
			t.Fatal(err)
		}
	case repository.DurableWorkerResultCancelled:
		params.Failure, _ = runtime.NewRuntimeFailure(runtime.FailureCategoryCanceled, "CANCELLED", "cancelled", false, nil)
	case repository.DurableWorkerResultTimedOut:
		params.Failure, _ = runtime.NewRuntimeFailure(runtime.FailureCategoryTimeout, "TIMED_OUT", "timed out", false, nil)
	}
	result, err := repository.NewDurableWorkerResult(params)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func anyJSONBytes(value any) []byte {
	if value == nil {
		return nil
	}
	return []byte(value.(string))
}

type durableResultTestRow struct {
	result                    repository.DurableWorkerResult
	routed, terminal, changes []byte
	failure                   []byte
	err                       error
}

func (row durableResultTestRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != 14 {
		return errors.New("unexpected durable result scan count")
	}
	*dest[0].(*string) = row.result.IdentityKey()
	*dest[1].(*string) = row.result.CompanyID().String()
	*dest[2].(*string) = row.result.WorkflowExecutionID().String()
	*dest[3].(*string) = row.result.NodeExecutionID().String()
	*dest[4].(*string) = row.result.NodeID().String()
	*dest[5].(*int16) = row.result.Attempt()
	*dest[6].(*string) = string(row.result.Status())
	*dest[7].(*[]byte) = append([]byte(nil), row.routed...)
	*dest[8].(*[]byte) = append([]byte(nil), row.terminal...)
	*dest[9].(*[]byte) = append([]byte(nil), row.changes...)
	*dest[10].(*[]byte) = append([]byte(nil), row.failure...)
	*dest[11].(*time.Time) = row.result.StartedAt()
	*dest[12].(*time.Time) = row.result.FinishedAt()
	*dest[13].(*time.Time) = row.result.CreatedAt()
	return nil
}

func TestMapPostgreSQLErrorReturnsNilForNilError(
	t *testing.T,
) {
	if err := mapPostgreSQLError(
		"get",
		"workflow execution",
		nil,
	); err != nil {
		t.Fatalf(
			"mapPostgreSQLError() = %v, want nil",
			err,
		)
	}
}

func TestMapPostgreSQLErrorMapsNoRowsToNotFound(
	t *testing.T,
) {
	err := mapPostgreSQLError(
		"get",
		"workflow execution",
		pgx.ErrNoRows,
	)

	if !repository.IsNotFound(err) {
		t.Fatalf(
			"mapped error type = %T, want NOT_FOUND",
			err,
		)
	}

	if !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		t.Fatal(
			"mapped not-found error did not preserve pgx.ErrNoRows",
		)
	}
}

func TestMapPostgreSQLErrorMapsUniqueViolationToConflict(
	t *testing.T,
) {
	cause := &pgconn.PgError{
		Code:           postgreSQLUniqueViolationCode,
		ConstraintName: "outbox_messages_operation_uk",
		Message:        "duplicate key value violates unique constraint",
	}

	err := mapPostgreSQLError(
		"create",
		"workflow execution",
		cause,
	)

	if !repository.IsConflict(err) {
		t.Fatalf(
			"mapped error type = %T, want CONFLICT",
			err,
		)
	}

	if !errors.Is(
		err,
		cause,
	) {
		t.Fatal(
			"mapped conflict error did not preserve its cause",
		)
	}

	if strings.Contains(
		err.Error(),
		cause.ConstraintName,
	) {
		t.Fatalf(
			"mapped error exposed a PostgreSQL constraint: %q",
			err.Error(),
		)
	}
}

func TestMapPostgreSQLErrorMapsKnownStructuredConstraintsToConflict(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		constraint string
	}{
		{
			name:       "foreign key",
			code:       postgreSQLForeignKeyViolationCode,
			constraint: "outbox_messages_execution_fk",
		},
		{
			name:       "check",
			code:       postgreSQLCheckViolationCode,
			constraint: "outbox_messages_claim_state",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cause := &pgconn.PgError{
				Code:           test.code,
				ConstraintName: test.constraint,
				Message:        "technical database detail",
			}
			err := mapPostgreSQLError("persist", "outbox message", cause)
			if !repository.IsConflict(err) {
				t.Fatalf("mapped error type = %T, want CONFLICT", err)
			}
			if !errors.Is(err, cause) {
				t.Fatal("mapped conflict did not preserve its cause")
			}
			if strings.Contains(err.Error(), test.constraint) ||
				strings.Contains(err.Error(), cause.Message) {
				t.Fatalf("mapped error exposed technical detail: %q", err.Error())
			}
		})
	}
}

func TestMapPostgreSQLErrorWrapsUnknownStructuredConstraintsSafely(t *testing.T) {
	tests := []string{
		postgreSQLUniqueViolationCode,
		postgreSQLForeignKeyViolationCode,
		postgreSQLCheckViolationCode,
	}

	for _, code := range tests {
		t.Run(code, func(t *testing.T) {
			cause := &pgconn.PgError{
				Code:           code,
				ConstraintName: "unknown_secret_constraint",
				Message:        "outbox_messages_operation_uk appears only in message text",
			}
			err := mapPostgreSQLError("persist", "outbox message", cause)
			if repository.IsNotFound(err) || repository.IsConflict(err) || repository.IsStaleWrite(err) {
				t.Fatalf("unknown constraint was incorrectly classified: %T", err)
			}
			var safeDatabaseError *databaseError
			if !errors.As(err, &safeDatabaseError) {
				t.Fatalf("mapped error type = %T, want *databaseError", err)
			}
			if !errors.Is(err, cause) {
				t.Fatal("safe database error did not preserve its cause")
			}
			if strings.Contains(err.Error(), cause.ConstraintName) ||
				strings.Contains(err.Error(), "outbox_messages_operation_uk") {
				t.Fatalf("safe database error exposed technical detail: %q", err.Error())
			}
		})
	}
}

func TestMapPostgreSQLErrorWrapsUnknownSQLStateSafely(t *testing.T) {
	cause := &pgconn.PgError{
		Code:           "99999",
		ConstraintName: "outbox_messages_operation_uk",
		Message:        "duplicate key text must not drive classification",
	}
	err := mapPostgreSQLError("persist", "outbox message", cause)

	if repository.IsNotFound(err) || repository.IsConflict(err) || repository.IsStaleWrite(err) {
		t.Fatalf("unknown SQLSTATE was incorrectly classified: %T", err)
	}
	var safeDatabaseError *databaseError
	if !errors.As(err, &safeDatabaseError) {
		t.Fatalf("mapped error type = %T, want *databaseError", err)
	}
	var preservedPostgreSQLError *pgconn.PgError
	if !errors.As(err, &preservedPostgreSQLError) || preservedPostgreSQLError != cause {
		t.Fatal("safe database error did not preserve the structured PostgreSQL cause")
	}
	if strings.Contains(err.Error(), cause.ConstraintName) ||
		strings.Contains(err.Error(), cause.Message) {
		t.Fatalf("safe database error exposed technical detail: %q", err.Error())
	}
}

func TestMapPostgreSQLErrorPreservesContextErrors(
	t *testing.T,
) {
	tests := []error{
		context.Canceled,
		context.DeadlineExceeded,
	}

	for _, expected := range tests {
		actual := mapPostgreSQLError(
			"list",
			"execution logs",
			expected,
		)

		if !errors.Is(
			actual,
			expected,
		) {
			t.Fatalf(
				"mapped context error = %v, want %v",
				actual,
				expected,
			)
		}
	}
}

func TestMapPostgreSQLErrorWrapsUnexpectedDatabaseErrorSafely(
	t *testing.T,
) {
	cause := errors.New(
		`connection failed for secret database "production_runtime"`,
	)

	err := mapPostgreSQLError(
		"update",
		"node execution",
		cause,
	)

	if err == nil {
		t.Fatal(
			"mapPostgreSQLError() returned nil",
		)
	}

	if repository.IsNotFound(err) ||
		repository.IsConflict(err) ||
		repository.IsStaleWrite(err) {
		t.Fatal(
			"unexpected database error was incorrectly classified",
		)
	}

	if !errors.Is(
		err,
		cause,
	) {
		t.Fatal(
			"database error did not preserve its technical cause",
		)
	}

	if strings.Contains(
		err.Error(),
		"production_runtime",
	) {
		t.Fatalf(
			"database error exposed technical detail: %q",
			err.Error(),
		)
	}

	expected :=
		"PostgreSQL update node execution: operation failed"

	if err.Error() != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			err.Error(),
			expected,
		)
	}
}
