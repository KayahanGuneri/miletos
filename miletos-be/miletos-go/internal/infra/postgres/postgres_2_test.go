package postgres

import (
	context "context"
	sql "database/sql"
	json "encoding/json"
	errors "errors"
	fmt "fmt"
	pgx "github.com/jackc/pgx/v5"
	pgconn "github.com/jackc/pgx/v5/pgconn"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
	config "miletos-go/internal/config"
	runtimefailure "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	transport "miletos-go/internal/infra/kafka"
	repository "miletos-go/internal/ports/persistence"
	os "os"
	reflect "reflect"
	strings "strings"
	sync "sync"
	atomic "sync/atomic"
	testing "testing"
	time "time"
)

func TestExecutionErrorReaderIntegration(
	t *testing.T,
) {
	ctx, store := requirePostgreSQLIntegrationStore(t)

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

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"execution_errors",
	)

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	companyA := workflow.CompanyID(
		"company-error-reader-a-" + suffix,
	)

	companyB := workflow.CompanyID(
		"company-error-reader-b-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-error-reader-" + suffix,
	)

	workflowExecutionID := execution.WorkflowExecutionID(
		"execution-error-reader-" + suffix,
	)

	nodeExecutionID := execution.NodeExecutionID(
		"node-execution-error-reader-" + suffix,
	)

	snapshot := newWorkflowExecutionReadSnapshot(
		t,
		companyA,
		workflowID,
		repository.DefinitionSnapshotID(
			"snapshot-error-reader-"+suffix,
		),
	)

	workflowCreatedAt := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	workflowExecution := newWorkflowExecutionReadRecord(
		t,
		workflowExecutionID,
		companyA,
		workflowID,
		snapshot.ID(),
		execution.WorkflowExecutionStatusRunning,
		workflowCreatedAt,
	)

	nodeExecution := newNodeExecutionReadRecord(
		t,
		nodeExecutionID,
		workflowExecutionID,
		companyA,
		workflow.NodeID("node-1"),
		execution.NodeExecutionStatusRunning,
		workflowCreatedAt.Add(time.Minute),
	)

	relatedEvent := newExecutionEventReadRecord(
		t,
		repository.ExecutionEventRecordParams{
			ID: repository.ExecutionEventID(
				"event-error-reader-" + suffix,
			),
			WorkflowExecutionID: workflowExecutionID,
			CompanyID:           companyA,
			NodeExecutionID:     nodeExecutionID,
			SequenceNumber:      repository.SequenceNumber(1),
			Type:                repository.ExecutionEventTypeNodeStarted,
			PreviousStatus:      execution.NodeExecutionStatusReady.String(),
			NewStatus:           execution.NodeExecutionStatusRunning.String(),
			SafeMessage:         "Node execution started",
			Metadata: []byte(
				`{"nodeId":"node-1"}`,
			),
			CreatedAt: nodeExecution.UpdatedAt(),
		},
	)

	t.Cleanup(func() {
		cleanupContext, cancelCleanup := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancelCleanup()

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM execution_errors
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"execution error cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM execution_events
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"execution event cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM node_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"node execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"workflow execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_definition_snapshots
WHERE company_id = $1
  AND snapshot_id = $2
`,
			companyA.String(),
			snapshot.ID().String(),
		); err != nil {
			t.Errorf(
				"snapshot cleanup failed: %v",
				err,
			)
		}
	})

	if err := store.Create(
		ctx,
		snapshot,
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
		workflowExecution,
	)

	insertNodeExecutionReadFixture(
		t,
		ctx,
		store,
		nodeExecution,
	)

	insertExecutionEventReadFixture(
		t,
		ctx,
		store,
		relatedEvent,
	)

	baseErrorTime := workflowCreatedAt.Add(
		2 * time.Minute,
	)

	errors := []repository.ExecutionErrorRecord{
		newExecutionErrorReadRecord(
			t,
			repository.ExecutionErrorRecordParams{
				ID: repository.ExecutionErrorID(
					"error-1-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				Category:            runtimefailure.FailureCategoryValidation,
				Code:                "WORKFLOW_VALIDATION_WARNING",
				SafeMessage:         "Workflow validation reported a controlled error",
				Retryable:           false,
				Details: []byte(
					`{"scope":"workflow"}`,
				),
				CreatedAt: baseErrorTime.Add(
					1 * time.Second,
				),
			},
		),
		newExecutionErrorReadRecord(
			t,
			repository.ExecutionErrorRecordParams{
				ID: repository.ExecutionErrorID(
					"error-2-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				Category:            runtimefailure.FailureCategoryDependency,
				Code:                "DEPENDENCY_UNAVAILABLE",
				SafeMessage:         "A required dependency is unavailable",
				TechnicalDetail:     "test dependency connection refused",
				Retryable:           true,
				Details: []byte(
					`{"dependency":"integration-test"}`,
				),
				CreatedAt: baseErrorTime.Add(
					2 * time.Second,
				),
			},
		),
		newExecutionErrorReadRecord(
			t,
			repository.ExecutionErrorRecordParams{
				ID: repository.ExecutionErrorID(
					"error-3-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				NodeExecutionID:     nodeExecutionID,
				RelatedEventID:      relatedEvent.ID(),
				Category:            runtimefailure.FailureCategoryExecution,
				Code:                "NODE_EXECUTION_FAILED",
				SafeMessage:         "Node execution failed",
				TechnicalDetail:     "test node executor returned a failure",
				Retryable:           false,
				Details: []byte(
					`{"nodeId":"node-1"}`,
				),
				CreatedAt: baseErrorTime.Add(
					3 * time.Second,
				),
			},
		),
		newExecutionErrorReadRecord(
			t,
			repository.ExecutionErrorRecordParams{
				ID: repository.ExecutionErrorID(
					"error-4-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				NodeExecutionID:     nodeExecutionID,
				Category:            runtimefailure.FailureCategoryInternal,
				Code:                "INTERNAL_RUNTIME_FAILURE",
				SafeMessage:         "An internal execution error occurred",
				TechnicalDetail:     "controlled integration test detail",
				Retryable:           false,
				Details: []byte(
					`{"component":"runtime"}`,
				),
				CreatedAt: baseErrorTime.Add(
					4 * time.Second,
				),
			},
		),
	}

	for _, errorRecord := range errors {
		insertExecutionErrorReadFixture(
			t,
			ctx,
			store,
			errorRecord,
		)
	}

	firstRequest, err := repository.NewPageRequest(
		2,
		"",
	)
	if err != nil {
		t.Fatalf(
			"create first page request: %v",
			err,
		)
	}

	firstPage, err := store.ListExecutionErrors(
		ctx,
		companyA,
		workflowExecutionID,
		firstRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListExecutionErrors() first page error: %v",
			err,
		)
	}

	assertExecutionErrorPageIDs(
		t,
		firstPage.Items(),
		[]repository.ExecutionErrorID{
			errors[0].ID(),
			errors[1].ID(),
		},
	)

	assertExecutionErrorRecordsEqual(
		t,
		firstPage.Items()[0],
		errors[0],
	)

	assertExecutionErrorRecordsEqual(
		t,
		firstPage.Items()[1],
		errors[1],
	)

	nextToken, exists := firstPage.Next()
	if !exists {
		t.Fatal(
			"first execution error page has no next token",
		)
	}

	decodedCreatedAt, decodedErrorID, err :=
		decodeTimestampCursor(
			cursorKindExecutionErrors,
			nextToken,
		)
	if err != nil {
		t.Fatalf(
			"decode first page next token: %v",
			err,
		)
	}

	if !decodedCreatedAt.Equal(
		errors[1].CreatedAt(),
	) {
		t.Fatalf(
			"decoded cursor time = %v, want %v",
			decodedCreatedAt,
			errors[1].CreatedAt(),
		)
	}

	if decodedErrorID != errors[1].ID().String() {
		t.Fatalf(
			"decoded cursor ID = %q, want %q",
			decodedErrorID,
			errors[1].ID(),
		)
	}

	secondRequest, err := repository.NewPageRequest(
		2,
		nextToken,
	)
	if err != nil {
		t.Fatalf(
			"create second page request: %v",
			err,
		)
	}

	secondPage, err := store.ListExecutionErrors(
		ctx,
		companyA,
		workflowExecutionID,
		secondRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListExecutionErrors() second page error: %v",
			err,
		)
	}

	assertExecutionErrorPageIDs(
		t,
		secondPage.Items(),
		[]repository.ExecutionErrorID{
			errors[2].ID(),
			errors[3].ID(),
		},
	)

	assertExecutionErrorRecordsEqual(
		t,
		secondPage.Items()[0],
		errors[2],
	)

	assertExecutionErrorRecordsEqual(
		t,
		secondPage.Items()[1],
		errors[3],
	)

	if secondPage.HasNext() {
		t.Fatal(
			"second execution error page unexpectedly has a next token",
		)
	}

	_, crossTenantErr := store.ListExecutionErrors(
		ctx,
		companyB,
		workflowExecutionID,
		firstRequest,
	)
	if !repository.IsNotFound(
		crossTenantErr,
	) {
		t.Fatalf(
			"cross-tenant ListExecutionErrors() error = %v, want NOT_FOUND",
			crossTenantErr,
		)
	}

	_, missingErr := store.ListExecutionErrors(
		ctx,
		companyA,
		execution.WorkflowExecutionID(
			"missing-execution-"+suffix,
		),
		firstRequest,
	)
	if !repository.IsNotFound(
		missingErr,
	) {
		t.Fatalf(
			"missing ListExecutionErrors() error = %v, want NOT_FOUND",
			missingErr,
		)
	}
}

func newExecutionErrorReadRecord(
	t *testing.T,
	params repository.ExecutionErrorRecordParams,
) repository.ExecutionErrorRecord {
	t.Helper()

	record, err := repository.NewExecutionErrorRecord(
		params,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an error: %v",
			err,
		)
	}

	return record
}

func insertExecutionErrorReadFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	record repository.ExecutionErrorRecord,
) {
	t.Helper()

	_, err := store.pool.Exec(
		ctx,
		`
INSERT INTO execution_errors (
	error_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	related_event_id,
	category,
	code,
	safe_message,
	technical_detail,
	retryable,
	details,
	created_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10,
	$11::jsonb,
	$12
)
`,
		record.ID().String(),
		record.WorkflowExecutionID().String(),
		record.CompanyID().String(),
		executionErrorOptionalNodeIDValue(
			record,
		),
		executionErrorOptionalEventIDValue(
			record,
		),
		record.Category().String(),
		record.Code(),
		record.SafeMessage(),
		executionErrorOptionalTechnicalDetailValue(
			record,
		),
		record.Retryable(),
		record.Details().String(),
		record.CreatedAt(),
	)
	if err != nil {
		t.Fatalf(
			"insert execution error fixture: %v",
			err,
		)
	}
}

func executionErrorOptionalNodeIDValue(
	record repository.ExecutionErrorRecord,
) any {
	value, exists := record.NodeExecutionID()
	if !exists {
		return nil
	}

	return value.String()
}

func executionErrorOptionalEventIDValue(
	record repository.ExecutionErrorRecord,
) any {
	value, exists := record.RelatedEventID()
	if !exists {
		return nil
	}

	return value.String()
}

func executionErrorOptionalTechnicalDetailValue(
	record repository.ExecutionErrorRecord,
) any {
	value, exists := record.TechnicalDetail()
	if !exists {
		return nil
	}

	return value
}

func assertExecutionErrorPageIDs(
	t *testing.T,
	records []repository.ExecutionErrorRecord,
	expected []repository.ExecutionErrorID,
) {
	t.Helper()

	if len(records) != len(expected) {
		t.Fatalf(
			"record count = %d, want %d",
			len(records),
			len(expected),
		)
	}

	for index, expectedID := range expected {
		if records[index].ID() != expectedID {
			t.Fatalf(
				"record[%d] ID = %q, want %q",
				index,
				records[index].ID(),
				expectedID,
			)
		}
	}
}

func assertExecutionErrorRecordsEqual(
	t *testing.T,
	actual repository.ExecutionErrorRecord,
	expected repository.ExecutionErrorRecord,
) {
	t.Helper()

	if actual.ID() != expected.ID() {
		t.Fatalf(
			"error ID = %q, want %q",
			actual.ID(),
			expected.ID(),
		)
	}

	if actual.WorkflowExecutionID() !=
		expected.WorkflowExecutionID() {
		t.Fatalf(
			"workflow execution ID = %q, want %q",
			actual.WorkflowExecutionID(),
			expected.WorkflowExecutionID(),
		)
	}

	if actual.CompanyID() != expected.CompanyID() {
		t.Fatalf(
			"company ID = %q, want %q",
			actual.CompanyID(),
			expected.CompanyID(),
		)
	}

	actualNodeID, actualHasNodeID :=
		actual.NodeExecutionID()

	expectedNodeID, expectedHasNodeID :=
		expected.NodeExecutionID()

	if actualHasNodeID != expectedHasNodeID ||
		actualNodeID != expectedNodeID {
		t.Fatalf(
			"node execution ID = %q, %t; want %q, %t",
			actualNodeID,
			actualHasNodeID,
			expectedNodeID,
			expectedHasNodeID,
		)
	}

	actualEventID, actualHasEventID :=
		actual.RelatedEventID()

	expectedEventID, expectedHasEventID :=
		expected.RelatedEventID()

	if actualHasEventID != expectedHasEventID ||
		actualEventID != expectedEventID {
		t.Fatalf(
			"related event ID = %q, %t; want %q, %t",
			actualEventID,
			actualHasEventID,
			expectedEventID,
			expectedHasEventID,
		)
	}

	if actual.Category() != expected.Category() {
		t.Fatalf(
			"category = %q, want %q",
			actual.Category(),
			expected.Category(),
		)
	}

	if actual.Code() != expected.Code() {
		t.Fatalf(
			"code = %q, want %q",
			actual.Code(),
			expected.Code(),
		)
	}

	if actual.SafeMessage() != expected.SafeMessage() {
		t.Fatalf(
			"safe message = %q, want %q",
			actual.SafeMessage(),
			expected.SafeMessage(),
		)
	}

	actualTechnical, actualHasTechnical :=
		actual.TechnicalDetail()

	expectedTechnical, expectedHasTechnical :=
		expected.TechnicalDetail()

	if actualHasTechnical != expectedHasTechnical ||
		actualTechnical != expectedTechnical {
		t.Fatalf(
			"technical detail = %q, %t; want %q, %t",
			actualTechnical,
			actualHasTechnical,
			expectedTechnical,
			expectedHasTechnical,
		)
	}

	if actual.Retryable() != expected.Retryable() {
		t.Fatalf(
			"retryable = %t, want %t",
			actual.Retryable(),
			expected.Retryable(),
		)
	}

	assertEquivalentJSON(
		t,
		actual.Details().Bytes(),
		expected.Details().Bytes(),
	)

	if !actual.CreatedAt().Equal(
		expected.CreatedAt(),
	) {
		t.Fatalf(
			"createdAt = %v, want %v",
			actual.CreatedAt(),
			expected.CreatedAt(),
		)
	}
}

func TestExecutionErrorReaderContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.ExecutionErrorReader = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement ExecutionErrorReader",
		)
	}
}

func TestExecutionErrorReaderRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionErrors(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionErrors() returned nil error for nil store",
		)
	}
}

func TestExecutionErrorReaderRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedExecutionErrorReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionErrors(
		nil,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionErrors() accepted nil context",
		)
	}
}

func TestExecutionErrorReaderPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedExecutionErrorReaderTestStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionErrors(
		ctx,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"ListExecutionErrors() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestExecutionErrorReaderRejectsInvalidIdentifiers(
	t *testing.T,
) {
	store := newUnconnectedExecutionErrorReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	tests := []struct {
		name                string
		companyID           workflow.CompanyID
		workflowExecutionID execution.WorkflowExecutionID
	}{
		{
			name:      "blank company ID",
			companyID: workflow.CompanyID(" "),
			workflowExecutionID: execution.WorkflowExecutionID(
				"execution-1",
			),
		},
		{
			name: "blank workflow execution ID",
			companyID: workflow.CompanyID(
				"company-1",
			),
			workflowExecutionID: execution.WorkflowExecutionID(" "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.ListExecutionErrors(
				context.Background(),
				test.companyID,
				test.workflowExecutionID,
				pageRequest,
			)
			if err == nil {
				t.Fatal(
					"ListExecutionErrors() accepted invalid identifiers",
				)
			}
		})
	}
}

func TestExecutionErrorReaderRejectsMalformedCursor(
	t *testing.T,
) {
	store := newUnconnectedExecutionErrorReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		repository.PageToken("***"),
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionErrors(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionErrors() accepted malformed cursor",
		)
	}

	var validationError *repository.ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error = %T, want *repository.ValidationError",
			err,
		)
	}

	if validationError.Field != "pageToken" {
		t.Fatalf(
			"validation field = %q, want pageToken",
			validationError.Field,
		)
	}
}

func TestBuildExecutionErrorListQueryUsesTenantScopeAndCursor(
	t *testing.T,
) {
	cursorTime := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	cursor, err := encodeTimestampCursor(
		cursorKindExecutionErrors,
		cursorTime,
		"error-9",
	)
	if err != nil {
		t.Fatalf(
			"encodeTimestampCursor() returned an unexpected error: %v",
			err,
		)
	}

	pageRequest, err := repository.NewPageRequest(
		25,
		cursor,
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	query, arguments, err := buildExecutionErrorListQuery(
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err != nil {
		t.Fatalf(
			"buildExecutionErrorListQuery() returned an unexpected error: %v",
			err,
		)
	}

	requiredFragments := []string{
		"WHERE company_id = $1",
		"workflow_execution_id = $2",
		"(created_at, error_id) > ($3, $4)",
		"ORDER BY created_at ASC, error_id ASC",
		"LIMIT $5",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(
			query,
			fragment,
		) {
			t.Fatalf(
				"query does not contain %q:\n%s",
				fragment,
				query,
			)
		}
	}

	if len(arguments) != 5 {
		t.Fatalf(
			"argument count = %d, want 5",
			len(arguments),
		)
	}

	if arguments[0] != "company-1" {
		t.Fatalf(
			"company argument = %#v",
			arguments[0],
		)
	}

	if arguments[1] != "execution-1" {
		t.Fatalf(
			"workflow execution argument = %#v",
			arguments[1],
		)
	}

	if arguments[3] != "error-9" {
		t.Fatalf(
			"error cursor argument = %#v",
			arguments[3],
		)
	}

	if arguments[4] != 26 {
		t.Fatalf(
			"limit argument = %#v, want 26",
			arguments[4],
		)
	}
}

func newUnconnectedExecutionErrorReaderTestStore(
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

func TestExecutionEventReaderIntegration(
	t *testing.T,
) {
	ctx, store := requirePostgreSQLIntegrationStore(t)

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

	companyA := workflow.CompanyID(
		"company-event-reader-a-" + suffix,
	)

	companyB := workflow.CompanyID(
		"company-event-reader-b-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-event-reader-" + suffix,
	)

	workflowExecutionID := execution.WorkflowExecutionID(
		"execution-event-reader-" + suffix,
	)

	nodeExecutionID := execution.NodeExecutionID(
		"node-execution-event-reader-" + suffix,
	)

	snapshot := newWorkflowExecutionReadSnapshot(
		t,
		companyA,
		workflowID,
		repository.DefinitionSnapshotID(
			"snapshot-event-reader-"+suffix,
		),
	)

	workflowCreatedAt := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	workflowExecution := newWorkflowExecutionReadRecord(
		t,
		workflowExecutionID,
		companyA,
		workflowID,
		snapshot.ID(),
		execution.WorkflowExecutionStatusRunning,
		workflowCreatedAt,
	)

	nodeExecution := newNodeExecutionReadRecord(
		t,
		nodeExecutionID,
		workflowExecutionID,
		companyA,
		workflow.NodeID("node-1"),
		execution.NodeExecutionStatusRunning,
		workflowCreatedAt.Add(time.Minute),
	)

	t.Cleanup(func() {
		cleanupContext, cancelCleanup := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancelCleanup()

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM execution_events
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"execution event cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM node_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"node execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"workflow execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_definition_snapshots
WHERE company_id = $1
  AND snapshot_id = $2
`,
			companyA.String(),
			snapshot.ID().String(),
		); err != nil {
			t.Errorf(
				"snapshot cleanup failed: %v",
				err,
			)
		}
	})

	if err := store.Create(
		ctx,
		snapshot,
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
		workflowExecution,
	)

	insertNodeExecutionReadFixture(
		t,
		ctx,
		store,
		nodeExecution,
	)

	events := []repository.ExecutionEventRecord{
		newExecutionEventReadRecord(
			t,
			repository.ExecutionEventRecordParams{
				ID: repository.ExecutionEventID(
					"event-1-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				SequenceNumber:      repository.SequenceNumber(1),
				Type:                repository.ExecutionEventTypeWorkflowCreated,
				NewStatus:           execution.WorkflowExecutionStatusCreated.String(),
				CorrelationID:       "correlation-" + suffix,
				CausationID:         "request-" + suffix,
				SafeMessage:         "Workflow execution created",
				Metadata: []byte(
					`{"source":"integration-test"}`,
				),
				CreatedAt: workflowCreatedAt,
			},
		),
		newExecutionEventReadRecord(
			t,
			repository.ExecutionEventRecordParams{
				ID: repository.ExecutionEventID(
					"event-2-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				SequenceNumber:      repository.SequenceNumber(2),
				Type:                repository.ExecutionEventTypeWorkflowStarted,
				PreviousStatus:      execution.WorkflowExecutionStatusValidating.String(),
				NewStatus:           execution.WorkflowExecutionStatusRunning.String(),
				CorrelationID:       "correlation-" + suffix,
				CausationID:         "validation-" + suffix,
				SafeMessage:         "Workflow execution started",
				Metadata: []byte(
					`{"mode":"SYNC"}`,
				),
				CreatedAt: workflowCreatedAt.Add(
					10 * time.Second,
				),
			},
		),
		newExecutionEventReadRecord(
			t,
			repository.ExecutionEventRecordParams{
				ID: repository.ExecutionEventID(
					"event-3-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				NodeExecutionID:     nodeExecutionID,
				SequenceNumber:      repository.SequenceNumber(3),
				Type:                repository.ExecutionEventTypeNodeCreated,
				NewStatus:           execution.NodeExecutionStatusPending.String(),
				CorrelationID:       "correlation-" + suffix,
				CausationID:         "workflow-start-" + suffix,
				SafeMessage:         "Node execution created",
				Metadata: []byte(
					`{"nodeId":"node-1"}`,
				),
				CreatedAt: nodeExecution.CreatedAt(),
			},
		),
		newExecutionEventReadRecord(
			t,
			repository.ExecutionEventRecordParams{
				ID: repository.ExecutionEventID(
					"event-4-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				NodeExecutionID:     nodeExecutionID,
				SequenceNumber:      repository.SequenceNumber(4),
				Type:                repository.ExecutionEventTypeNodeStarted,
				PreviousStatus:      execution.NodeExecutionStatusReady.String(),
				NewStatus:           execution.NodeExecutionStatusRunning.String(),
				CorrelationID:       "correlation-" + suffix,
				CausationID:         "node-ready-" + suffix,
				SafeMessage:         "Node execution started",
				Metadata: []byte(
					`{"pluginType":"core.pass-through"}`,
				),
				CreatedAt: nodeExecution.UpdatedAt(),
			},
		),
	}

	for _, event := range events {
		insertExecutionEventReadFixture(
			t,
			ctx,
			store,
			event,
		)
	}

	firstRequest, err := repository.NewPageRequest(
		2,
		"",
	)
	if err != nil {
		t.Fatalf(
			"create first page request: %v",
			err,
		)
	}

	firstPage, err := store.ListExecutionEvents(
		ctx,
		companyA,
		workflowExecutionID,
		firstRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListExecutionEvents() first page error: %v",
			err,
		)
	}

	assertExecutionEventPageIDs(
		t,
		firstPage.Items(),
		[]repository.ExecutionEventID{
			events[0].ID(),
			events[1].ID(),
		},
	)

	assertExecutionEventRecordsEqual(
		t,
		firstPage.Items()[1],
		events[1],
	)

	nextToken, exists := firstPage.Next()
	if !exists {
		t.Fatal(
			"first execution event page has no next token",
		)
	}

	decodedSequence, err := decodeSequenceCursor(
		cursorKindExecutionEvents,
		nextToken,
	)
	if err != nil {
		t.Fatalf(
			"decode first page next token: %v",
			err,
		)
	}

	if decodedSequence != repository.SequenceNumber(2) {
		t.Fatalf(
			"decoded next sequence = %d, want 2",
			decodedSequence,
		)
	}

	secondRequest, err := repository.NewPageRequest(
		2,
		nextToken,
	)
	if err != nil {
		t.Fatalf(
			"create second page request: %v",
			err,
		)
	}

	secondPage, err := store.ListExecutionEvents(
		ctx,
		companyA,
		workflowExecutionID,
		secondRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListExecutionEvents() second page error: %v",
			err,
		)
	}

	assertExecutionEventPageIDs(
		t,
		secondPage.Items(),
		[]repository.ExecutionEventID{
			events[2].ID(),
			events[3].ID(),
		},
	)

	assertExecutionEventRecordsEqual(
		t,
		secondPage.Items()[0],
		events[2],
	)

	assertExecutionEventRecordsEqual(
		t,
		secondPage.Items()[1],
		events[3],
	)

	if secondPage.HasNext() {
		t.Fatal(
			"second execution event page unexpectedly has a next token",
		)
	}

	_, crossTenantErr := store.ListExecutionEvents(
		ctx,
		companyB,
		workflowExecutionID,
		firstRequest,
	)
	if !repository.IsNotFound(
		crossTenantErr,
	) {
		t.Fatalf(
			"cross-tenant ListExecutionEvents() error = %v, want NOT_FOUND",
			crossTenantErr,
		)
	}

	_, missingErr := store.ListExecutionEvents(
		ctx,
		companyA,
		execution.WorkflowExecutionID(
			"missing-execution-"+suffix,
		),
		firstRequest,
	)
	if !repository.IsNotFound(
		missingErr,
	) {
		t.Fatalf(
			"missing ListExecutionEvents() error = %v, want NOT_FOUND",
			missingErr,
		)
	}
}

func newExecutionEventReadRecord(
	t *testing.T,
	params repository.ExecutionEventRecordParams,
) repository.ExecutionEventRecord {
	t.Helper()

	record, err := repository.NewExecutionEventRecord(
		params,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventRecord() returned an error: %v",
			err,
		)
	}

	return record
}

func insertExecutionEventReadFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	record repository.ExecutionEventRecord,
) {
	t.Helper()

	_, err := store.pool.Exec(
		ctx,
		`
INSERT INTO execution_events (
	event_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	sequence_number,
	event_type,
	previous_status,
	new_status,
	correlation_id,
	causation_id,
	safe_message,
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
	$7,
	$8,
	$9,
	$10,
	$11,
	$12::jsonb,
	$13
)
`,
		record.ID().String(),
		record.WorkflowExecutionID().String(),
		record.CompanyID().String(),
		executionEventOptionalNodeIDValue(
			record,
		),
		record.SequenceNumber().Int64(),
		record.Type().String(),
		executionEventOptionalStringValue(
			record.PreviousStatus,
		),
		executionEventOptionalStringValue(
			record.NewStatus,
		),
		executionEventOptionalStringValue(
			record.CorrelationID,
		),
		executionEventOptionalStringValue(
			record.CausationID,
		),
		executionEventOptionalStringValue(
			record.SafeMessage,
		),
		record.Metadata().String(),
		record.CreatedAt(),
	)
	if err != nil {
		t.Fatalf(
			"insert execution event fixture: %v",
			err,
		)
	}
}

func executionEventOptionalNodeIDValue(
	record repository.ExecutionEventRecord,
) any {
	value, exists := record.NodeExecutionID()
	if !exists {
		return nil
	}

	return value.String()
}

func executionEventOptionalStringValue(
	getter func() (string, bool),
) any {
	value, exists := getter()
	if !exists {
		return nil
	}

	return value
}

func assertExecutionEventPageIDs(
	t *testing.T,
	records []repository.ExecutionEventRecord,
	expected []repository.ExecutionEventID,
) {
	t.Helper()

	if len(records) != len(expected) {
		t.Fatalf(
			"record count = %d, want %d",
			len(records),
			len(expected),
		)
	}

	for index, expectedID := range expected {
		if records[index].ID() != expectedID {
			t.Fatalf(
				"record[%d] ID = %q, want %q",
				index,
				records[index].ID(),
				expectedID,
			)
		}
	}
}

func assertExecutionEventRecordsEqual(
	t *testing.T,
	actual repository.ExecutionEventRecord,
	expected repository.ExecutionEventRecord,
) {
	t.Helper()

	if actual.ID() != expected.ID() {
		t.Fatalf(
			"event ID = %q, want %q",
			actual.ID(),
			expected.ID(),
		)
	}

	if actual.WorkflowExecutionID() !=
		expected.WorkflowExecutionID() {
		t.Fatalf(
			"workflow execution ID = %q, want %q",
			actual.WorkflowExecutionID(),
			expected.WorkflowExecutionID(),
		)
	}

	if actual.CompanyID() != expected.CompanyID() {
		t.Fatalf(
			"company ID = %q, want %q",
			actual.CompanyID(),
			expected.CompanyID(),
		)
	}

	actualNodeID, actualHasNodeID :=
		actual.NodeExecutionID()

	expectedNodeID, expectedHasNodeID :=
		expected.NodeExecutionID()

	if actualHasNodeID != expectedHasNodeID ||
		actualNodeID != expectedNodeID {
		t.Fatalf(
			"node execution ID = %q, %t; want %q, %t",
			actualNodeID,
			actualHasNodeID,
			expectedNodeID,
			expectedHasNodeID,
		)
	}

	if actual.SequenceNumber() !=
		expected.SequenceNumber() {
		t.Fatalf(
			"sequence number = %d, want %d",
			actual.SequenceNumber(),
			expected.SequenceNumber(),
		)
	}

	if actual.Type() != expected.Type() {
		t.Fatalf(
			"event type = %q, want %q",
			actual.Type(),
			expected.Type(),
		)
	}

	assertExecutionEventOptionalString(
		t,
		"previousStatus",
		actual.PreviousStatus,
		expected.PreviousStatus,
	)

	assertExecutionEventOptionalString(
		t,
		"newStatus",
		actual.NewStatus,
		expected.NewStatus,
	)

	assertExecutionEventOptionalString(
		t,
		"correlationID",
		actual.CorrelationID,
		expected.CorrelationID,
	)

	assertExecutionEventOptionalString(
		t,
		"causationID",
		actual.CausationID,
		expected.CausationID,
	)

	assertExecutionEventOptionalString(
		t,
		"safeMessage",
		actual.SafeMessage,
		expected.SafeMessage,
	)

	if !actual.CreatedAt().Equal(
		expected.CreatedAt(),
	) {
		t.Fatalf(
			"createdAt = %v, want %v",
			actual.CreatedAt(),
			expected.CreatedAt(),
		)
	}

	var actualMetadata any

	if err := json.Unmarshal(
		actual.Metadata().Bytes(),
		&actualMetadata,
	); err != nil {
		t.Fatalf(
			"actual metadata JSON is invalid: %v",
			err,
		)
	}

	var expectedMetadata any

	if err := json.Unmarshal(
		expected.Metadata().Bytes(),
		&expectedMetadata,
	); err != nil {
		t.Fatalf(
			"expected metadata JSON is invalid: %v",
			err,
		)
	}

	if !reflect.DeepEqual(
		actualMetadata,
		expectedMetadata,
	) {
		t.Fatalf(
			"metadata differs\nactual: %s\nexpected: %s",
			actual.Metadata().Bytes(),
			expected.Metadata().Bytes(),
		)
	}
}

func assertExecutionEventOptionalString(
	t *testing.T,
	field string,
	actualGetter func() (string, bool),
	expectedGetter func() (string, bool),
) {
	t.Helper()

	actual, actualExists := actualGetter()
	expected, expectedExists := expectedGetter()

	if actualExists != expectedExists {
		t.Fatalf(
			"%s exists = %t, want %t",
			field,
			actualExists,
			expectedExists,
		)
	}

	if actualExists && actual != expected {
		t.Fatalf(
			"%s = %q, want %q",
			field,
			actual,
			expected,
		)
	}
}

func TestExecutionEventReaderContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.ExecutionEventReader = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement ExecutionEventReader",
		)
	}
}

func TestExecutionEventReaderRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionEvents(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionEvents() returned nil error for nil store",
		)
	}
}

func TestExecutionEventReaderRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedExecutionEventReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionEvents(
		nil,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionEvents() accepted nil context",
		)
	}
}

func TestExecutionEventReaderPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedExecutionEventReaderTestStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionEvents(
		ctx,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"ListExecutionEvents() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestExecutionEventReaderRejectsInvalidIdentifiers(
	t *testing.T,
) {
	store := newUnconnectedExecutionEventReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	tests := []struct {
		name                string
		companyID           workflow.CompanyID
		workflowExecutionID execution.WorkflowExecutionID
	}{
		{
			name:      "blank company ID",
			companyID: workflow.CompanyID(" "),
			workflowExecutionID: execution.WorkflowExecutionID(
				"execution-1",
			),
		},
		{
			name: "blank workflow execution ID",
			companyID: workflow.CompanyID(
				"company-1",
			),
			workflowExecutionID: execution.WorkflowExecutionID(" "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.ListExecutionEvents(
				context.Background(),
				test.companyID,
				test.workflowExecutionID,
				pageRequest,
			)
			if err == nil {
				t.Fatal(
					"ListExecutionEvents() accepted invalid identifiers",
				)
			}
		})
	}
}

func TestExecutionEventReaderRejectsMalformedCursor(
	t *testing.T,
) {
	store := newUnconnectedExecutionEventReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		repository.PageToken("***"),
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionEvents(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionEvents() accepted malformed cursor",
		)
	}

	var validationError *repository.ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error = %T, want *repository.ValidationError",
			err,
		)
	}

	if validationError.Field != "pageToken" {
		t.Fatalf(
			"validation field = %q, want pageToken",
			validationError.Field,
		)
	}
}

func TestBuildExecutionEventListQueryUsesTenantScopeAndSequenceCursor(
	t *testing.T,
) {
	cursor, err := encodeSequenceCursor(
		cursorKindExecutionEvents,
		repository.SequenceNumber(19),
	)
	if err != nil {
		t.Fatalf(
			"encodeSequenceCursor() returned an unexpected error: %v",
			err,
		)
	}

	pageRequest, err := repository.NewPageRequest(
		25,
		cursor,
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	query, arguments, err := buildExecutionEventListQuery(
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err != nil {
		t.Fatalf(
			"buildExecutionEventListQuery() returned an unexpected error: %v",
			err,
		)
	}

	requiredFragments := []string{
		"WHERE company_id = $1",
		"workflow_execution_id = $2",
		"sequence_number > $3",
		"ORDER BY sequence_number ASC",
		"LIMIT $4",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(
			query,
			fragment,
		) {
			t.Fatalf(
				"query does not contain %q:\n%s",
				fragment,
				query,
			)
		}
	}

	if len(arguments) != 4 {
		t.Fatalf(
			"argument count = %d, want 4",
			len(arguments),
		)
	}

	if arguments[0] != "company-1" {
		t.Fatalf(
			"company argument = %#v",
			arguments[0],
		)
	}

	if arguments[1] != "execution-1" {
		t.Fatalf(
			"workflow execution argument = %#v",
			arguments[1],
		)
	}

	if arguments[2] != int64(19) {
		t.Fatalf(
			"sequence cursor argument = %#v, want 19",
			arguments[2],
		)
	}

	if arguments[3] != 26 {
		t.Fatalf(
			"limit argument = %#v, want 26",
			arguments[3],
		)
	}
}

func newUnconnectedExecutionEventReaderTestStore(
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

func TestExecutionLogReaderIntegration(
	t *testing.T,
) {
	ctx, store := requirePostgreSQLIntegrationStore(t)

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
		"execution_logs",
	)

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	companyA := workflow.CompanyID(
		"company-log-reader-a-" + suffix,
	)

	companyB := workflow.CompanyID(
		"company-log-reader-b-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-log-reader-" + suffix,
	)

	workflowExecutionID := execution.WorkflowExecutionID(
		"execution-log-reader-" + suffix,
	)

	nodeExecutionID := execution.NodeExecutionID(
		"node-execution-log-reader-" + suffix,
	)

	snapshot := newWorkflowExecutionReadSnapshot(
		t,
		companyA,
		workflowID,
		repository.DefinitionSnapshotID(
			"snapshot-log-reader-"+suffix,
		),
	)

	workflowCreatedAt := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	workflowExecution := newWorkflowExecutionReadRecord(
		t,
		workflowExecutionID,
		companyA,
		workflowID,
		snapshot.ID(),
		execution.WorkflowExecutionStatusRunning,
		workflowCreatedAt,
	)

	nodeExecution := newNodeExecutionReadRecord(
		t,
		nodeExecutionID,
		workflowExecutionID,
		companyA,
		workflow.NodeID("node-1"),
		execution.NodeExecutionStatusRunning,
		workflowCreatedAt.Add(time.Minute),
	)

	t.Cleanup(func() {
		cleanupContext, cancelCleanup := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancelCleanup()

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM execution_logs
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"execution log cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM node_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"node execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"workflow execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_definition_snapshots
WHERE company_id = $1
  AND snapshot_id = $2
`,
			companyA.String(),
			snapshot.ID().String(),
		); err != nil {
			t.Errorf(
				"snapshot cleanup failed: %v",
				err,
			)
		}
	})

	if err := store.Create(
		ctx,
		snapshot,
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
		workflowExecution,
	)

	insertNodeExecutionReadFixture(
		t,
		ctx,
		store,
		nodeExecution,
	)

	logs := []repository.ExecutionLogRecord{
		newExecutionLogReadRecord(
			t,
			repository.ExecutionLogRecordParams{
				ID: repository.ExecutionLogID(
					"log-1-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				SequenceNumber:      repository.SequenceNumber(1),
				Level:               repository.ExecutionLogLevelDebug,
				Message:             "Workflow execution preparation started",
				Metadata: []byte(
					`{"phase":"preparation"}`,
				),
				CreatedAt: workflowCreatedAt,
			},
		),
		newExecutionLogReadRecord(
			t,
			repository.ExecutionLogRecordParams{
				ID: repository.ExecutionLogID(
					"log-2-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				SequenceNumber:      repository.SequenceNumber(2),
				Level:               repository.ExecutionLogLevelInfo,
				Message:             "Workflow execution started",
				Metadata: []byte(
					`{"mode":"SYNC"}`,
				),
				CreatedAt: workflowCreatedAt.Add(
					10 * time.Second,
				),
			},
		),
		newExecutionLogReadRecord(
			t,
			repository.ExecutionLogRecordParams{
				ID: repository.ExecutionLogID(
					"log-3-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				NodeExecutionID:     nodeExecutionID,
				SequenceNumber:      repository.SequenceNumber(3),
				Level:               repository.ExecutionLogLevelWarn,
				Message:             "Node execution is using a test warning",
				Metadata: []byte(
					`{"nodeId":"node-1"}`,
				),
				CreatedAt: nodeExecution.CreatedAt(),
			},
		),
		newExecutionLogReadRecord(
			t,
			repository.ExecutionLogRecordParams{
				ID: repository.ExecutionLogID(
					"log-4-" + suffix,
				),
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyA,
				NodeExecutionID:     nodeExecutionID,
				SequenceNumber:      repository.SequenceNumber(4),
				Level:               repository.ExecutionLogLevelError,
				Message:             "Node execution produced a controlled test error",
				Metadata: []byte(
					`{"code":"CONTROLLED_TEST_ERROR"}`,
				),
				CreatedAt: nodeExecution.UpdatedAt(),
			},
		),
	}

	for _, logRecord := range logs {
		insertExecutionLogReadFixture(
			t,
			ctx,
			store,
			logRecord,
		)
	}

	firstRequest, err := repository.NewPageRequest(
		2,
		"",
	)
	if err != nil {
		t.Fatalf(
			"create first page request: %v",
			err,
		)
	}

	firstPage, err := store.ListExecutionLogs(
		ctx,
		companyA,
		workflowExecutionID,
		firstRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListExecutionLogs() first page error: %v",
			err,
		)
	}

	assertExecutionLogPageIDs(
		t,
		firstPage.Items(),
		[]repository.ExecutionLogID{
			logs[0].ID(),
			logs[1].ID(),
		},
	)

	assertExecutionLogRecordsEqual(
		t,
		firstPage.Items()[0],
		logs[0],
	)

	assertExecutionLogRecordsEqual(
		t,
		firstPage.Items()[1],
		logs[1],
	)

	nextToken, exists := firstPage.Next()
	if !exists {
		t.Fatal(
			"first execution log page has no next token",
		)
	}

	decodedSequence, err := decodeSequenceCursor(
		cursorKindExecutionLogs,
		nextToken,
	)
	if err != nil {
		t.Fatalf(
			"decode first page next token: %v",
			err,
		)
	}

	if decodedSequence != repository.SequenceNumber(2) {
		t.Fatalf(
			"decoded next sequence = %d, want 2",
			decodedSequence,
		)
	}

	secondRequest, err := repository.NewPageRequest(
		2,
		nextToken,
	)
	if err != nil {
		t.Fatalf(
			"create second page request: %v",
			err,
		)
	}

	secondPage, err := store.ListExecutionLogs(
		ctx,
		companyA,
		workflowExecutionID,
		secondRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListExecutionLogs() second page error: %v",
			err,
		)
	}

	assertExecutionLogPageIDs(
		t,
		secondPage.Items(),
		[]repository.ExecutionLogID{
			logs[2].ID(),
			logs[3].ID(),
		},
	)

	assertExecutionLogRecordsEqual(
		t,
		secondPage.Items()[0],
		logs[2],
	)

	assertExecutionLogRecordsEqual(
		t,
		secondPage.Items()[1],
		logs[3],
	)

	if secondPage.HasNext() {
		t.Fatal(
			"second execution log page unexpectedly has a next token",
		)
	}

	_, crossTenantErr := store.ListExecutionLogs(
		ctx,
		companyB,
		workflowExecutionID,
		firstRequest,
	)
	if !repository.IsNotFound(
		crossTenantErr,
	) {
		t.Fatalf(
			"cross-tenant ListExecutionLogs() error = %v, want NOT_FOUND",
			crossTenantErr,
		)
	}

	_, missingErr := store.ListExecutionLogs(
		ctx,
		companyA,
		execution.WorkflowExecutionID(
			"missing-execution-"+suffix,
		),
		firstRequest,
	)
	if !repository.IsNotFound(
		missingErr,
	) {
		t.Fatalf(
			"missing ListExecutionLogs() error = %v, want NOT_FOUND",
			missingErr,
		)
	}
}

func newExecutionLogReadRecord(
	t *testing.T,
	params repository.ExecutionLogRecordParams,
) repository.ExecutionLogRecord {
	t.Helper()

	record, err := repository.NewExecutionLogRecord(
		params,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogRecord() returned an error: %v",
			err,
		)
	}

	return record
}

func insertExecutionLogReadFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	record repository.ExecutionLogRecord,
) {
	t.Helper()

	_, err := store.pool.Exec(
		ctx,
		`
INSERT INTO execution_logs (
	log_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
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
	$7,
	$8::jsonb,
	$9
)
`,
		record.ID().String(),
		record.WorkflowExecutionID().String(),
		record.CompanyID().String(),
		executionLogOptionalNodeIDValue(
			record,
		),
		record.SequenceNumber().Int64(),
		record.Level().String(),
		record.Message(),
		record.Metadata().String(),
		record.CreatedAt(),
	)
	if err != nil {
		t.Fatalf(
			"insert execution log fixture: %v",
			err,
		)
	}
}

func executionLogOptionalNodeIDValue(
	record repository.ExecutionLogRecord,
) any {
	value, exists := record.NodeExecutionID()
	if !exists {
		return nil
	}

	return value.String()
}

func assertExecutionLogPageIDs(
	t *testing.T,
	records []repository.ExecutionLogRecord,
	expected []repository.ExecutionLogID,
) {
	t.Helper()

	if len(records) != len(expected) {
		t.Fatalf(
			"record count = %d, want %d",
			len(records),
			len(expected),
		)
	}

	for index, expectedID := range expected {
		if records[index].ID() != expectedID {
			t.Fatalf(
				"record[%d] ID = %q, want %q",
				index,
				records[index].ID(),
				expectedID,
			)
		}
	}
}

func assertExecutionLogRecordsEqual(
	t *testing.T,
	actual repository.ExecutionLogRecord,
	expected repository.ExecutionLogRecord,
) {
	t.Helper()

	if actual.ID() != expected.ID() {
		t.Fatalf(
			"log ID = %q, want %q",
			actual.ID(),
			expected.ID(),
		)
	}

	if actual.WorkflowExecutionID() !=
		expected.WorkflowExecutionID() {
		t.Fatalf(
			"workflow execution ID = %q, want %q",
			actual.WorkflowExecutionID(),
			expected.WorkflowExecutionID(),
		)
	}

	if actual.CompanyID() != expected.CompanyID() {
		t.Fatalf(
			"company ID = %q, want %q",
			actual.CompanyID(),
			expected.CompanyID(),
		)
	}

	actualNodeID, actualHasNodeID :=
		actual.NodeExecutionID()

	expectedNodeID, expectedHasNodeID :=
		expected.NodeExecutionID()

	if actualHasNodeID != expectedHasNodeID ||
		actualNodeID != expectedNodeID {
		t.Fatalf(
			"node execution ID = %q, %t; want %q, %t",
			actualNodeID,
			actualHasNodeID,
			expectedNodeID,
			expectedHasNodeID,
		)
	}

	if actual.SequenceNumber() !=
		expected.SequenceNumber() {
		t.Fatalf(
			"sequence number = %d, want %d",
			actual.SequenceNumber(),
			expected.SequenceNumber(),
		)
	}

	if actual.Level() != expected.Level() {
		t.Fatalf(
			"log level = %q, want %q",
			actual.Level(),
			expected.Level(),
		)
	}

	if actual.Message() != expected.Message() {
		t.Fatalf(
			"log message = %q, want %q",
			actual.Message(),
			expected.Message(),
		)
	}

	assertEquivalentJSON(
		t,
		actual.Metadata().Bytes(),
		expected.Metadata().Bytes(),
	)

	if !actual.CreatedAt().Equal(
		expected.CreatedAt(),
	) {
		t.Fatalf(
			"createdAt = %v, want %v",
			actual.CreatedAt(),
			expected.CreatedAt(),
		)
	}
}

func TestExecutionLogReaderContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.ExecutionLogReader = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement ExecutionLogReader",
		)
	}
}

func TestExecutionLogReaderRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionLogs(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionLogs() returned nil error for nil store",
		)
	}
}

func TestExecutionLogReaderRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedExecutionLogReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionLogs(
		nil,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionLogs() accepted nil context",
		)
	}
}

func TestExecutionLogReaderPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedExecutionLogReaderTestStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionLogs(
		ctx,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"ListExecutionLogs() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestExecutionLogReaderRejectsInvalidIdentifiers(
	t *testing.T,
) {
	store := newUnconnectedExecutionLogReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	tests := []struct {
		name                string
		companyID           workflow.CompanyID
		workflowExecutionID execution.WorkflowExecutionID
	}{
		{
			name:      "blank company ID",
			companyID: workflow.CompanyID(" "),
			workflowExecutionID: execution.WorkflowExecutionID(
				"execution-1",
			),
		},
		{
			name: "blank workflow execution ID",
			companyID: workflow.CompanyID(
				"company-1",
			),
			workflowExecutionID: execution.WorkflowExecutionID(" "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.ListExecutionLogs(
				context.Background(),
				test.companyID,
				test.workflowExecutionID,
				pageRequest,
			)
			if err == nil {
				t.Fatal(
					"ListExecutionLogs() accepted invalid identifiers",
				)
			}
		})
	}
}

func TestExecutionLogReaderRejectsMalformedCursor(
	t *testing.T,
) {
	store := newUnconnectedExecutionLogReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		repository.PageToken("***"),
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListExecutionLogs(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListExecutionLogs() accepted malformed cursor",
		)
	}

	var validationError *repository.ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error = %T, want *repository.ValidationError",
			err,
		)
	}

	if validationError.Field != "pageToken" {
		t.Fatalf(
			"validation field = %q, want pageToken",
			validationError.Field,
		)
	}
}

func TestBuildExecutionLogListQueryUsesTenantScopeAndSequenceCursor(
	t *testing.T,
) {
	cursor, err := encodeSequenceCursor(
		cursorKindExecutionLogs,
		repository.SequenceNumber(29),
	)
	if err != nil {
		t.Fatalf(
			"encodeSequenceCursor() returned an unexpected error: %v",
			err,
		)
	}

	pageRequest, err := repository.NewPageRequest(
		25,
		cursor,
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	query, arguments, err := buildExecutionLogListQuery(
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err != nil {
		t.Fatalf(
			"buildExecutionLogListQuery() returned an unexpected error: %v",
			err,
		)
	}

	requiredFragments := []string{
		"WHERE company_id = $1",
		"workflow_execution_id = $2",
		"sequence_number > $3",
		"ORDER BY sequence_number ASC",
		"LIMIT $4",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(
			query,
			fragment,
		) {
			t.Fatalf(
				"query does not contain %q:\n%s",
				fragment,
				query,
			)
		}
	}

	if len(arguments) != 4 {
		t.Fatalf(
			"argument count = %d, want 4",
			len(arguments),
		)
	}

	if arguments[0] != "company-1" {
		t.Fatalf(
			"company argument = %#v",
			arguments[0],
		)
	}

	if arguments[1] != "execution-1" {
		t.Fatalf(
			"workflow execution argument = %#v",
			arguments[1],
		)
	}

	if arguments[2] != int64(29) {
		t.Fatalf(
			"sequence cursor argument = %#v, want 29",
			arguments[2],
		)
	}

	if arguments[3] != 26 {
		t.Fatalf(
			"limit argument = %#v, want 26",
			arguments[3],
		)
	}
}

func newUnconnectedExecutionLogReaderTestStore(
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

const integrationHTTPIdempotencyFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestHTTPIdempotencyStoreIntegration(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(
			t,
		)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"http_idempotency_keys",
	)

	suffix :=
		fmt.Sprintf(
			"%d",
			time.Now().
				UTC().
				UnixNano(),
		)

	companyID :=
		workflow.CompanyID(
			"company-http-idempotency-" +
				suffix,
		)

	otherCompanyID :=
		workflow.CompanyID(
			"company-http-idempotency-other-" +
				suffix,
		)

	registerHTTPIdempotencyCleanup(
		t,
		store,
		companyID,
		otherCompanyID,
	)

	createdAt :=
		time.Now().
			UTC().
			Truncate(
				time.Microsecond,
			)

	reservation :=
		mustHTTPIdempotencyReservation(
			t,
			companyID,
			"request-1",
			integrationHTTPIdempotencyFingerprint,
			execution.WorkflowExecutionID(
				"execution-1-"+
					suffix,
			),
			createdAt,
		)

	record, created, err :=
		store.ReserveHTTPIdempotency(
			ctx,
			reservation,
		)
	if err != nil {
		t.Fatalf(
			"ReserveHTTPIdempotency() error = %v",
			err,
		)
	}

	if !created {
		t.Fatal(
			"first reservation was not created",
		)
	}

	if record.State() !=
		repository.HTTPIdempotencyStateReserved {
		t.Fatalf(
			"state = %q",
			record.State(),
		)
	}

	replayed, replayCreated, err :=
		store.ReserveHTTPIdempotency(
			ctx,
			reservation,
		)
	if err != nil {
		t.Fatalf(
			"repeated ReserveHTTPIdempotency() error = %v",
			err,
		)
	}

	if replayCreated {
		t.Fatal(
			"repeated reservation created a second record",
		)
	}

	if replayed.WorkflowExecutionID() !=
		record.WorkflowExecutionID() {
		t.Fatalf(
			"replayed execution ID = %q, want %q",
			replayed.WorkflowExecutionID(),
			record.WorkflowExecutionID(),
		)
	}

	otherCompanyReservation :=
		mustHTTPIdempotencyReservation(
			t,
			otherCompanyID,
			"request-1",
			integrationHTTPIdempotencyFingerprint,
			execution.WorkflowExecutionID(
				"execution-other-"+
					suffix,
			),
			createdAt,
		)

	_, otherCompanyCreated, err :=
		store.ReserveHTTPIdempotency(
			ctx,
			otherCompanyReservation,
		)
	if err != nil {
		t.Fatalf(
			"cross-company ReserveHTTPIdempotency() error = %v",
			err,
		)
	}

	if !otherCompanyCreated {
		t.Fatal(
			"same key in another company was not independent",
		)
	}

	acceptedAt :=
		createdAt.Add(
			time.Second,
		)

	acceptance :=
		mustHTTPIdempotencyAcceptance(
			t,
			record,
			acceptedAt,
		)

	accepted, err :=
		store.MarkHTTPIdempotencyAccepted(
			ctx,
			acceptance,
		)
	if err != nil {
		t.Fatalf(
			"MarkHTTPIdempotencyAccepted() error = %v",
			err,
		)
	}

	if accepted.State() !=
		repository.HTTPIdempotencyStateAccepted {
		t.Fatalf(
			"accepted state = %q",
			accepted.State(),
		)
	}

	actualAcceptedAt, exists :=
		accepted.AcceptedAt()

	if !exists {
		t.Fatal(
			"accepted timestamp does not exist",
		)
	}

	if !actualAcceptedAt.Equal(
		acceptedAt,
	) {
		t.Fatalf(
			"acceptedAt = %v, want %v",
			actualAcceptedAt,
			acceptedAt,
		)
	}

	reaccepted, err :=
		store.MarkHTTPIdempotencyAccepted(
			ctx,
			acceptance,
		)
	if err != nil {
		t.Fatalf(
			"idempotent MarkHTTPIdempotencyAccepted() error = %v",
			err,
		)
	}

	if reaccepted.State() !=
		repository.HTTPIdempotencyStateAccepted {
		t.Fatalf(
			"reaccepted state = %q",
			reaccepted.State(),
		)
	}
}

func TestHTTPIdempotencyStoreReturnsExistingForSameKey(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(
			t,
		)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"http_idempotency_keys",
	)

	suffix :=
		fmt.Sprintf(
			"%d",
			time.Now().
				UTC().
				UnixNano(),
		)

	companyID :=
		workflow.CompanyID(
			"company-http-existing-" +
				suffix,
		)

	registerHTTPIdempotencyCleanup(
		t,
		store,
		companyID,
	)

	createdAt :=
		time.Now().
			UTC().
			Truncate(
				time.Microsecond,
			)

	first :=
		mustHTTPIdempotencyReservation(
			t,
			companyID,
			"same-key",
			integrationHTTPIdempotencyFingerprint,
			execution.WorkflowExecutionID(
				"execution-first-"+
					suffix,
			),
			createdAt,
		)

	firstRecord, created, err :=
		store.ReserveHTTPIdempotency(
			ctx,
			first,
		)
	if err != nil {
		t.Fatalf(
			"first reservation error = %v",
			err,
		)
	}

	if !created {
		t.Fatal(
			"first reservation was not created",
		)
	}

	differentFingerprint :=
		"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	second :=
		mustHTTPIdempotencyReservation(
			t,
			companyID,
			"same-key",
			differentFingerprint,
			execution.WorkflowExecutionID(
				"execution-second-"+
					suffix,
			),
			createdAt,
		)

	existing, secondCreated, err :=
		store.ReserveHTTPIdempotency(
			ctx,
			second,
		)
	if err != nil {
		t.Fatalf(
			"second reservation error = %v",
			err,
		)
	}

	if secondCreated {
		t.Fatal(
			"same company and key created another reservation",
		)
	}

	if existing.RequestFingerprint() !=
		firstRecord.RequestFingerprint() {
		t.Fatalf(
			"existing fingerprint = %q, want %q",
			existing.RequestFingerprint(),
			firstRecord.RequestFingerprint(),
		)
	}

	if existing.WorkflowExecutionID() !=
		firstRecord.WorkflowExecutionID() {
		t.Fatalf(
			"existing execution = %q, want %q",
			existing.WorkflowExecutionID(),
			firstRecord.WorkflowExecutionID(),
		)
	}
}

func TestHTTPIdempotencyStoreRejectsExecutionReuse(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(
			t,
		)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"http_idempotency_keys",
	)

	suffix :=
		fmt.Sprintf(
			"%d",
			time.Now().
				UTC().
				UnixNano(),
		)

	companyID :=
		workflow.CompanyID(
			"company-http-execution-reuse-" +
				suffix,
		)

	registerHTTPIdempotencyCleanup(
		t,
		store,
		companyID,
	)

	createdAt :=
		time.Now().
			UTC().
			Truncate(
				time.Microsecond,
			)

	executionID :=
		execution.WorkflowExecutionID(
			"execution-shared-" +
				suffix,
		)

	first :=
		mustHTTPIdempotencyReservation(
			t,
			companyID,
			"key-1",
			integrationHTTPIdempotencyFingerprint,
			executionID,
			createdAt,
		)

	_, _, err :=
		store.ReserveHTTPIdempotency(
			ctx,
			first,
		)
	if err != nil {
		t.Fatalf(
			"first reservation error = %v",
			err,
		)
	}

	second :=
		mustHTTPIdempotencyReservation(
			t,
			companyID,
			"key-2",
			integrationHTTPIdempotencyFingerprint,
			executionID,
			createdAt,
		)

	_, _, err =
		store.ReserveHTTPIdempotency(
			ctx,
			second,
		)

	if !repository.IsConflict(
		err,
	) {
		t.Fatalf(
			"execution reuse error = %v, want CONFLICT",
			err,
		)
	}
}

func TestHTTPIdempotencyStoreRejectsMismatchedAcceptance(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(
			t,
		)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"http_idempotency_keys",
	)

	suffix :=
		fmt.Sprintf(
			"%d",
			time.Now().
				UTC().
				UnixNano(),
		)

	companyID :=
		workflow.CompanyID(
			"company-http-accept-conflict-" +
				suffix,
		)

	registerHTTPIdempotencyCleanup(
		t,
		store,
		companyID,
	)

	createdAt :=
		time.Now().
			UTC().
			Truncate(
				time.Microsecond,
			)

	reservation :=
		mustHTTPIdempotencyReservation(
			t,
			companyID,
			"request-conflict",
			integrationHTTPIdempotencyFingerprint,
			execution.WorkflowExecutionID(
				"execution-conflict-"+
					suffix,
			),
			createdAt,
		)

	record, _, err :=
		store.ReserveHTTPIdempotency(
			ctx,
			reservation,
		)
	if err != nil {
		t.Fatalf(
			"reservation error = %v",
			err,
		)
	}

	acceptance :=
		mustHTTPIdempotencyAcceptanceWithFingerprint(
			t,
			record,
			"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			createdAt.Add(
				time.Second,
			),
		)

	_, err =
		store.MarkHTTPIdempotencyAccepted(
			ctx,
			acceptance,
		)

	if !repository.IsConflict(
		err,
	) {
		t.Fatalf(
			"mismatched acceptance error = %v, want CONFLICT",
			err,
		)
	}
}

func TestHTTPIdempotencyStoreConcurrentReservation(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(
			t,
		)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"http_idempotency_keys",
	)

	suffix :=
		fmt.Sprintf(
			"%d",
			time.Now().
				UTC().
				UnixNano(),
		)

	companyID :=
		workflow.CompanyID(
			"company-http-concurrent-" +
				suffix,
		)

	registerHTTPIdempotencyCleanup(
		t,
		store,
		companyID,
	)

	const workers = 12

	var createdCount atomic.Int32

	results :=
		make(
			[]repository.HTTPIdempotencyRecord,
			workers,
		)

	errorsByWorker :=
		make(
			[]error,
			workers,
		)

	start :=
		make(
			chan struct{},
		)

	var waitGroup sync.WaitGroup

	for index := 0; index < workers; index++ {
		waitGroup.Add(
			1,
		)

		go func(
			workerIndex int,
		) {
			defer waitGroup.Done()

			<-start

			reservation :=
				mustHTTPIdempotencyReservation(
					t,
					companyID,
					"concurrent-key",
					integrationHTTPIdempotencyFingerprint,
					execution.WorkflowExecutionID(
						fmt.Sprintf(
							"execution-concurrent-%d-%s",
							workerIndex,
							suffix,
						),
					),
					time.Now().
						UTC().
						Truncate(
							time.Microsecond,
						),
				)

			record, created, err :=
				store.ReserveHTTPIdempotency(
					ctx,
					reservation,
				)

			results[workerIndex] =
				record

			errorsByWorker[workerIndex] =
				err

			if created {
				createdCount.Add(
					1,
				)
			}
		}(
			index,
		)
	}

	close(
		start,
	)

	waitGroup.Wait()

	for index, err := range errorsByWorker {
		if err != nil {
			t.Fatalf(
				"worker %d error = %v",
				index,
				err,
			)
		}
	}

	if createdCount.Load() != 1 {
		t.Fatalf(
			"created count = %d, want 1",
			createdCount.Load(),
		)
	}

	expectedExecutionID :=
		results[0].
			WorkflowExecutionID()

	for index, record := range results {
		if record.WorkflowExecutionID() !=
			expectedExecutionID {
			t.Fatalf(
				"worker %d execution ID = %q, want %q",
				index,
				record.WorkflowExecutionID(),
				expectedExecutionID,
			)
		}
	}
}

func mustHTTPIdempotencyReservation(
	t *testing.T,
	companyID workflow.CompanyID,
	idempotencyKey string,
	fingerprint string,
	workflowExecutionID execution.WorkflowExecutionID,
	createdAt time.Time,
) repository.HTTPIdempotencyReservation {
	t.Helper()

	reservation, err :=
		repository.NewHTTPIdempotencyReservation(
			repository.HTTPIdempotencyReservationParams{
				CompanyID:           companyID,
				IdempotencyKey:      idempotencyKey,
				RequestFingerprint:  fingerprint,
				WorkflowExecutionID: workflowExecutionID,
				CreatedAt:           createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewHTTPIdempotencyReservation() error = %v",
			err,
		)
	}

	return reservation
}

func mustHTTPIdempotencyAcceptance(
	t *testing.T,
	record repository.HTTPIdempotencyRecord,
	acceptedAt time.Time,
) repository.HTTPIdempotencyAcceptance {
	t.Helper()

	return mustHTTPIdempotencyAcceptanceWithFingerprint(
		t,
		record,
		record.RequestFingerprint(),
		acceptedAt,
	)
}

func mustHTTPIdempotencyAcceptanceWithFingerprint(
	t *testing.T,
	record repository.HTTPIdempotencyRecord,
	fingerprint string,
	acceptedAt time.Time,
) repository.HTTPIdempotencyAcceptance {
	t.Helper()

	acceptance, err :=
		repository.NewHTTPIdempotencyAcceptance(
			repository.HTTPIdempotencyAcceptanceParams{
				CompanyID:           record.CompanyID(),
				IdempotencyKey:      record.IdempotencyKey(),
				RequestFingerprint:  fingerprint,
				WorkflowExecutionID: record.WorkflowExecutionID(),
				AcceptedAt:          acceptedAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewHTTPIdempotencyAcceptance() error = %v",
			err,
		)
	}

	return acceptance
}

func registerHTTPIdempotencyCleanup(
	t *testing.T,
	store *Store,
	companyIDs ...workflow.CompanyID,
) {
	t.Helper()

	t.Cleanup(
		func() {
			cleanupContext, cancelCleanup :=
				context.WithTimeout(
					context.Background(),
					20*time.Second,
				)
			defer cancelCleanup()

			for _, companyID := range companyIDs {
				_, err :=
					store.pool.Exec(
						cleanupContext,
						`
DELETE FROM workflow_runtime.http_idempotency_keys
WHERE company_id = $1
`,
						companyID.String(),
					)

				if err != nil {
					t.Errorf(
						"HTTP idempotency cleanup for %q failed: %v",
						companyID,
						err,
					)
				}
			}
		},
	)
}

func TestHTTPIdempotencyAcceptanceEvidenceIntegration(
	t *testing.T,
) {
	ctx, store :=
		requireAsyncPersistenceStore(
			t,
		)

	suffix :=
		uniqueAsyncPrefix(
			t,
		)

	dispatchedScope :=
		createAsyncPersistenceScope(
			t,
			ctx,
			store,
			"http-evidence-dispatched-"+suffix,
		)

	rejectedScope :=
		createAsyncPersistenceScope(
			t,
			ctx,
			store,
			"http-evidence-rejected-"+suffix,
		)

	evidence, err :=
		store.
			HasHTTPIdempotencyAcceptanceEvidence(
				ctx,
				dispatchedScope.companyID,
				dispatchedScope.executionID,
			)

	if err != nil {
		t.Fatalf(
			"initial evidence check error = %v",
			err,
		)
	}

	if evidence {
		t.Fatal(
			"execution without durable evidence was accepted",
		)
	}

	workflowEvent :=
		newIntegrationOutboxMessage(
			t,
			dispatchedScope,
			"evidence-workflow-event",
			repository.OutboxOperationWorkflowEvent,
			time.Now().
				UTC(),
		)

	if err :=
		store.
			CreateOutboxMessage(
				ctx,
				workflowEvent,
			); err != nil {

		t.Fatalf(
			"create workflow event outbox error = %v",
			err,
		)
	}

	evidence, err =
		store.
			HasHTTPIdempotencyAcceptanceEvidence(
				ctx,
				dispatchedScope.companyID,
				dispatchedScope.executionID,
			)

	if err != nil {
		t.Fatalf(
			"workflow event evidence check error = %v",
			err,
		)
	}

	if evidence {
		t.Fatal(
			"workflow event incorrectly qualified as async acceptance evidence",
		)
	}

	command :=
		newIntegrationOutboxMessage(
			t,
			dispatchedScope,
			"evidence-node-command",
			repository.OutboxOperationNodeCommand,
			time.Now().
				UTC(),
		)

	if err :=
		store.
			CreateOutboxMessage(
				ctx,
				command,
			); err != nil {

		t.Fatalf(
			"create node command outbox error = %v",
			err,
		)
	}

	evidence, err =
		store.
			HasHTTPIdempotencyAcceptanceEvidence(
				ctx,
				dispatchedScope.companyID,
				dispatchedScope.executionID,
			)

	if err != nil {
		t.Fatalf(
			"node command evidence check error = %v",
			err,
		)
	}

	if !evidence {
		t.Fatal(
			"durable node command was not recognized as acceptance evidence",
		)
	}

	otherCompanyID :=
		workflow.CompanyID(
			"company-http-evidence-other-" +
				suffix,
		)

	evidence, err =
		store.
			HasHTTPIdempotencyAcceptanceEvidence(
				ctx,
				otherCompanyID,
				dispatchedScope.executionID,
			)

	if err != nil {
		t.Fatalf(
			"tenant isolation evidence check error = %v",
			err,
		)
	}

	if evidence {
		t.Fatal(
			"acceptance evidence crossed company boundary",
		)
	}

	rejectedAt :=
		time.Now().
			UTC().
			Add(
				time.Second,
			).
			Truncate(
				time.Microsecond,
			)

	result, err :=
		store.
			pool.
			Exec(
				ctx,
				`
UPDATE workflow_runtime.workflow_executions
SET
	status = 'REJECTED',
	validating_at = COALESCE(validating_at, $1),
	finished_at = $1,
	updated_at = $1
WHERE company_id = $2
  AND workflow_execution_id = $3
`,
				rejectedAt,
				rejectedScope.companyID.String(),
				rejectedScope.executionID.String(),
			)

	if err != nil {
		t.Fatalf(
			"persist rejected execution error = %v",
			err,
		)
	}

	if result.RowsAffected() != 1 {
		t.Fatalf(
			"rejected execution rows affected = %d, want 1",
			result.RowsAffected(),
		)
	}

	evidence, err =
		store.
			HasHTTPIdempotencyAcceptanceEvidence(
				ctx,
				rejectedScope.companyID,
				rejectedScope.executionID,
			)

	if err != nil {
		t.Fatalf(
			"rejected execution evidence check error = %v",
			err,
		)
	}

	if !evidence {
		t.Fatal(
			"durable rejected execution was not recognized as acceptance evidence",
		)
	}
}

func TestInboxStoreIntegrationIdentityAndHydration(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	received := time.Now().UTC()
	position, _ := repository.NewInboxSourcePosition("topic-a", 1, 42)
	applied := newIntegrationInboxMessage(t, scope, "consumer-a", "shared-message", repository.InboxProcessingApplied, &position, received)
	if err := store.RecordInboxMessage(ctx, applied); err != nil {
		t.Fatal(err)
	}
	processed, err := store.HasProcessedInboxMessage(ctx, applied.ConsumerIdentity(), applied.MessageID())
	if err != nil || !processed {
		t.Fatalf("processed=%t,%v", processed, err)
	}
	if err := store.RecordInboxMessage(ctx, applied); !repository.IsConflict(err) {
		t.Fatalf("duplicate=%v", err)
	}
	ignored := newIntegrationInboxMessage(t, scope, "consumer-b", "shared-message", repository.InboxProcessingIgnored, nil, received)
	if err := store.RecordInboxMessage(ctx, ignored); err != nil {
		t.Fatal(err)
	}
	hydrated, err := getInboxMessage(ctx, store.pool, ignored.ConsumerIdentity(), ignored.MessageID())
	if err != nil || hydrated.ProcessingResult() != repository.InboxProcessingIgnored {
		t.Fatalf("hydrate=%v,%v", hydrated.ProcessingResult(), err)
	}
	changedPosition, _ := repository.NewInboxSourcePosition("topic-b", 9, 999)
	sameIdentity := newIntegrationInboxMessage(t, scope, "consumer-a", "shared-message", repository.InboxProcessingApplied, &changedPosition, received)
	if err := store.RecordInboxMessage(ctx, sameIdentity); !repository.IsConflict(err) {
		t.Fatalf("diagnostic metadata changed identity: %v", err)
	}
}

func TestInboxStoreIntegrationRollbackAndConcurrentDuplicate(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	received := time.Now().UTC()
	message := newIntegrationInboxMessage(t, scope, "rollback-consumer", "rollback-message", repository.InboxProcessingApplied, nil, received)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := recordInboxMessage(ctx, tx, message); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	processed, err := store.HasProcessedInboxMessage(ctx, message.ConsumerIdentity(), message.MessageID())
	if err != nil || processed {
		t.Fatalf("rollback left inbox row=%t,%v", processed, err)
	}
	if err := store.RecordInboxMessage(ctx, message); err != nil {
		t.Fatalf("reinsert after rollback=%v", err)
	}
	concurrent := newIntegrationInboxMessage(t, scope, "concurrent-consumer", "concurrent-message", repository.InboxProcessingIgnored, nil, received)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() { defer wait.Done(); <-start; results <- store.RecordInboxMessage(ctx, concurrent) }()
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
		t.Fatalf("concurrent results success=%d conflict=%d", successes, conflicts)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_runtime.inbox_messages WHERE consumer_name=$1 AND message_id=$2`, concurrent.ConsumerIdentity().String(), concurrent.MessageID().String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable duplicate count=%d,%v", count, err)
	}
}

func newIntegrationInboxMessage(t *testing.T, scope asyncPersistenceScope, consumer, messageID string, result repository.InboxProcessingResult, position *repository.InboxSourcePosition, received time.Time) repository.InboxMessage {
	t.Helper()
	message, err := repository.NewInboxMessage(repository.InboxMessageParams{ConsumerIdentity: repository.ConsumerIdentity(consumer), MessageID: repository.MessageID(messageID + "-" + scope.prefix), CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID, NodeExecutionID: scope.commandNodeID, MessageType: "miletos.node-result", MessageVersion: 1, ProcessingResult: result, SourcePosition: position, ReceivedAt: received, ProcessedAt: received.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func TestInboxStoreContractAndSQLMapping(t *testing.T) {
	var implementation repository.InboxStore = &Store{}
	if implementation == nil {
		t.Fatal("inbox contract unavailable")
	}
	if strings.Contains(strings.ToUpper(insertInboxSQL), "SELECT *") || strings.Contains(strings.ToUpper(selectInboxSQL), "SELECT *") {
		t.Fatal("inbox SQL uses SELECT *")
	}
	for _, fragment := range []string{"consumer_name", "message_id", "processing_result", "source_partition", "source_offset"} {
		if !strings.Contains(insertInboxSQL, fragment) {
			t.Fatalf("insert SQL missing %q", fragment)
		}
	}
}

func TestInboxConstraintErrorsAreMeaningfulAndPreserveCause(t *testing.T) {
	duplicate := &pgconn.PgError{Code: postgreSQLUniqueViolationCode, ConstraintName: "inbox_messages_pk"}
	err := mapInboxWriteError("record", duplicate)
	if !repository.IsConflict(err) || !errors.Is(err, duplicate) || !strings.Contains(err.Error(), "consumer/message identity") {
		t.Fatalf("duplicate mapping=%v", err)
	}
	fk := &pgconn.PgError{Code: postgreSQLForeignKeyViolationCode, ConstraintName: "inbox_messages_execution_fk"}
	err = mapInboxWriteError("record", fk)
	if !repository.IsConflict(err) || !errors.Is(err, fk) {
		t.Fatalf("FK mapping=%v", err)
	}
}

const integrationTestTimeout = 30 * time.Second

func requirePostgreSQLIntegrationStore(
	t *testing.T,
) (
	context.Context,
	*Store,
) {
	t.Helper()

	if strings.TrimSpace(
		os.Getenv(
			"MILETOS_RUNTIME_POSTGRES_PASSWORD",
		),
	) == "" {
		t.Skip(
			"PostgreSQL integration test skipped: " +
				"MILETOS_RUNTIME_POSTGRES_PASSWORD is not set",
		)
	}

	configuration, err :=
		config.LoadPostgreSQL()
	if err != nil {
		t.Fatalf(
			"config.LoadPostgreSQL() returned an error: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		integrationTestTimeout,
	)

	t.Cleanup(cancel)

	pool, err := OpenPool(
		ctx,
		configuration,
	)
	if err != nil {
		t.Fatalf(
			"OpenPool() returned an error: %v",
			err,
		)
	}

	t.Cleanup(pool.Close)

	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf(
			"NewStore() returned an error: %v",
			err,
		)
	}

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"workflow_definition_snapshots",
	)

	return ctx, store
}

func requirePostgreSQLIntegrationTable(
	t *testing.T,
	ctx context.Context,
	store *Store,
	tableName string,
) {
	t.Helper()

	var exists bool

	err := store.pool.QueryRow(
		ctx,
		`
SELECT to_regclass($1) IS NOT NULL
`,
		tableName,
	).Scan(
		&exists,
	)
	if err != nil {
		t.Fatalf(
			"inspect PostgreSQL table %q: %v",
			tableName,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"required PostgreSQL table %q does not exist; apply migrations first",
			tableName,
		)
	}
}

func TestNewMigrationConnectionConfigRemovesRuntimeSearchPath(
	t *testing.T,
) {
	configuration := newTestPostgreSQLConfiguration()

	actual, err := newMigrationConnectionConfig(
		configuration,
	)
	if err != nil {
		t.Fatalf(
			"newMigrationConnectionConfig() returned an unexpected error: %v",
			err,
		)
	}

	if _, exists := actual.RuntimeParams["search_path"]; exists {
		t.Fatal(
			"migration connection configuration retained the runtime search_path",
		)
	}

	if actual.RuntimeParams["application_name"] !=
		migrationApplicationName {
		t.Fatalf(
			"application_name = %q, want %q",
			actual.RuntimeParams["application_name"],
			migrationApplicationName,
		)
	}
}

func TestNewMigrationConnectionConfigPreservesDatabaseIdentity(
	t *testing.T,
) {
	configuration := newTestPostgreSQLConfiguration()

	actual, err := newMigrationConnectionConfig(
		configuration,
	)
	if err != nil {
		t.Fatalf(
			"newMigrationConnectionConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.Host != configuration.Host {
		t.Fatalf(
			"Host = %q, want %q",
			actual.Host,
			configuration.Host,
		)
	}

	if actual.Port != uint16(configuration.Port) {
		t.Fatalf(
			"Port = %d, want %d",
			actual.Port,
			configuration.Port,
		)
	}

	if actual.Database != configuration.Database {
		t.Fatalf(
			"Database = %q, want %q",
			actual.Database,
			configuration.Database,
		)
	}

	if actual.User != configuration.User {
		t.Fatalf(
			"User = %q, want %q",
			actual.User,
			configuration.User,
		)
	}

	if actual.Password != configuration.Password {
		t.Fatal(
			"migration connection password was changed",
		)
	}
}

func TestOpenMigrationDatabaseRejectsNilParentContext(
	t *testing.T,
) {
	database, err := OpenMigrationDatabase(
		nil,
		newTestPostgreSQLConfiguration(),
	)

	if err == nil {
		t.Fatal(
			"OpenMigrationDatabase() returned nil error for a nil parent context",
		)
	}

	if database != nil {
		t.Fatal(
			"OpenMigrationDatabase() returned a database for a nil parent context",
		)
	}

	if !strings.Contains(
		err.Error(),
		"must not be nil",
	) {
		t.Fatalf(
			"error = %q, want it to contain %q",
			err.Error(),
			"must not be nil",
		)
	}
}

func TestParseMigrationCommandAcceptsSupportedCommands(
	t *testing.T,
) {
	tests := map[string]MigrationCommand{
		"up":        MigrationCommandUp,
		" UP ":      MigrationCommandUp,
		"status":    MigrationCommandStatus,
		" STATUS ":  MigrationCommandStatus,
		"version":   MigrationCommandVersion,
		" VERSION ": MigrationCommandVersion,
	}

	for value, expected := range tests {
		t.Run(value, func(t *testing.T) {
			actual, err := ParseMigrationCommand(value)
			if err != nil {
				t.Fatalf(
					"ParseMigrationCommand() returned an unexpected error: %v",
					err,
				)
			}

			if actual != expected {
				t.Fatalf(
					"command = %q, want %q",
					actual,
					expected,
				)
			}
		})
	}
}

func TestParseMigrationCommandRejectsUnsupportedCommands(
	t *testing.T,
) {
	tests := []string{
		"",
		" ",
		"down",
		"reset",
		"drop",
		"invalid",
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			_, err := ParseMigrationCommand(value)
			if err == nil {
				t.Fatal(
					"ParseMigrationCommand() returned nil error for an unsupported command",
				)
			}

			if !strings.Contains(
				err.Error(),
				"supported commands",
			) {
				t.Fatalf(
					"error = %q, want it to describe supported commands",
					err.Error(),
				)
			}
		})
	}
}

func TestRunMigrationCommandRejectsNilContext(
	t *testing.T,
) {
	result, err := RunMigrationCommand(
		nil,
		&sql.DB{},
		t.TempDir(),
		MigrationCommandUp,
	)

	if err == nil {
		t.Fatal(
			"RunMigrationCommand() returned nil error for a nil context",
		)
	}

	if result.HasVersion {
		t.Fatal(
			"RunMigrationCommand() returned a version for a rejected command",
		)
	}
}

func TestRunMigrationCommandRejectsNilDatabase(
	t *testing.T,
) {
	result, err := RunMigrationCommand(
		context.Background(),
		nil,
		t.TempDir(),
		MigrationCommandUp,
	)

	if err == nil {
		t.Fatal(
			"RunMigrationCommand() returned nil error for a nil database",
		)
	}

	if result.HasVersion {
		t.Fatal(
			"RunMigrationCommand() returned a version for a rejected command",
		)
	}
}

func TestRunMigrationCommandRejectsUnsupportedCommand(
	t *testing.T,
) {
	result, err := RunMigrationCommand(
		context.Background(),
		&sql.DB{},
		t.TempDir(),
		MigrationCommand("unsupported"),
	)

	if err == nil {
		t.Fatal(
			"RunMigrationCommand() returned nil error for an unsupported command",
		)
	}

	if result.HasVersion {
		t.Fatal(
			"RunMigrationCommand() returned a version for a rejected command",
		)
	}
}

func TestValidateMigrationsPathAcceptsDirectory(
	t *testing.T,
) {
	directory := t.TempDir()

	actual, err := validateMigrationsPath(
		"  " + directory + "  ",
	)
	if err != nil {
		t.Fatalf(
			"validateMigrationsPath() returned an unexpected error: %v",
			err,
		)
	}

	if actual != directory {
		t.Fatalf(
			"path = %q, want %q",
			actual,
			directory,
		)
	}
}

func TestValidateMigrationsPathRejectsBlankPath(
	t *testing.T,
) {
	_, err := validateMigrationsPath("   ")
	if err == nil {
		t.Fatal(
			"validateMigrationsPath() returned nil error for a blank path",
		)
	}
}

func TestValidateMigrationsPathRejectsMissingPath(
	t *testing.T,
) {
	_, err := validateMigrationsPath(
		t.TempDir() + "/missing",
	)
	if err == nil {
		t.Fatal(
			"validateMigrationsPath() returned nil error for a missing path",
		)
	}
}

func TestValidateMigrationsPathRejectsFile(
	t *testing.T,
) {
	file := t.TempDir() + "/migration.sql"

	if err := writeTestFile(file); err != nil {
		t.Fatalf(
			"writeTestFile() returned an unexpected error: %v",
			err,
		)
	}

	_, err := validateMigrationsPath(file)
	if err == nil {
		t.Fatal(
			"validateMigrationsPath() returned nil error for a file",
		)
	}
}

func TestNodeExecutionReaderIntegration(
	t *testing.T,
) {
	ctx, store := requirePostgreSQLIntegrationStore(t)

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

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	companyA := workflow.CompanyID(
		"company-node-reader-a-" + suffix,
	)

	companyB := workflow.CompanyID(
		"company-node-reader-b-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-node-reader-" + suffix,
	)

	workflowExecutionID := execution.WorkflowExecutionID(
		"execution-node-reader-" + suffix,
	)

	snapshot := newWorkflowExecutionReadSnapshot(
		t,
		companyA,
		workflowID,
		repository.DefinitionSnapshotID(
			"snapshot-node-reader-"+suffix,
		),
	)

	workflowCreatedAt := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	workflowExecution := newWorkflowExecutionReadRecord(
		t,
		workflowExecutionID,
		companyA,
		workflowID,
		snapshot.ID(),
		execution.WorkflowExecutionStatusRunning,
		workflowCreatedAt,
	)

	t.Cleanup(func() {
		cleanupContext, cancelCleanup := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancelCleanup()

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM node_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"node execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`,
			companyA.String(),
			workflowExecutionID.String(),
		); err != nil {
			t.Errorf(
				"workflow execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_definition_snapshots
WHERE company_id = $1
  AND snapshot_id = $2
`,
			companyA.String(),
			snapshot.ID().String(),
		); err != nil {
			t.Errorf(
				"snapshot cleanup failed: %v",
				err,
			)
		}
	})

	if err := store.Create(ctx, snapshot); err != nil {
		t.Fatalf(
			"create snapshot fixture: %v",
			err,
		)
	}

	insertWorkflowExecutionReadFixture(
		t,
		ctx,
		store,
		workflowExecution,
	)

	baseNodeTime := workflowCreatedAt.Add(time.Minute)

	records := []repository.NodeExecutionRecord{
		newNodeExecutionReadRecord(
			t,
			execution.NodeExecutionID("node-execution-1-"+suffix),
			workflowExecutionID,
			companyA,
			workflow.NodeID("node-1"),
			execution.NodeExecutionStatusPending,
			baseNodeTime.Add(1*time.Minute),
		),
		newNodeExecutionReadRecord(
			t,
			execution.NodeExecutionID("node-execution-2-"+suffix),
			workflowExecutionID,
			companyA,
			workflow.NodeID("node-2"),
			execution.NodeExecutionStatusRunning,
			baseNodeTime.Add(2*time.Minute),
		),
		newNodeExecutionReadRecord(
			t,
			execution.NodeExecutionID("node-execution-3-"+suffix),
			workflowExecutionID,
			companyA,
			workflow.NodeID("node-3"),
			execution.NodeExecutionStatusSucceeded,
			baseNodeTime.Add(3*time.Minute),
		),
		newNodeExecutionReadRecord(
			t,
			execution.NodeExecutionID("node-execution-4-"+suffix),
			workflowExecutionID,
			companyA,
			workflow.NodeID("node-4"),
			execution.NodeExecutionStatusFailed,
			baseNodeTime.Add(4*time.Minute),
		),
	}

	for _, record := range records {
		insertNodeExecutionReadFixture(
			t,
			ctx,
			store,
			record,
		)
	}

	firstRequest, err := repository.NewPageRequest(2, "")
	if err != nil {
		t.Fatalf(
			"create first page request: %v",
			err,
		)
	}

	firstPage, err := store.ListNodeExecutions(
		ctx,
		companyA,
		workflowExecutionID,
		firstRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListNodeExecutions() first page error: %v",
			err,
		)
	}

	assertNodeExecutionPageIDs(
		t,
		firstPage.Items(),
		[]execution.NodeExecutionID{
			records[0].ID(),
			records[1].ID(),
		},
	)

	assertNodeExecutionRecordsEqual(
		t,
		firstPage.Items()[1],
		records[1],
	)

	nextToken, exists := firstPage.Next()
	if !exists {
		t.Fatal(
			"first node execution page has no next token",
		)
	}

	secondRequest, err := repository.NewPageRequest(2, nextToken)
	if err != nil {
		t.Fatalf(
			"create second page request: %v",
			err,
		)
	}

	secondPage, err := store.ListNodeExecutions(
		ctx,
		companyA,
		workflowExecutionID,
		secondRequest,
	)
	if err != nil {
		t.Fatalf(
			"ListNodeExecutions() second page error: %v",
			err,
		)
	}

	assertNodeExecutionPageIDs(
		t,
		secondPage.Items(),
		[]execution.NodeExecutionID{
			records[2].ID(),
			records[3].ID(),
		},
	)

	assertNodeExecutionRecordsEqual(
		t,
		secondPage.Items()[0],
		records[2],
	)

	assertNodeExecutionRecordsEqual(
		t,
		secondPage.Items()[1],
		records[3],
	)

	if secondPage.HasNext() {
		t.Fatal(
			"second node execution page unexpectedly has a next token",
		)
	}

	_, crossTenantErr := store.ListNodeExecutions(
		ctx,
		companyB,
		workflowExecutionID,
		firstRequest,
	)
	if !repository.IsNotFound(crossTenantErr) {
		t.Fatalf(
			"cross-tenant ListNodeExecutions() error = %v, want NOT_FOUND",
			crossTenantErr,
		)
	}

	_, missingErr := store.ListNodeExecutions(
		ctx,
		companyA,
		execution.WorkflowExecutionID("missing-execution-"+suffix),
		firstRequest,
	)
	if !repository.IsNotFound(missingErr) {
		t.Fatalf(
			"missing ListNodeExecutions() error = %v, want NOT_FOUND",
			missingErr,
		)
	}
}

func newNodeExecutionReadRecord(
	t *testing.T,
	id execution.NodeExecutionID,
	workflowExecutionID execution.WorkflowExecutionID,
	companyID workflow.CompanyID,
	nodeID workflow.NodeID,
	status execution.NodeExecutionStatus,
	createdAt time.Time,
) repository.NodeExecutionRecord {
	t.Helper()

	params := repository.NodeExecutionRecordParams{
		ID:                  id,
		WorkflowExecutionID: workflowExecutionID,
		CompanyID:           companyID,
		NodeID:              nodeID,
		PluginType:          workflow.PluginType("core.pass-through"),
		PluginVersion:       workflow.PluginVersion("v1"),
		Status:              status,
		Attempt:             1,
		CreatedAt:           createdAt,
		UpdatedAt:           createdAt,
		LockVersion:         2,
	}

	switch status {
	case execution.NodeExecutionStatusPending:
		params.LockVersion = 0

	case execution.NodeExecutionStatusRunning:
		params.ReadyAt = createdAt.Add(1 * time.Second)
		params.StartedAt = createdAt.Add(2 * time.Second)
		params.UpdatedAt = params.StartedAt
		params.InputSummary = []byte(
			`{"inputBytes":12}`,
		)

	case execution.NodeExecutionStatusSucceeded:
		params.ReadyAt = createdAt.Add(1 * time.Second)
		params.StartedAt = createdAt.Add(2 * time.Second)
		params.FinishedAt = createdAt.Add(3 * time.Second)
		params.UpdatedAt = params.FinishedAt
		params.InputSummary = []byte(
			`{"inputBytes":12}`,
		)
		params.OutputSummary = []byte(
			`{"outputBytes":24}`,
		)

	case execution.NodeExecutionStatusFailed:
		params.ReadyAt = createdAt.Add(1 * time.Second)
		params.StartedAt = createdAt.Add(2 * time.Second)
		params.FinishedAt = createdAt.Add(3 * time.Second)
		params.UpdatedAt = params.FinishedAt
		params.InputSummary = []byte(
			`{"inputBytes":12}`,
		)
		params.FailureSummary = []byte(
			`{"code":"NODE_FAILED"}`,
		)

	default:
		t.Fatalf(
			"unsupported node execution test status: %q",
			status,
		)
	}

	record, err := repository.NewNodeExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an error: %v",
			err,
		)
	}

	return record
}

func insertNodeExecutionReadFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	record repository.NodeExecutionRecord,
) {
	t.Helper()

	_, err := store.pool.Exec(
		ctx,
		`
INSERT INTO node_executions (
	node_execution_id,
	workflow_execution_id,
	company_id,
	node_id,
	plugin_type,
	plugin_version,
	status,
	attempt,
	created_at,
	ready_at,
	queued_at,
	started_at,
	finished_at,
	updated_at,
	input_summary,
	output_summary,
	failure_summary,
	lock_version
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10,
	$11,
	$12,
	$13,
	$14,
	$15::jsonb,
	$16::jsonb,
	$17::jsonb,
	$18
)
`,
		record.ID().String(),
		record.WorkflowExecutionID().String(),
		record.CompanyID().String(),
		record.NodeID().String(),
		record.PluginType().String(),
		record.PluginVersion().String(),
		record.Status().String(),
		record.Attempt(),
		record.CreatedAt(),
		nodeExecutionOptionalTimeValue(record.ReadyAt),
		nodeExecutionOptionalTimeValue(record.QueuedAt),
		nodeExecutionOptionalTimeValue(record.StartedAt),
		nodeExecutionOptionalTimeValue(record.FinishedAt),
		record.UpdatedAt(),
		nodeExecutionOptionalJSONValue(record.InputSummary),
		nodeExecutionOptionalJSONValue(record.OutputSummary),
		nodeExecutionOptionalJSONValue(record.FailureSummary),
		record.LockVersion(),
	)
	if err != nil {
		t.Fatalf(
			"insert node execution fixture: %v",
			err,
		)
	}
}

func nodeExecutionOptionalTimeValue(
	getter func() (time.Time, bool),
) any {
	value, exists := getter()
	if !exists {
		return nil
	}

	return value
}

func nodeExecutionOptionalJSONValue(
	getter func() (repository.JSONObject, bool),
) any {
	value, exists := getter()
	if !exists {
		return nil
	}

	return value.String()
}

func assertNodeExecutionPageIDs(
	t *testing.T,
	records []repository.NodeExecutionRecord,
	expected []execution.NodeExecutionID,
) {
	t.Helper()

	if len(records) != len(expected) {
		t.Fatalf(
			"record count = %d, want %d",
			len(records),
			len(expected),
		)
	}

	for index, expectedID := range expected {
		if records[index].ID() != expectedID {
			t.Fatalf(
				"record[%d] ID = %q, want %q",
				index,
				records[index].ID(),
				expectedID,
			)
		}
	}
}

func assertNodeExecutionRecordsEqual(
	t *testing.T,
	actual repository.NodeExecutionRecord,
	expected repository.NodeExecutionRecord,
) {
	t.Helper()

	if actual.ID() != expected.ID() {
		t.Fatalf(
			"node execution ID = %q, want %q",
			actual.ID(),
			expected.ID(),
		)
	}

	if actual.WorkflowExecutionID() != expected.WorkflowExecutionID() {
		t.Fatalf(
			"workflow execution ID = %q, want %q",
			actual.WorkflowExecutionID(),
			expected.WorkflowExecutionID(),
		)
	}

	if actual.CompanyID() != expected.CompanyID() {
		t.Fatalf(
			"company ID = %q, want %q",
			actual.CompanyID(),
			expected.CompanyID(),
		)
	}

	if actual.NodeID() != expected.NodeID() {
		t.Fatalf(
			"node ID = %q, want %q",
			actual.NodeID(),
			expected.NodeID(),
		)
	}

	if actual.PluginType() != expected.PluginType() {
		t.Fatalf(
			"plugin type = %q, want %q",
			actual.PluginType(),
			expected.PluginType(),
		)
	}

	if actual.PluginVersion() != expected.PluginVersion() {
		t.Fatalf(
			"plugin version = %q, want %q",
			actual.PluginVersion(),
			expected.PluginVersion(),
		)
	}

	if actual.Status() != expected.Status() {
		t.Fatalf(
			"status = %q, want %q",
			actual.Status(),
			expected.Status(),
		)
	}

	if actual.Attempt() != expected.Attempt() {
		t.Fatalf(
			"attempt = %d, want %d",
			actual.Attempt(),
			expected.Attempt(),
		)
	}

	if !actual.CreatedAt().Equal(expected.CreatedAt()) {
		t.Fatalf(
			"createdAt = %v, want %v",
			actual.CreatedAt(),
			expected.CreatedAt(),
		)
	}

	assertNodeExecutionOptionalTime(
		t,
		"readyAt",
		actual.ReadyAt,
		expected.ReadyAt,
	)

	assertNodeExecutionOptionalTime(
		t,
		"queuedAt",
		actual.QueuedAt,
		expected.QueuedAt,
	)

	assertNodeExecutionOptionalTime(
		t,
		"startedAt",
		actual.StartedAt,
		expected.StartedAt,
	)

	assertNodeExecutionOptionalTime(
		t,
		"finishedAt",
		actual.FinishedAt,
		expected.FinishedAt,
	)

	if !actual.UpdatedAt().Equal(expected.UpdatedAt()) {
		t.Fatalf(
			"updatedAt = %v, want %v",
			actual.UpdatedAt(),
			expected.UpdatedAt(),
		)
	}

	assertNodeExecutionOptionalJSON(
		t,
		"inputSummary",
		actual.InputSummary,
		expected.InputSummary,
	)

	assertNodeExecutionOptionalJSON(
		t,
		"outputSummary",
		actual.OutputSummary,
		expected.OutputSummary,
	)

	assertNodeExecutionOptionalJSON(
		t,
		"failureSummary",
		actual.FailureSummary,
		expected.FailureSummary,
	)

	if actual.LockVersion() != expected.LockVersion() {
		t.Fatalf(
			"lock version = %d, want %d",
			actual.LockVersion(),
			expected.LockVersion(),
		)
	}
}

func assertNodeExecutionOptionalTime(
	t *testing.T,
	field string,
	actualGetter func() (time.Time, bool),
	expectedGetter func() (time.Time, bool),
) {
	t.Helper()

	actual, actualExists := actualGetter()
	expected, expectedExists := expectedGetter()

	if actualExists != expectedExists {
		t.Fatalf(
			"%s exists = %t, want %t",
			field,
			actualExists,
			expectedExists,
		)
	}

	if actualExists && !actual.Equal(expected) {
		t.Fatalf(
			"%s = %v, want %v",
			field,
			actual,
			expected,
		)
	}
}

func assertNodeExecutionOptionalJSON(
	t *testing.T,
	field string,
	actualGetter func() (repository.JSONObject, bool),
	expectedGetter func() (repository.JSONObject, bool),
) {
	t.Helper()

	actual, actualExists := actualGetter()
	expected, expectedExists := expectedGetter()

	if actualExists != expectedExists {
		t.Fatalf(
			"%s exists = %t, want %t",
			field,
			actualExists,
			expectedExists,
		)
	}

	if !actualExists {
		return
	}

	var actualValue any
	if err := json.Unmarshal(actual.Bytes(), &actualValue); err != nil {
		t.Fatalf(
			"actual %s JSON is invalid: %v",
			field,
			err,
		)
	}

	var expectedValue any
	if err := json.Unmarshal(expected.Bytes(), &expectedValue); err != nil {
		t.Fatalf(
			"expected %s JSON is invalid: %v",
			field,
			err,
		)
	}

	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf(
			"%s JSON differs\nactual: %s\nexpected: %s",
			field,
			actual.Bytes(),
			expected.Bytes(),
		)
	}
}

func TestNodeExecutionReaderContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.NodeExecutionReader = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement NodeExecutionReader",
		)
	}
}

func TestNodeExecutionReaderRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	pageRequest, err := repository.NewPageRequest(10, "")
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListNodeExecutions(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListNodeExecutions() returned nil error for nil store",
		)
	}
}

func TestNodeExecutionReaderRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedNodeExecutionReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(10, "")
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListNodeExecutions(
		nil,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListNodeExecutions() accepted nil context",
		)
	}
}

func TestNodeExecutionReaderPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedNodeExecutionReaderTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pageRequest, err := repository.NewPageRequest(10, "")
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListNodeExecutions(
		ctx,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"ListNodeExecutions() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestNodeExecutionReaderRejectsInvalidIdentifiers(
	t *testing.T,
) {
	store := newUnconnectedNodeExecutionReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(10, "")
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	tests := []struct {
		name                string
		companyID           workflow.CompanyID
		workflowExecutionID execution.WorkflowExecutionID
	}{
		{
			name:                "blank company ID",
			companyID:           workflow.CompanyID(" "),
			workflowExecutionID: execution.WorkflowExecutionID("execution-1"),
		},
		{
			name:                "blank workflow execution ID",
			companyID:           workflow.CompanyID("company-1"),
			workflowExecutionID: execution.WorkflowExecutionID(" "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.ListNodeExecutions(
				context.Background(),
				test.companyID,
				test.workflowExecutionID,
				pageRequest,
			)
			if err == nil {
				t.Fatal(
					"ListNodeExecutions() accepted invalid identifiers",
				)
			}
		})
	}
}

func TestNodeExecutionReaderRejectsMalformedCursor(
	t *testing.T,
) {
	store := newUnconnectedNodeExecutionReaderTestStore(t)

	pageRequest, err := repository.NewPageRequest(
		10,
		repository.PageToken("***"),
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err = store.ListNodeExecutions(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListNodeExecutions() accepted malformed cursor",
		)
	}

	var validationError *repository.ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"ListNodeExecutions() error type = %T, want *repository.ValidationError",
			err,
		)
	}

	if validationError.Field !=
		"pageToken" {

		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			"pageToken",
		)
	}
}

func TestBuildNodeExecutionListQueryUsesTenantScopeAndCursor(
	t *testing.T,
) {
	cursorTime := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	cursor, err := encodeTimestampCursor(
		cursorKindNodeExecutions,
		cursorTime,
		"node-execution-9",
	)
	if err != nil {
		t.Fatalf(
			"encodeTimestampCursor() returned an unexpected error: %v",
			err,
		)
	}

	pageRequest, err := repository.NewPageRequest(25, cursor)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	query, arguments, err := buildNodeExecutionListQuery(
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID("execution-1"),
		pageRequest,
	)
	if err != nil {
		t.Fatalf(
			"buildNodeExecutionListQuery() returned an unexpected error: %v",
			err,
		)
	}

	requiredFragments := []string{
		"WHERE company_id = $1",
		"workflow_execution_id = $2",
		"(created_at, node_execution_id) > ($3, $4)",
		"ORDER BY created_at ASC, node_execution_id ASC",
		"LIMIT $5",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(query, fragment) {
			t.Fatalf(
				"query does not contain %q:\n%s",
				fragment,
				query,
			)
		}
	}

	if len(arguments) != 5 {
		t.Fatalf(
			"argument count = %d, want 5",
			len(arguments),
		)
	}

	if arguments[0] != "company-1" {
		t.Fatalf(
			"company argument = %#v",
			arguments[0],
		)
	}

	if arguments[1] != "execution-1" {
		t.Fatalf(
			"workflow execution argument = %#v",
			arguments[1],
		)
	}

	if arguments[3] != "node-execution-9" {
		t.Fatalf(
			"node execution cursor argument = %#v",
			arguments[3],
		)
	}

	if arguments[4] != 26 {
		t.Fatalf(
			"limit argument = %#v, want 26",
			arguments[4],
		)
	}
}

func newUnconnectedNodeExecutionReaderTestStore(
	t *testing.T,
) *Store {
	t.Helper()

	store, err := NewStore(new(pgxpool.Pool))
	if err != nil {
		t.Fatalf(
			"NewStore() returned an unexpected error: %v",
			err,
		)
	}

	return store
}

const (
	nodeTransitionExpectedWorkflowLockVersion int64 = 8
	nodeTransitionExpectedNodeLockVersion     int64 = 3
)

var nodeTransitionExpectedSequence = repository.SequenceNumber(30)

func TestNodeTransitionStoreIntegration(
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

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"execution_logs",
	)

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"execution_errors",
	)

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	successFixture :=
		newNodeTransitionIntegrationFixture(
			t,
			"success-"+suffix,
			"",
		)

	sharedErrorID :=
		repository.ExecutionErrorID(
			"shared-node-transition-error-" +
				suffix,
		)

	rollbackHolder :=
		newNodeTransitionIntegrationFixture(
			t,
			"rollback-holder-"+suffix,
			sharedErrorID.String(),
		)

	rollbackTarget :=
		newNodeTransitionIntegrationFixture(
			t,
			"rollback-target-"+suffix,
			sharedErrorID.String(),
		)

	fixtures := []nodeTransitionIntegrationFixture{
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
		"persists node event log and structured error atomically",
		func(t *testing.T) {
			prepareNodeTransitionIntegrationFixture(
				t,
				ctx,
				store,
				successFixture,
			)

			err := store.ApplyNodeTransition(
				ctx,
				successFixture.command,
			)
			if err != nil {
				t.Fatalf(
					"ApplyNodeTransition() returned an error: %v",
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
				execution.WorkflowExecutionStatusRunning {
				t.Fatalf(
					"workflow status = %q, want RUNNING",
					actualWorkflow.Status(),
				)
			}

			if actualWorkflow.LockVersion() !=
				nodeTransitionExpectedWorkflowLockVersion+1 {
				t.Fatalf(
					"workflow lock version = %d, want %d",
					actualWorkflow.LockVersion(),
					nodeTransitionExpectedWorkflowLockVersion+1,
				)
			}

			expectedNextSequence :=
				repository.SequenceNumber(
					nodeTransitionExpectedSequence.
						Int64() + 2,
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

			if len(nodePage.Items()) != 1 {
				t.Fatalf(
					"node count = %d, want 1",
					len(nodePage.Items()),
				)
			}

			actualNode := nodePage.Items()[0]

			if actualNode.ID() !=
				successFixture.nodeExecutionID {
				t.Fatalf(
					"node execution ID = %q, want %q",
					actualNode.ID(),
					successFixture.nodeExecutionID,
				)
			}

			if actualNode.Status() !=
				execution.NodeExecutionStatusFailed {
				t.Fatalf(
					"node status = %q, want FAILED",
					actualNode.Status(),
				)
			}

			if actualNode.LockVersion() !=
				nodeTransitionExpectedNodeLockVersion+1 {
				t.Fatalf(
					"node lock version = %d, want %d",
					actualNode.LockVersion(),
					nodeTransitionExpectedNodeLockVersion+1,
				)
			}

			finishedAt, exists :=
				actualNode.FinishedAt()
			if !exists {
				t.Fatal(
					"node finished time was not persisted",
				)
			}

			if !finishedAt.Equal(
				successFixture.transitionAt,
			) {
				t.Fatalf(
					"node finished time = %v, want %v",
					finishedAt,
					successFixture.transitionAt,
				)
			}

			failureSummary, exists :=
				actualNode.FailureSummary()
			if !exists {
				t.Fatal(
					"node failure summary was not persisted",
				)
			}

			assertEquivalentJSON(
				t,
				failureSummary.Bytes(),
				[]byte(
					`{"code":"NODE_FAILED"}`,
				),
			)

			if _, exists :=
				actualNode.OutputSummary(); exists {
				t.Fatal(
					"failed node unexpectedly contains output summary",
				)
			}

			inputSummary, exists :=
				actualNode.InputSummary()
			if !exists {
				t.Fatal(
					"node input summary was not preserved",
				)
			}

			assertEquivalentJSON(
				t,
				inputSummary.Bytes(),
				[]byte(
					`{"input":"prepared"}`,
				),
			)

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

			if len(eventPage.Items()) != 1 {
				t.Fatalf(
					"event count = %d, want 1",
					len(eventPage.Items()),
				)
			}

			eventRecord := eventPage.Items()[0]

			if eventRecord.ID() !=
				successFixture.eventID {
				t.Fatalf(
					"event ID = %q, want %q",
					eventRecord.ID(),
					successFixture.eventID,
				)
			}

			if eventRecord.SequenceNumber() !=
				nodeTransitionExpectedSequence {
				t.Fatalf(
					"event sequence = %d, want %d",
					eventRecord.SequenceNumber(),
					nodeTransitionExpectedSequence,
				)
			}

			if eventRecord.Type() !=
				repository.ExecutionEventTypeNodeFailed {
				t.Fatalf(
					"event type = %q, want NODE_FAILED",
					eventRecord.Type(),
				)
			}

			eventNodeID, exists :=
				eventRecord.NodeExecutionID()
			if !exists ||
				eventNodeID !=
					successFixture.nodeExecutionID {
				t.Fatalf(
					"event node execution ID = %q, %t; want %q",
					eventNodeID,
					exists,
					successFixture.nodeExecutionID,
				)
			}

			logPage, err :=
				store.ListExecutionLogs(
					ctx,
					successFixture.companyID,
					successFixture.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionLogs() returned an error: %v",
					err,
				)
			}

			if len(logPage.Items()) != 1 {
				t.Fatalf(
					"log count = %d, want 1",
					len(logPage.Items()),
				)
			}

			if logPage.Items()[0].
				SequenceNumber() !=
				repository.SequenceNumber(
					nodeTransitionExpectedSequence.
						Int64()+1,
				) {
				t.Fatalf(
					"log sequence = %d, want %d",
					logPage.Items()[0].
						SequenceNumber(),
					nodeTransitionExpectedSequence.
						Int64()+1,
				)
			}

			logNodeID, exists :=
				logPage.Items()[0].
					NodeExecutionID()
			if !exists ||
				logNodeID !=
					successFixture.nodeExecutionID {
				t.Fatalf(
					"log node execution ID = %q, %t; want %q",
					logNodeID,
					exists,
					successFixture.nodeExecutionID,
				)
			}

			errorPage, err :=
				store.ListExecutionErrors(
					ctx,
					successFixture.companyID,
					successFixture.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionErrors() returned an error: %v",
					err,
				)
			}

			if len(errorPage.Items()) != 1 {
				t.Fatalf(
					"error count = %d, want 1",
					len(errorPage.Items()),
				)
			}

			errorRecord := errorPage.Items()[0]

			if errorRecord.ID() !=
				successFixture.errorID {
				t.Fatalf(
					"error ID = %q, want %q",
					errorRecord.ID(),
					successFixture.errorID,
				)
			}

			if errorRecord.Category() !=
				runtimefailure.FailureCategoryExecution {
				t.Fatalf(
					"error category = %q, want EXECUTION",
					errorRecord.Category(),
				)
			}

			if errorRecord.SafeMessage() !=
				"Node execution failed" {
				t.Fatalf(
					"safe message = %q",
					errorRecord.SafeMessage(),
				)
			}

			errorNodeID, exists :=
				errorRecord.NodeExecutionID()
			if !exists ||
				errorNodeID !=
					successFixture.nodeExecutionID {
				t.Fatalf(
					"error node execution ID = %q, %t; want %q",
					errorNodeID,
					exists,
					successFixture.nodeExecutionID,
				)
			}

			relatedEventID, exists :=
				errorRecord.RelatedEventID()
			if !exists ||
				relatedEventID !=
					successFixture.eventID {
				t.Fatalf(
					"related event ID = %q, %t; want %q",
					relatedEventID,
					exists,
					successFixture.eventID,
				)
			}

			technicalDetail, exists :=
				errorRecord.TechnicalDetail()
			if !exists {
				t.Fatal(
					"technical detail was not persisted",
				)
			}

			if technicalDetail !=
				"controlled node transition integration failure" {
				t.Fatalf(
					"technical detail = %q",
					technicalDetail,
				)
			}

			staleErr :=
				store.ApplyNodeTransition(
					ctx,
					successFixture.command,
				)
			if !repository.IsStaleWrite(
				staleErr,
			) {
				t.Fatalf(
					"repeated ApplyNodeTransition() error = %v, want STALE_WRITE",
					staleErr,
				)
			}
		},
	)

	t.Run(
		"rolls back workflow node timeline and error when error insert fails",
		func(t *testing.T) {
			prepareNodeTransitionIntegrationFixture(
				t,
				ctx,
				store,
				rollbackHolder,
			)

			prepareNodeTransitionIntegrationFixture(
				t,
				ctx,
				store,
				rollbackTarget,
			)

			if err := store.ApplyNodeTransition(
				ctx,
				rollbackHolder.command,
			); err != nil {
				t.Fatalf(
					"apply rollback holder node transition: %v",
					err,
				)
			}

			transitionErr :=
				store.ApplyNodeTransition(
					ctx,
					rollbackTarget.command,
				)
			if !repository.IsConflict(
				transitionErr,
			) {
				t.Fatalf(
					"ApplyNodeTransition() rollback error = %v, want CONFLICT",
					transitionErr,
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

			if actualWorkflow.Status() !=
				execution.WorkflowExecutionStatusRunning {
				t.Fatalf(
					"workflow status after rollback = %q, want RUNNING",
					actualWorkflow.Status(),
				)
			}

			if actualWorkflow.LockVersion() !=
				nodeTransitionExpectedWorkflowLockVersion {
				t.Fatalf(
					"workflow lock version after rollback = %d, want %d",
					actualWorkflow.LockVersion(),
					nodeTransitionExpectedWorkflowLockVersion,
				)
			}

			if actualWorkflow.NextSequenceNumber() !=
				nodeTransitionExpectedSequence {
				t.Fatalf(
					"workflow next sequence after rollback = %d, want %d",
					actualWorkflow.NextSequenceNumber(),
					nodeTransitionExpectedSequence,
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

			if len(nodePage.Items()) != 1 {
				t.Fatalf(
					"node count after rollback = %d, want 1",
					len(nodePage.Items()),
				)
			}

			actualNode := nodePage.Items()[0]

			if actualNode.Status() !=
				execution.NodeExecutionStatusRunning {
				t.Fatalf(
					"node status after rollback = %q, want RUNNING",
					actualNode.Status(),
				)
			}

			if actualNode.LockVersion() !=
				nodeTransitionExpectedNodeLockVersion {
				t.Fatalf(
					"node lock version after rollback = %d, want %d",
					actualNode.LockVersion(),
					nodeTransitionExpectedNodeLockVersion,
				)
			}

			if _, exists :=
				actualNode.FinishedAt(); exists {
				t.Fatal(
					"node finished time remained after rollback",
				)
			}

			if _, exists :=
				actualNode.FailureSummary(); exists {
				t.Fatal(
					"node failure summary remained after rollback",
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

			logPage, err :=
				store.ListExecutionLogs(
					ctx,
					rollbackTarget.companyID,
					rollbackTarget.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionLogs() after rollback: %v",
					err,
				)
			}

			if len(logPage.Items()) != 0 {
				t.Fatalf(
					"log count after rollback = %d, want 0",
					len(logPage.Items()),
				)
			}

			errorPage, err :=
				store.ListExecutionErrors(
					ctx,
					rollbackTarget.companyID,
					rollbackTarget.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionErrors() after rollback: %v",
					err,
				)
			}

			if len(errorPage.Items()) != 0 {
				t.Fatalf(
					"error count after rollback = %d, want 0",
					len(errorPage.Items()),
				)
			}

			assertNodeTransitionIntegrationRowCount(
				t,
				ctx,
				store,
				"execution_errors",
				"error_id",
				sharedErrorID.String(),
				1,
			)
		},
	)
}

type nodeTransitionIntegrationFixture struct {
	command repository.NodeTransitionCommand

	snapshot repository.DefinitionSnapshot

	currentWorkflow repository.WorkflowExecutionRecord
	currentNode     repository.NodeExecutionRecord

	companyID workflow.CompanyID

	workflowExecutionID execution.WorkflowExecutionID
	nodeExecutionID     execution.NodeExecutionID

	eventID repository.ExecutionEventID
	logID   repository.ExecutionLogID
	errorID repository.ExecutionErrorID

	transitionAt time.Time
}

func newNodeTransitionIntegrationFixture(
	t *testing.T,
	suffix string,
	errorIDOverride string,
) nodeTransitionIntegrationFixture {
	t.Helper()

	companyID := workflow.CompanyID(
		"company-node-transition-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-node-transition-" + suffix,
	)

	snapshotID :=
		repository.DefinitionSnapshotID(
			"snapshot-node-transition-" + suffix,
		)

	workflowExecutionID :=
		execution.WorkflowExecutionID(
			"execution-node-transition-" + suffix,
		)

	nodeExecutionID :=
		execution.NodeExecutionID(
			"node-execution-transition-" + suffix,
		)

	createdAt := time.Date(
		2026,
		time.July,
		17,
		22,
		0,
		0,
		0,
		time.UTC,
	).Add(
		time.Duration(len(suffix)) *
			time.Millisecond,
	)

	validatingAt :=
		createdAt.Add(time.Second)

	workflowStartedAt :=
		createdAt.Add(2 * time.Second)

	nodeCreatedAt :=
		createdAt.Add(3 * time.Second)

	nodeReadyAt :=
		createdAt.Add(4 * time.Second)

	nodeStartedAt :=
		createdAt.Add(5 * time.Second)

	transitionAt :=
		createdAt.Add(10 * time.Second)

	snapshot, err :=
		repository.NewDefinitionSnapshot(
			snapshotID,
			companyID,
			workflowID,
			9,
			"Node Transition Integration",
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

	currentWorkflow, err :=
		repository.NewWorkflowExecutionRecord(
			repository.WorkflowExecutionRecordParams{
				ID:                 workflowExecutionID,
				CompanyID:          companyID,
				WorkflowID:         workflowID,
				WorkflowRevision:   9,
				SnapshotID:         snapshotID,
				Mode:               execution.ExecutionModeSync,
				CorrelationID:      "correlation-" + suffix,
				Status:             execution.WorkflowExecutionStatusRunning,
				CreatedAt:          createdAt,
				ValidatingAt:       validatingAt,
				StartedAt:          workflowStartedAt,
				UpdatedAt:          workflowStartedAt,
				TerminalOutputs:    []byte(`{}`),
				NextSequenceNumber: nodeTransitionExpectedSequence,
				LockVersion:        nodeTransitionExpectedWorkflowLockVersion,
			},
		)
	if err != nil {
		t.Fatalf(
			"create current workflow record: %v",
			err,
		)
	}

	currentNode, err :=
		repository.NewNodeExecutionRecord(
			repository.NodeExecutionRecordParams{
				ID:                  nodeExecutionID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				NodeID:              workflow.NodeID("node-1"),
				PluginType: workflow.PluginType(
					"core.pass-through",
				),
				PluginVersion: workflow.PluginVersion("v1"),
				Status:        execution.NodeExecutionStatusRunning,
				Attempt:       1,
				CreatedAt:     nodeCreatedAt,
				ReadyAt:       nodeReadyAt,
				StartedAt:     nodeStartedAt,
				UpdatedAt:     nodeStartedAt,
				InputSummary: []byte(
					`{"input":"prepared"}`,
				),
				LockVersion: nodeTransitionExpectedNodeLockVersion,
			},
		)
	if err != nil {
		t.Fatalf(
			"create current node record: %v",
			err,
		)
	}

	updatedNode, err :=
		repository.NewNodeExecutionRecord(
			repository.NodeExecutionRecordParams{
				ID:                  nodeExecutionID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				NodeID:              workflow.NodeID("node-1"),
				PluginType: workflow.PluginType(
					"core.pass-through",
				),
				PluginVersion: workflow.PluginVersion("v1"),
				Status:        execution.NodeExecutionStatusFailed,
				Attempt:       1,
				CreatedAt:     nodeCreatedAt,
				ReadyAt:       nodeReadyAt,
				StartedAt:     nodeStartedAt,
				FinishedAt:    transitionAt,
				UpdatedAt:     transitionAt,
				InputSummary: []byte(
					`{"input":"prepared"}`,
				),
				FailureSummary: []byte(
					`{"code":"NODE_FAILED"}`,
				),
				LockVersion: nodeTransitionExpectedNodeLockVersion + 1,
			},
		)
	if err != nil {
		t.Fatalf(
			"create updated node record: %v",
			err,
		)
	}

	eventID := repository.ExecutionEventID(
		"event-node-failed-" + suffix,
	)

	eventDraft, err :=
		repository.NewExecutionEventDraft(
			repository.ExecutionEventDraftParams{
				ID:                  eventID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				NodeExecutionID:     nodeExecutionID,
				Type:                repository.ExecutionEventTypeNodeFailed,
				PreviousStatus: execution.NodeExecutionStatusRunning.
					String(),
				NewStatus: execution.NodeExecutionStatusFailed.
					String(),
				CorrelationID: "correlation-" + suffix,
				CausationID:   "node-run-" + suffix,
				SafeMessage:   "Node execution failed",
				Metadata: []byte(
					`{"nodeId":"node-1"}`,
				),
				CreatedAt: transitionAt,
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

	logID := repository.ExecutionLogID(
		"log-node-failed-" + suffix,
	)

	logDraft, err :=
		repository.NewExecutionLogDraft(
			repository.ExecutionLogDraftParams{
				ID:                  logID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				NodeExecutionID:     nodeExecutionID,
				Level:               repository.ExecutionLogLevelError,
				Message:             "Node execution entered FAILED state",
				Metadata: []byte(
					`{"status":"FAILED"}`,
				),
				CreatedAt: transitionAt.Add(time.Second),
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an error: %v",
			err,
		)
	}

	logEntry, err :=
		repository.NewLogTimelineEntry(
			logDraft,
		)
	if err != nil {
		t.Fatalf(
			"NewLogTimelineEntry() returned an error: %v",
			err,
		)
	}

	errorID := repository.ExecutionErrorID(
		"error-node-failed-" + suffix,
	)

	if errorIDOverride != "" {
		errorID =
			repository.ExecutionErrorID(
				errorIDOverride,
			)
	}

	errorRecord, err :=
		repository.NewExecutionErrorRecord(
			repository.ExecutionErrorRecordParams{
				ID:                  errorID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				NodeExecutionID:     nodeExecutionID,
				RelatedEventID:      eventID,
				Category:            runtimefailure.FailureCategoryExecution,
				Code:                "NODE_EXECUTION_FAILED",
				SafeMessage:         "Node execution failed",
				TechnicalDetail:     "controlled node transition integration failure",
				Retryable:           false,
				Details: []byte(
					`{"component":"node-transition"}`,
				),
				CreatedAt: transitionAt.Add(
					2 * time.Second,
				),
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an error: %v",
			err,
		)
	}

	command, err :=
		repository.NewNodeTransitionCommand(
			repository.NodeTransitionCommandParams{
				CompanyID:                   companyID,
				WorkflowExecutionID:         workflowExecutionID,
				NodeExecutionID:             nodeExecutionID,
				ExpectedWorkflowStatus:      execution.WorkflowExecutionStatusRunning,
				ExpectedWorkflowLockVersion: nodeTransitionExpectedWorkflowLockVersion,
				ExpectedNextSequenceNumber:  nodeTransitionExpectedSequence,
				ExpectedNodeStatus:          execution.NodeExecutionStatusRunning,
				ExpectedNodeLockVersion:     nodeTransitionExpectedNodeLockVersion,
				NodeExecution:               updatedNode,
				Timeline: []repository.TimelineEntry{
					eventEntry,
					logEntry,
				},
				Errors: []repository.ExecutionErrorRecord{
					errorRecord,
				},
			},
		)
	if err != nil {
		t.Fatalf(
			"NewNodeTransitionCommand() returned an error: %v",
			err,
		)
	}

	return nodeTransitionIntegrationFixture{
		command:             command,
		snapshot:            snapshot,
		currentWorkflow:     currentWorkflow,
		currentNode:         currentNode,
		companyID:           companyID,
		workflowExecutionID: workflowExecutionID,
		nodeExecutionID:     nodeExecutionID,
		eventID:             eventID,
		logID:               logID,
		errorID:             errorID,
		transitionAt:        transitionAt,
	}
}

func prepareNodeTransitionIntegrationFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	fixture nodeTransitionIntegrationFixture,
) {
	t.Helper()

	if err := store.Create(
		ctx,
		fixture.snapshot,
	); err != nil {
		t.Fatalf(
			"create node transition snapshot: %v",
			err,
		)
	}

	insertWorkflowExecutionReadFixture(
		t,
		ctx,
		store,
		fixture.currentWorkflow,
	)

	insertNodeExecutionReadFixture(
		t,
		ctx,
		store,
		fixture.currentNode,
	)
}

func assertNodeTransitionIntegrationRowCount(
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

func TestNodeTransitionStoreContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.NodeTransitionStore = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement NodeTransitionStore",
		)
	}
}

func TestExecutionLifecycleStoreContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.ExecutionLifecycleStore = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement ExecutionLifecycleStore",
		)
	}
}

func TestApplyNodeTransitionRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	err := store.ApplyNodeTransition(
		context.Background(),
		repository.NodeTransitionCommand{},
	)
	if err == nil {
		t.Fatal(
			"ApplyNodeTransition() returned nil error for nil store",
		)
	}
}

func TestApplyNodeTransitionRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedNodeTransitionStore(t)

	err := store.ApplyNodeTransition(
		nil,
		repository.NodeTransitionCommand{},
	)
	if err == nil {
		t.Fatal(
			"ApplyNodeTransition() accepted nil context",
		)
	}
}

func TestApplyNodeTransitionPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedNodeTransitionStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err := store.ApplyNodeTransition(
		ctx,
		repository.NodeTransitionCommand{},
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"ApplyNodeTransition() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestApplyNodeTransitionRejectsInvalidCommand(
	t *testing.T,
) {
	store := newUnconnectedNodeTransitionStore(t)

	err := store.ApplyNodeTransition(
		context.Background(),
		repository.NodeTransitionCommand{},
	)
	if err == nil {
		t.Fatal(
			"ApplyNodeTransition() accepted an invalid command",
		)
	}
}

func newUnconnectedNodeTransitionStore(
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

func TestOutboxPublisherKafkaIntegration(t *testing.T) {
	broker := os.Getenv("MILETOS_KAFKA_TEST_BROKER")
	if broker == "" {
		t.Skip("MILETOS_KAFKA_TEST_BROKER is not configured")
	}
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	message := newIntegrationOutboxMessage(t, scope, "kafka-publish", repository.OutboxOperationWorkflowEvent, time.Now().UTC())
	if err := store.CreateOutboxMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	configuration := config.KafkaConfig{Enabled: true, Brokers: []string{broker}, EngineClientID: "miletos-test-engine", WorkerClientID: "miletos-test-worker", CommandTopic: "test-destination", EventTopic: "test-events", WorkerGroupID: "miletos-test-workers", EngineGroupID: "miletos-test-engine", MaxMessageBytes: 1024 * 1024}
	producer, err := transport.NewProducer(
		configuration.Brokers,
		configuration.EngineClientID,
		configuration.MaxMessageBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	publisher, err := transport.NewOutboxPublisher(store, producer, "kafka-integration-publisher", 10, time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	count, err := publisher.PublishBatch(ctx)
	if err != nil || count != 1 {
		t.Fatalf("published=%d err=%v", count, err)
	}
	consumer, err := transport.NewConsumer(
		configuration.Brokers,
		configuration.EngineClientID,
		"miletos-outbox-integration",
		message.Destination(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	consumerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	received := make(chan transport.Delivery, 1)
	go func() {
		_ = consumer.Run(consumerCtx, func(_ context.Context, delivery transport.Delivery) error { received <- delivery; return nil })
	}()
	select {
	case delivery := <-received:
		if string(delivery.Key) != message.MessageKey() || string(delivery.Value) != string(message.EncodedPayload()) {
			t.Fatalf("published delivery mismatch")
		}
	case <-consumerCtx.Done():
		t.Fatal(consumerCtx.Err())
	}
}

func TestOutboxStoreIntegrationLifecycle(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	base := time.Now().UTC().Add(-time.Minute)
	workflowMessage := newIntegrationOutboxMessage(t, scope, "workflow", repository.OutboxOperationWorkflowEvent, base)
	commandMessage := newIntegrationOutboxMessage(t, scope, "command", repository.OutboxOperationNodeCommand, base.Add(time.Second))
	resultMessage := newIntegrationOutboxMessage(t, scope, "result", repository.OutboxOperationNodeResult, base.Add(2*time.Second))
	for _, message := range []repository.OutboxMessage{workflowMessage, commandMessage, resultMessage} {
		if err := store.CreateOutboxMessage(ctx, message); err != nil {
			t.Fatalf("CreateOutboxMessage()=%v", err)
		}
	}
	if err := store.CreateOutboxMessage(ctx, workflowMessage); !repository.IsConflict(err) {
		t.Fatalf("duplicate ID=%v", err)
	}
	duplicateOperation := newIntegrationOutboxMessageWithID(t, scope, "workflow-duplicate", repository.OutboxOperationWorkflowEvent, base, "workflow")
	if err := store.CreateOutboxMessage(ctx, duplicateOperation); !repository.IsConflict(err) {
		t.Fatalf("duplicate operation=%v", err)
	}

	request, _ := repository.NewOutboxClaimRequest("publisher-a", base.Add(10*time.Minute), 2)
	claimed, err := store.ClaimPublishableOutboxMessages(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 || claimed[0].MessageID() != workflowMessage.MessageID() || claimed[1].MessageID() != commandMessage.MessageID() {
		t.Fatalf("claim order=%v", messageIDs(claimed))
	}
	if discriminator, ok := claimed[0].OperationDiscriminator(); !ok || discriminator != "workflow" {
		t.Fatalf("workflow discriminator=(%q,%t)", discriminator, ok)
	}
	if claimed[0].PublicationState() != repository.OutboxPublicationPublishing || claimed[0].LockVersion() != 1 {
		t.Fatal("claim state/version mismatch")
	}
	if owner, ok := claimed[0].ClaimOwner(); !ok || owner != "publisher-a" {
		t.Fatal("claim owner mismatch")
	}

	wrongOwner, _ := repository.NewMarkOutboxPublishedCommand(claimed[0].MessageID(), "publisher-b", claimed[0].LockVersion(), base.Add(11*time.Minute))
	if err := store.MarkOutboxMessagePublished(ctx, wrongOwner); !repository.IsStaleWrite(err) {
		t.Fatalf("wrong owner=%v", err)
	}
	wrongVersion, _ := repository.NewMarkOutboxPublishedCommand(claimed[0].MessageID(), "publisher-a", claimed[0].LockVersion()+1, base.Add(11*time.Minute))
	if err := store.MarkOutboxMessagePublished(ctx, wrongVersion); !repository.IsStaleWrite(err) {
		t.Fatalf("wrong version=%v", err)
	}
	mark, _ := repository.NewMarkOutboxPublishedCommand(claimed[0].MessageID(), "publisher-a", claimed[0].LockVersion(), base.Add(11*time.Minute))
	if err := store.MarkOutboxMessagePublished(ctx, mark); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkOutboxMessagePublished(ctx, mark); !repository.IsStaleWrite(err) {
		t.Fatalf("wrong state=%v", err)
	}
	missing, _ := repository.NewMarkOutboxPublishedCommand("missing-message", "publisher-a", 1, base.Add(11*time.Minute))
	if err := store.MarkOutboxMessagePublished(ctx, missing); !repository.IsNotFound(err) {
		t.Fatalf("missing=%v", err)
	}

	release, _ := repository.NewReleaseStaleOutboxClaimsRequest(base.Add(20*time.Minute), 1)
	count, err := store.ReleaseStaleOutboxClaims(ctx, release)
	if err != nil || count != 1 {
		t.Fatalf("release=(%d,%v)", count, err)
	}
	claimedAgain, err := store.ClaimPublishableOutboxMessages(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range claimedAgain {
		if message.MessageID() == workflowMessage.MessageID() {
			t.Fatal("published message was reclaimed")
		}
	}
}

func TestOutboxStoreIntegrationConcurrencyAndRollback(t *testing.T) {
	ctx, store := requireAsyncPersistenceStore(t)
	scope := createAsyncPersistenceScope(t, ctx, store, uniqueAsyncPrefix(t))
	base := time.Now().UTC().Add(-time.Minute)
	for index := 0; index < 12; index++ {
		message := newIntegrationOutboxMessageWithID(t, scope, fmt.Sprintf("concurrent-%02d", index), repository.OutboxOperationWorkflowEvent, base.Add(time.Duration(index)*time.Millisecond), fmt.Sprintf("event-%02d", index))
		if err := store.CreateOutboxMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	claimedAt := base.Add(time.Hour)
	owners := []string{"publisher-1", "publisher-2"}
	results := make(chan []repository.OutboxMessage, 2)
	errorsChannel := make(chan error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for _, owner := range owners {
		wait.Add(1)
		go func(owner string) {
			defer wait.Done()
			<-start
			request, _ := repository.NewOutboxClaimRequest(owner, claimedAt, 12)
			messages, err := store.ClaimPublishableOutboxMessages(ctx, request)
			results <- messages
			errorsChannel <- err
		}(owner)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[repository.MessageID]struct{}{}
	total := 0
	for messages := range results {
		for _, message := range messages {
			if _, exists := seen[message.MessageID()]; exists {
				t.Fatalf("duplicate concurrent claim %s", message.MessageID())
			}
			seen[message.MessageID()] = struct{}{}
			total++
		}
	}
	if total != 12 {
		t.Fatalf("claimed total=%d,want 12", total)
	}

	rollbackMessage := newIntegrationOutboxMessageWithID(t, scope, "rollback-insert", repository.OutboxOperationWorkflowEvent, base, "rollback-insert")
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createOutboxMessage(ctx, tx, rollbackMessage); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM workflow_runtime.outbox_messages WHERE message_id=$1`, rollbackMessage.MessageID().String()).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback insert count=%d err=%v", count, err)
	}

	claimRollback := newIntegrationOutboxMessageWithID(t, scope, "rollback-claim", repository.OutboxOperationWorkflowEvent, base, "rollback-claim")
	if err := store.CreateOutboxMessage(ctx, claimRollback); err != nil {
		t.Fatal(err)
	}
	tx, err = store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := repository.NewOutboxClaimRequest("rollback-owner", claimedAt, 1)
	inside, err := claimPublishableOutboxMessages(ctx, tx, request)
	if err != nil || len(inside) != 1 {
		t.Fatalf("transaction claim=%d,%v", len(inside), err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := store.ClaimPublishableOutboxMessages(ctx, request)
	if err != nil || len(after) != 1 || after[0].MessageID() != claimRollback.MessageID() {
		t.Fatalf("claim after rollback=%v,%v", messageIDs(after), err)
	}
}

func newIntegrationOutboxMessage(t *testing.T, scope asyncPersistenceScope, suffix string, kind repository.OutboxOperationKind, createdAt time.Time) repository.OutboxMessage {
	return newIntegrationOutboxMessageWithID(t, scope, suffix, kind, createdAt, suffix)
}
func newIntegrationOutboxMessageWithID(t *testing.T, scope asyncPersistenceScope, idSuffix string, kind repository.OutboxOperationKind, createdAt time.Time, discriminator string) repository.OutboxMessage {
	t.Helper()
	params := repository.OutboxMessageParams{MessageID: repository.MessageID("message-" + scope.prefix + "-" + idSuffix), CompanyID: scope.companyID, WorkflowExecutionID: scope.executionID, OperationKind: kind, MessageType: "miletos.test", MessageVersion: 1, Destination: "test-destination", MessageKey: "test-key", EncodedPayload: []byte(`{"version":1}`), PublicationState: repository.OutboxPublicationPending, CreatedAt: createdAt}
	switch kind {
	case repository.OutboxOperationWorkflowEvent:
		params.OperationDiscriminator = discriminator
	case repository.OutboxOperationNodeCommand:
		params.NodeExecutionID = scope.commandNodeID
		params.NodeID = workflow.NodeID("command")
		params.Attempt = 1
	case repository.OutboxOperationNodeResult:
		params.NodeExecutionID = scope.resultNodeID
		params.NodeID = workflow.NodeID("result")
		params.Attempt = 1
	}
	message, err := repository.NewOutboxMessage(params)
	if err != nil {
		t.Fatal(err)
	}
	return message
}
func messageIDs(messages []repository.OutboxMessage) []repository.MessageID {
	ids := make([]repository.MessageID, len(messages))
	for i, m := range messages {
		ids[i] = m.MessageID()
	}
	return ids
}

var _ context.Context

func TestOutboxStoreContractsAndSQLDesign(t *testing.T) {
	var writer repository.OutboxStore = &Store{}
	var publisher repository.OutboxPublicationStore = &Store{}
	if writer == nil || publisher == nil {
		t.Fatal("outbox contracts unavailable")
	}
	for name, sql := range map[string]string{"insert": insertOutboxSQL, "claim": claimOutboxSQL, "mark": markOutboxPublishedSQL, "release": releaseStaleOutboxSQL} {
		if strings.Contains(strings.ToUpper(sql), "SELECT *") {
			t.Fatalf("%s uses SELECT *", name)
		}
	}
	for _, fragment := range []string{"FOR UPDATE SKIP LOCKED", "ORDER BY created_at, message_id", "lock_version=outbox.lock_version+1"} {
		if !strings.Contains(claimOutboxSQL, fragment) {
			t.Fatalf("claim SQL missing %q", fragment)
		}
	}
	for _, fragment := range []string{"publication_state='PUBLISHING'", "claim_owner=$2", "lock_version=$3", "claimed_at=NULL", "claim_owner=NULL"} {
		if !strings.Contains(markOutboxPublishedSQL, fragment) {
			t.Fatalf("mark SQL missing %q", fragment)
		}
	}
	for _, fragment := range []string{"claimed_at < $1", "ORDER BY claimed_at, message_id", "publication_state='PENDING'"} {
		if !strings.Contains(releaseStaleOutboxSQL, fragment) {
			t.Fatalf("release SQL missing %q", fragment)
		}
	}
}

func TestOutboxConstraintErrorsAreMeaningfulAndPreserveCause(t *testing.T) {
	for _, test := range []struct{ constraint, resource string }{{"outbox_messages_pk", "outbox message ID"}, {"outbox_messages_operation_uk", "outbox operation key"}} {
		cause := &pgconn.PgError{Code: postgreSQLUniqueViolationCode, ConstraintName: test.constraint}
		err := mapOutboxWriteError("create", cause)
		if !repository.IsConflict(err) || !errors.Is(err, cause) || !strings.Contains(err.Error(), test.resource) {
			t.Fatalf("constraint %s mapping=%v", test.constraint, err)
		}
	}
	for _, test := range []struct{ code, constraint string }{{postgreSQLForeignKeyViolationCode, "outbox_messages_execution_fk"}, {postgreSQLCheckViolationCode, "outbox_messages_node_identity"}} {
		cause := &pgconn.PgError{Code: test.code, ConstraintName: test.constraint}
		if err := mapOutboxWriteError("create", cause); !repository.IsConflict(err) || !errors.Is(err, cause) {
			t.Fatalf("constraint mapping=%v", err)
		}
	}
}

func TestNewPoolConfigMapsPostgreSQLConfiguration(
	t *testing.T,
) {
	configuration := newTestPostgreSQLConfiguration()

	actual, err := NewPoolConfig(configuration)
	if err != nil {
		t.Fatalf(
			"NewPoolConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.ConnConfig.Host != configuration.Host {
		t.Fatalf(
			"host = %q, want %q",
			actual.ConnConfig.Host,
			configuration.Host,
		)
	}

	if actual.ConnConfig.Port != uint16(configuration.Port) {
		t.Fatalf(
			"port = %d, want %d",
			actual.ConnConfig.Port,
			configuration.Port,
		)
	}

	if actual.ConnConfig.Database != configuration.Database {
		t.Fatalf(
			"database = %q, want %q",
			actual.ConnConfig.Database,
			configuration.Database,
		)
	}

	if actual.ConnConfig.User != configuration.User {
		t.Fatalf(
			"user = %q, want %q",
			actual.ConnConfig.User,
			configuration.User,
		)
	}

	if actual.ConnConfig.Password != configuration.Password {
		t.Fatal(
			"password was not mapped into the private pgx connection configuration",
		)
	}

	if actual.ConnConfig.ConnectTimeout !=
		configuration.ConnectTimeout {
		t.Fatalf(
			"connect timeout = %s, want %s",
			actual.ConnConfig.ConnectTimeout,
			configuration.ConnectTimeout,
		)
	}

	if actual.MaxConns != configuration.MaxConnections {
		t.Fatalf(
			"maximum connections = %d, want %d",
			actual.MaxConns,
			configuration.MaxConnections,
		)
	}

	if actual.MinConns != configuration.MinConnections {
		t.Fatalf(
			"minimum connections = %d, want %d",
			actual.MinConns,
			configuration.MinConnections,
		)
	}

	if actual.ConnConfig.RuntimeParams["search_path"] !=
		configuration.Schema {
		t.Fatalf(
			"search_path = %q, want %q",
			actual.ConnConfig.RuntimeParams["search_path"],
			configuration.Schema,
		)
	}

	if actual.ConnConfig.RuntimeParams["application_name"] !=
		applicationName {
		t.Fatalf(
			"application_name = %q, want %q",
			actual.ConnConfig.RuntimeParams["application_name"],
			applicationName,
		)
	}
}

func TestNewPoolConfigPreservesEscapedCredentials(
	t *testing.T,
) {
	configuration := newTestPostgreSQLConfiguration()
	configuration.User = "runtime@user"
	configuration.Password = "p@ssword:/?# value"

	actual, err := NewPoolConfig(configuration)
	if err != nil {
		t.Fatalf(
			"NewPoolConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.ConnConfig.User != configuration.User {
		t.Fatalf(
			"user = %q, want %q",
			actual.ConnConfig.User,
			configuration.User,
		)
	}

	if actual.ConnConfig.Password != configuration.Password {
		t.Fatal(
			"password containing URL-sensitive characters was changed",
		)
	}
}

func TestNewPoolConfigSupportsIPv6Address(
	t *testing.T,
) {
	configuration := newTestPostgreSQLConfiguration()
	configuration.Host = "::1"

	actual, err := NewPoolConfig(configuration)
	if err != nil {
		t.Fatalf(
			"NewPoolConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.ConnConfig.Host != "::1" {
		t.Fatalf(
			"host = %q, want %q",
			actual.ConnConfig.Host,
			"::1",
		)
	}

	if actual.ConnConfig.Port !=
		uint16(configuration.Port) {
		t.Fatalf(
			"port = %d, want %d",
			actual.ConnConfig.Port,
			configuration.Port,
		)
	}
}

func TestNewPoolConfigDoesNotExposePasswordThroughRuntimeParams(
	t *testing.T,
) {
	configuration := newTestPostgreSQLConfiguration()

	actual, err := NewPoolConfig(configuration)
	if err != nil {
		t.Fatalf(
			"NewPoolConfig() returned an unexpected error: %v",
			err,
		)
	}

	for key, value := range actual.ConnConfig.RuntimeParams {
		if strings.Contains(
			key,
			configuration.Password,
		) {
			t.Fatalf(
				"runtime parameter key exposed the PostgreSQL password: %q",
				key,
			)
		}

		if strings.Contains(
			value,
			configuration.Password,
		) {
			t.Fatalf(
				"runtime parameter %q exposed the PostgreSQL password",
				key,
			)
		}
	}
}

func TestOpenPoolRejectsNilParentContext(
	t *testing.T,
) {
	pool, err := OpenPool(
		nil,
		newTestPostgreSQLConfiguration(),
	)

	if err == nil {
		t.Fatal(
			"OpenPool() returned nil error for a nil parent context",
		)
	}

	if pool != nil {
		t.Fatal(
			"OpenPool() returned a pool for a nil parent context",
		)
	}

	if !strings.Contains(
		err.Error(),
		"must not be nil",
	) {
		t.Fatalf(
			"error = %q, want it to contain %q",
			err.Error(),
			"must not be nil",
		)
	}
}

func newTestPostgreSQLConfiguration() config.PostgreSQLConfig {
	return config.PostgreSQLConfig{
		Host:           "127.0.0.1",
		Port:           55479,
		Database:       "miletos",
		Schema:         "workflow_runtime",
		User:           "miletos",
		Password:       "test-postgres-password",
		SSLMode:        "disable",
		MaxConnections: 12,
		MinConnections: 3,
		ConnectTimeout: 5 * time.Second,
		MigrationsPath: "./migrations",
	}
}

func TestWorkflowSnapshotRepositoryIntegration(
	t *testing.T,
) {
	ctx, store :=
		requirePostgreSQLIntegrationStore(t)

	suffix :=
		fmt.Sprintf(
			"%d",
			time.Now().UTC().UnixNano(),
		)

	companyA :=
		workflow.CompanyID(
			"company-snapshot-a-" + suffix,
		)

	companyB :=
		workflow.CompanyID(
			"company-snapshot-b-" + suffix,
		)

	snapshotID :=
		repository.DefinitionSnapshotID(
			"snapshot-integration-" + suffix,
		)

	snapshot, err :=
		repository.NewDefinitionSnapshot(
			snapshotID,
			companyA,
			workflow.WorkflowID(
				"workflow-integration-"+suffix,
			),
			7,
			"Integration Snapshot",
			[]byte(
				`{
					"nodes":[
						{
							"id":"node-1",
							"pluginType":"core.static-input",
							"pluginVersion":"v1"
						}
					],
					"edges":[]
				}`,
			),
			time.Date(
				2026,
				time.July,
				17,
				18,
				30,
				0,
				0,
				time.UTC,
			),
		)
	if err != nil {
		t.Fatalf(
			"NewDefinitionSnapshot() returned an error: %v",
			err,
		)
	}

	t.Cleanup(func() {
		cleanupContext, cancelCleanup :=
			context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
		defer cancelCleanup()

		_, cleanupErr := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_definition_snapshots
WHERE snapshot_id = $1
`,
			snapshotID.String(),
		)
		if cleanupErr != nil {
			t.Errorf(
				"snapshot integration cleanup failed: %v",
				cleanupErr,
			)
		}
	})

	if err := store.Create(
		ctx,
		snapshot,
	); err != nil {
		t.Fatalf(
			"Create() returned an error: %v",
			err,
		)
	}

	actual, err := store.GetByID(
		ctx,
		companyA,
		snapshotID,
	)
	if err != nil {
		t.Fatalf(
			"GetByID() returned an error: %v",
			err,
		)
	}

	assertDefinitionSnapshotsEqual(
		t,
		actual,
		snapshot,
	)

	duplicateErr := store.Create(
		ctx,
		snapshot,
	)
	if !repository.IsConflict(
		duplicateErr,
	) {
		t.Fatalf(
			"duplicate Create() error = %v, want CONFLICT",
			duplicateErr,
		)
	}

	_, crossTenantErr := store.GetByID(
		ctx,
		companyB,
		snapshotID,
	)
	if !repository.IsNotFound(
		crossTenantErr,
	) {
		t.Fatalf(
			"cross-tenant GetByID() error = %v, want NOT_FOUND",
			crossTenantErr,
		)
	}

	_, missingErr := store.GetByID(
		ctx,
		companyA,
		repository.DefinitionSnapshotID(
			"missing-snapshot-"+suffix,
		),
	)
	if !repository.IsNotFound(
		missingErr,
	) {
		t.Fatalf(
			"missing GetByID() error = %v, want NOT_FOUND",
			missingErr,
		)
	}

	var rowCount int

	err = store.pool.QueryRow(
		ctx,
		`
SELECT COUNT(*)
FROM workflow_definition_snapshots
WHERE company_id = $1
  AND snapshot_id = $2
`,
		companyA.String(),
		snapshotID.String(),
	).Scan(
		&rowCount,
	)
	if err != nil {
		t.Fatalf(
			"count persisted snapshot: %v",
			err,
		)
	}

	if rowCount != 1 {
		t.Fatalf(
			"persisted snapshot count = %d, want 1",
			rowCount,
		)
	}
}

func assertDefinitionSnapshotsEqual(
	t *testing.T,
	actual repository.DefinitionSnapshot,
	expected repository.DefinitionSnapshot,
) {
	t.Helper()

	if actual.ID() != expected.ID() {
		t.Fatalf(
			"snapshot ID = %q, want %q",
			actual.ID(),
			expected.ID(),
		)
	}

	if actual.CompanyID() !=
		expected.CompanyID() {
		t.Fatalf(
			"company ID = %q, want %q",
			actual.CompanyID(),
			expected.CompanyID(),
		)
	}

	if actual.WorkflowID() !=
		expected.WorkflowID() {
		t.Fatalf(
			"workflow ID = %q, want %q",
			actual.WorkflowID(),
			expected.WorkflowID(),
		)
	}

	if actual.WorkflowRevision() !=
		expected.WorkflowRevision() {
		t.Fatalf(
			"workflow revision = %d, want %d",
			actual.WorkflowRevision(),
			expected.WorkflowRevision(),
		)
	}

	if actual.WorkflowName() !=
		expected.WorkflowName() {
		t.Fatalf(
			"workflow name = %q, want %q",
			actual.WorkflowName(),
			expected.WorkflowName(),
		)
	}

	if !actual.CreatedAt().Equal(
		expected.CreatedAt(),
	) {
		t.Fatalf(
			"createdAt = %v, want %v",
			actual.CreatedAt(),
			expected.CreatedAt(),
		)
	}

	assertEquivalentJSON(
		t,
		actual.DefinitionJSON().Bytes(),
		expected.DefinitionJSON().Bytes(),
	)
}

func assertEquivalentJSON(
	t *testing.T,
	actual []byte,
	expected []byte,
) {
	t.Helper()

	var actualValue any

	if err := json.Unmarshal(
		actual,
		&actualValue,
	); err != nil {
		t.Fatalf(
			"actual JSON is invalid: %v",
			err,
		)
	}

	var expectedValue any

	if err := json.Unmarshal(
		expected,
		&expectedValue,
	); err != nil {
		t.Fatalf(
			"expected JSON is invalid: %v",
			err,
		)
	}

	if !reflect.DeepEqual(
		actualValue,
		expectedValue,
	) {
		t.Fatalf(
			"JSON values differ\nactual: %s\nexpected: %s",
			actual,
			expected,
		)
	}
}

func TestSnapshotRepositoryContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.WorkflowSnapshotRepository = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement WorkflowSnapshotRepository",
		)
	}
}

func TestSnapshotRepositoryRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	err := store.Create(
		context.Background(),
		repository.DefinitionSnapshot{},
	)
	if err == nil {
		t.Fatal(
			"Create() returned nil error for a nil store",
		)
	}

	_, err = store.GetByID(
		context.Background(),
		workflow.CompanyID("company-1"),
		repository.DefinitionSnapshotID(
			"snapshot-1",
		),
	)
	if err == nil {
		t.Fatal(
			"GetByID() returned nil error for a nil store",
		)
	}
}

func TestSnapshotRepositoryRejectsNilContexts(
	t *testing.T,
) {
	store := newUnconnectedSnapshotTestStore(t)

	err := store.Create(
		nil,
		newPostgreSQLSnapshotTestValue(t),
	)
	if err == nil {
		t.Fatal(
			"Create() returned nil error for a nil context",
		)
	}

	_, err = store.GetByID(
		nil,
		workflow.CompanyID("company-1"),
		repository.DefinitionSnapshotID(
			"snapshot-1",
		),
	)
	if err == nil {
		t.Fatal(
			"GetByID() returned nil error for a nil context",
		)
	}
}

func TestSnapshotRepositoryPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedSnapshotTestStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err := store.Create(
		ctx,
		newPostgreSQLSnapshotTestValue(t),
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"Create() error = %v, want context.Canceled",
			err,
		)
	}

	_, err = store.GetByID(
		ctx,
		workflow.CompanyID("company-1"),
		repository.DefinitionSnapshotID(
			"snapshot-1",
		),
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"GetByID() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestSnapshotRepositoryRejectsInvalidCreateValue(
	t *testing.T,
) {
	store := newUnconnectedSnapshotTestStore(t)

	err := store.Create(
		context.Background(),
		repository.DefinitionSnapshot{},
	)
	if err == nil {
		t.Fatal(
			"Create() accepted an invalid definition snapshot",
		)
	}
}

func TestSnapshotRepositoryRejectsInvalidLookupIdentifiers(
	t *testing.T,
) {
	store := newUnconnectedSnapshotTestStore(t)

	tests := []struct {
		name       string
		companyID  workflow.CompanyID
		snapshotID repository.DefinitionSnapshotID
	}{
		{
			name:      "blank company ID",
			companyID: workflow.CompanyID(" "),
			snapshotID: repository.DefinitionSnapshotID(
				"snapshot-1",
			),
		},
		{
			name:       "blank snapshot ID",
			companyID:  workflow.CompanyID("company-1"),
			snapshotID: repository.DefinitionSnapshotID(" "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.GetByID(
				context.Background(),
				test.companyID,
				test.snapshotID,
			)
			if err == nil {
				t.Fatal(
					"GetByID() accepted an invalid identifier",
				)
			}
		})
	}
}

func newUnconnectedSnapshotTestStore(
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

func newPostgreSQLSnapshotTestValue(
	t *testing.T,
) repository.DefinitionSnapshot {
	t.Helper()

	snapshot, err :=
		repository.NewDefinitionSnapshot(
			repository.DefinitionSnapshotID(
				"snapshot-1",
			),
			workflow.CompanyID(
				"company-1",
			),
			workflow.WorkflowID(
				"workflow-1",
			),
			3,
			"Snapshot Repository Test",
			[]byte(
				`{"nodes":[],"edges":[]}`,
			),
			time.Date(
				2026,
				time.July,
				17,
				18,
				0,
				0,
				0,
				time.UTC,
			),
		)
	if err != nil {
		t.Fatalf(
			"NewDefinitionSnapshot() returned an unexpected error: %v",
			err,
		)
	}

	return snapshot
}

func TestNewStoreRejectsNilPool(
	t *testing.T,
) {
	store, err := NewStore(nil)
	if err == nil {
		t.Fatal(
			"NewStore() returned nil error for a nil pool",
		)
	}

	if store != nil {
		t.Fatal(
			"NewStore() returned a store for a nil pool",
		)
	}
}

func TestNewStoreCreatesValidStore(
	t *testing.T,
) {
	pool := new(pgxpool.Pool)

	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf(
			"NewStore() returned an unexpected error: %v",
			err,
		)
	}

	if store == nil {
		t.Fatal(
			"NewStore() returned a nil store",
		)
	}

	if store.pool != pool {
		t.Fatal(
			"NewStore() did not preserve the provided pool",
		)
	}

	if !store.IsValid() {
		t.Fatal(
			"new PostgreSQL store was reported as invalid",
		)
	}
}

func TestStoreZeroValueIsInvalid(
	t *testing.T,
) {
	var store Store

	if store.IsValid() {
		t.Fatal(
			"zero-value PostgreSQL store was reported as valid",
		)
	}

	var pointer *Store

	if pointer.IsValid() {
		t.Fatal(
			"nil PostgreSQL store was reported as valid",
		)
	}
}

func writeTestFile(
	path string,
) error {
	return os.WriteFile(
		path,
		[]byte("-- test migration"),
		0o600,
	)
}

type transactionTestTx struct {
	commitCalls   int
	rollbackCalls int

	commitErr   error
	rollbackErr error
}

var _ pgx.Tx = (*transactionTestTx)(nil)

func (
	tx *transactionTestTx,
) Begin(
	context.Context,
) (pgx.Tx, error) {
	return nil, errors.New(
		"nested transaction is not supported by this test double",
	)
}

func (
	tx *transactionTestTx,
) Commit(
	context.Context,
) error {
	tx.commitCalls++
	return tx.commitErr
}

func (
	tx *transactionTestTx,
) Rollback(
	context.Context,
) error {
	tx.rollbackCalls++
	return tx.rollbackErr
}

func (
	*transactionTestTx,
) CopyFrom(
	context.Context,
	pgx.Identifier,
	[]string,
	pgx.CopyFromSource,
) (int64, error) {
	return 0, nil
}

func (
	*transactionTestTx,
) SendBatch(
	context.Context,
	*pgx.Batch,
) pgx.BatchResults {
	return nil
}

func (
	*transactionTestTx,
) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (
	*transactionTestTx,
) Prepare(
	context.Context,
	string,
	string,
) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (
	*transactionTestTx,
) Exec(
	context.Context,
	string,
	...any,
) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (
	*transactionTestTx,
) Query(
	context.Context,
	string,
	...any,
) (pgx.Rows, error) {
	return nil, nil
}

func (
	*transactionTestTx,
) QueryRow(
	context.Context,
	string,
	...any,
) pgx.Row {
	return nil
}

func (
	*transactionTestTx,
) Conn() *pgx.Conn {
	return nil
}

func TestWithinTransactionCommitsSuccessfulWork(
	t *testing.T,
) {
	tx := &transactionTestTx{}

	beginCalls := 0

	store := newTransactionTestStore(
		func(
			context.Context,
			pgx.TxOptions,
		) (pgx.Tx, error) {
			beginCalls++
			return tx, nil
		},
	)

	workCalls := 0

	err := store.withinTransaction(
		context.Background(),
		"create execution",
		func(actual pgx.Tx) error {
			workCalls++

			if actual != tx {
				t.Fatal(
					"transaction work received a different transaction",
				)
			}

			return nil
		},
	)
	if err != nil {
		t.Fatalf(
			"withinTransaction() returned an unexpected error: %v",
			err,
		)
	}

	if beginCalls != 1 {
		t.Fatalf(
			"begin call count = %d, want 1",
			beginCalls,
		)
	}

	if workCalls != 1 {
		t.Fatalf(
			"work call count = %d, want 1",
			workCalls,
		)
	}

	if tx.commitCalls != 1 {
		t.Fatalf(
			"commit call count = %d, want 1",
			tx.commitCalls,
		)
	}

	if tx.rollbackCalls != 0 {
		t.Fatalf(
			"rollback call count = %d, want 0",
			tx.rollbackCalls,
		)
	}
}

func TestWithinTransactionRollsBackFailedWork(
	t *testing.T,
) {
	tx := &transactionTestTx{}

	expected := errors.New(
		"repository write failed",
	)

	store := newTransactionTestStore(
		func(
			context.Context,
			pgx.TxOptions,
		) (pgx.Tx, error) {
			return tx, nil
		},
	)

	err := store.withinTransaction(
		context.Background(),
		"create execution",
		func(pgx.Tx) error {
			return expected
		},
	)

	if !errors.Is(
		err,
		expected,
	) {
		t.Fatalf(
			"withinTransaction() error = %v, want %v",
			err,
			expected,
		)
	}

	if tx.commitCalls != 0 {
		t.Fatalf(
			"commit call count = %d, want 0",
			tx.commitCalls,
		)
	}

	if tx.rollbackCalls != 1 {
		t.Fatalf(
			"rollback call count = %d, want 1",
			tx.rollbackCalls,
		)
	}
}

func TestWithinTransactionRollsBackAfterCommitFailure(
	t *testing.T,
) {
	commitError := errors.New(
		"commit connection failure",
	)

	tx := &transactionTestTx{
		commitErr: commitError,
	}

	store := newTransactionTestStore(
		func(
			context.Context,
			pgx.TxOptions,
		) (pgx.Tx, error) {
			return tx, nil
		},
	)

	err := store.withinTransaction(
		context.Background(),
		"apply node transition",
		func(pgx.Tx) error {
			return nil
		},
	)

	if !errors.Is(
		err,
		commitError,
	) {
		t.Fatalf(
			"withinTransaction() error = %v, want commit error",
			err,
		)
	}

	if tx.commitCalls != 1 {
		t.Fatalf(
			"commit call count = %d, want 1",
			tx.commitCalls,
		)
	}

	if tx.rollbackCalls != 1 {
		t.Fatalf(
			"rollback call count = %d, want 1",
			tx.rollbackCalls,
		)
	}
}

func TestWithinTransactionRejectsInvalidInputs(
	t *testing.T,
) {
	validStore := newTransactionTestStore(
		func(
			context.Context,
			pgx.TxOptions,
		) (pgx.Tx, error) {
			return &transactionTestTx{}, nil
		},
	)

	tests := []struct {
		name      string
		store     *Store
		context   context.Context
		operation string
		work      transactionWork
	}{
		{
			name:      "nil store",
			store:     nil,
			context:   context.Background(),
			operation: "create execution",
			work: func(pgx.Tx) error {
				return nil
			},
		},
		{
			name:      "nil context",
			store:     validStore,
			context:   nil,
			operation: "create execution",
			work: func(pgx.Tx) error {
				return nil
			},
		},
		{
			name:      "blank operation",
			store:     validStore,
			context:   context.Background(),
			operation: " ",
			work: func(pgx.Tx) error {
				return nil
			},
		},
		{
			name:      "nil work",
			store:     validStore,
			context:   context.Background(),
			operation: "create execution",
			work:      nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.store.withinTransaction(
				test.context,
				test.operation,
				test.work,
			)

			if err == nil {
				t.Fatal(
					"withinTransaction() returned nil error",
				)
			}
		})
	}
}

func TestWithinTransactionDoesNotBeginForCancelledContext(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	beginCalls := 0

	store := newTransactionTestStore(
		func(
			context.Context,
			pgx.TxOptions,
		) (pgx.Tx, error) {
			beginCalls++
			return &transactionTestTx{}, nil
		},
	)

	err := store.withinTransaction(
		ctx,
		"create execution",
		func(pgx.Tx) error {
			return nil
		},
	)

	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"withinTransaction() error = %v, want context.Canceled",
			err,
		)
	}

	if beginCalls != 0 {
		t.Fatalf(
			"begin call count = %d, want 0",
			beginCalls,
		)
	}
}

func newTransactionTestStore(
	begin beginTransactionFunc,
) *Store {
	return &Store{
		pool:    new(pgxpool.Pool),
		beginTx: begin,
	}
}

func TestWorkflowExecutionReaderIntegration(
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

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	companyA := workflow.CompanyID(
		"company-execution-a-" + suffix,
	)

	companyB := workflow.CompanyID(
		"company-execution-b-" + suffix,
	)

	workflowA := workflow.WorkflowID(
		"workflow-a-" + suffix,
	)

	workflowB := workflow.WorkflowID(
		"workflow-b-" + suffix,
	)

	snapshotA :=
		newWorkflowExecutionReadSnapshot(
			t,
			companyA,
			workflowA,
			repository.DefinitionSnapshotID(
				"snapshot-a-"+suffix,
			),
		)

	snapshotB :=
		newWorkflowExecutionReadSnapshot(
			t,
			companyA,
			workflowB,
			repository.DefinitionSnapshotID(
				"snapshot-b-"+suffix,
			),
		)

	t.Cleanup(func() {
		cleanupContext, cancelCleanup :=
			context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
		defer cancelCleanup()

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_executions
WHERE company_id = $1
`,
			companyA.String(),
		); err != nil {
			t.Errorf(
				"workflow execution cleanup failed: %v",
				err,
			)
		}

		if _, err := store.pool.Exec(
			cleanupContext,
			`
DELETE FROM workflow_definition_snapshots
WHERE company_id = $1
`,
			companyA.String(),
		); err != nil {
			t.Errorf(
				"snapshot cleanup failed: %v",
				err,
			)
		}
	})

	if err := store.Create(
		ctx,
		snapshotA,
	); err != nil {
		t.Fatalf(
			"create snapshot A: %v",
			err,
		)
	}

	if err := store.Create(
		ctx,
		snapshotB,
	); err != nil {
		t.Fatalf(
			"create snapshot B: %v",
			err,
		)
	}

	baseTime := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	records := []repository.WorkflowExecutionRecord{
		newWorkflowExecutionReadRecord(
			t,
			execution.WorkflowExecutionID(
				"execution-1-"+suffix,
			),
			companyA,
			workflowA,
			snapshotA.ID(),
			execution.WorkflowExecutionStatusCreated,
			baseTime.Add(1*time.Minute),
		),
		newWorkflowExecutionReadRecord(
			t,
			execution.WorkflowExecutionID(
				"execution-2-"+suffix,
			),
			companyA,
			workflowA,
			snapshotA.ID(),
			execution.WorkflowExecutionStatusRunning,
			baseTime.Add(2*time.Minute),
		),
		newWorkflowExecutionReadRecord(
			t,
			execution.WorkflowExecutionID(
				"execution-3-"+suffix,
			),
			companyA,
			workflowB,
			snapshotB.ID(),
			execution.WorkflowExecutionStatusFailed,
			baseTime.Add(3*time.Minute),
		),
		newWorkflowExecutionReadRecord(
			t,
			execution.WorkflowExecutionID(
				"execution-4-"+suffix,
			),
			companyA,
			workflowA,
			snapshotA.ID(),
			execution.WorkflowExecutionStatusFailed,
			baseTime.Add(4*time.Minute),
		),
	}

	for _, record := range records {
		insertWorkflowExecutionReadFixture(
			t,
			ctx,
			store,
			record,
		)
	}

	actual, err := store.GetWorkflowExecution(
		ctx,
		companyA,
		records[3].ID(),
	)
	if err != nil {
		t.Fatalf(
			"GetWorkflowExecution() error: %v",
			err,
		)
	}

	assertWorkflowExecutionRecordsEqual(
		t,
		actual,
		records[3],
	)

	_, crossTenantErr :=
		store.GetWorkflowExecution(
			ctx,
			companyB,
			records[3].ID(),
		)
	if !repository.IsNotFound(
		crossTenantErr,
	) {
		t.Fatalf(
			"cross-tenant GetWorkflowExecution() error = %v, want NOT_FOUND",
			crossTenantErr,
		)
	}

	_, missingErr :=
		store.GetWorkflowExecution(
			ctx,
			companyA,
			execution.WorkflowExecutionID(
				"missing-execution-"+suffix,
			),
		)
	if !repository.IsNotFound(
		missingErr,
	) {
		t.Fatalf(
			"missing GetWorkflowExecution() error = %v, want NOT_FOUND",
			missingErr,
		)
	}

	emptyFilter, err :=
		repository.NewWorkflowExecutionFilter(
			"",
			"",
		)
	if err != nil {
		t.Fatalf(
			"create empty filter: %v",
			err,
		)
	}

	firstRequest, err :=
		repository.NewPageRequest(
			2,
			"",
		)
	if err != nil {
		t.Fatalf(
			"create first page request: %v",
			err,
		)
	}

	firstPage, err :=
		store.ListWorkflowExecutions(
			ctx,
			companyA,
			emptyFilter,
			firstRequest,
		)
	if err != nil {
		t.Fatalf(
			"ListWorkflowExecutions() first page error: %v",
			err,
		)
	}

	assertWorkflowExecutionPageIDs(
		t,
		firstPage.Items(),
		[]execution.WorkflowExecutionID{
			records[3].ID(),
			records[2].ID(),
		},
	)

	nextToken, exists :=
		firstPage.Next()
	if !exists {
		t.Fatal(
			"first workflow execution page has no next token",
		)
	}

	secondRequest, err :=
		repository.NewPageRequest(
			2,
			nextToken,
		)
	if err != nil {
		t.Fatalf(
			"create second page request: %v",
			err,
		)
	}

	secondPage, err :=
		store.ListWorkflowExecutions(
			ctx,
			companyA,
			emptyFilter,
			secondRequest,
		)
	if err != nil {
		t.Fatalf(
			"ListWorkflowExecutions() second page error: %v",
			err,
		)
	}

	assertWorkflowExecutionPageIDs(
		t,
		secondPage.Items(),
		[]execution.WorkflowExecutionID{
			records[1].ID(),
			records[0].ID(),
		},
	)

	if secondPage.HasNext() {
		t.Fatal(
			"second workflow execution page unexpectedly has a next token",
		)
	}

	failedFilter, err :=
		repository.NewWorkflowExecutionFilter(
			"",
			execution.WorkflowExecutionStatusFailed,
		)
	if err != nil {
		t.Fatalf(
			"create failed filter: %v",
			err,
		)
	}

	fullPageRequest, err :=
		repository.NewPageRequest(
			20,
			"",
		)
	if err != nil {
		t.Fatalf(
			"create full page request: %v",
			err,
		)
	}

	failedPage, err :=
		store.ListWorkflowExecutions(
			ctx,
			companyA,
			failedFilter,
			fullPageRequest,
		)
	if err != nil {
		t.Fatalf(
			"list failed executions: %v",
			err,
		)
	}

	assertWorkflowExecutionPageIDs(
		t,
		failedPage.Items(),
		[]execution.WorkflowExecutionID{
			records[3].ID(),
			records[2].ID(),
		},
	)

	workflowFilter, err :=
		repository.NewWorkflowExecutionFilter(
			workflowA,
			"",
		)
	if err != nil {
		t.Fatalf(
			"create workflow filter: %v",
			err,
		)
	}

	workflowPage, err :=
		store.ListWorkflowExecutions(
			ctx,
			companyA,
			workflowFilter,
			fullPageRequest,
		)
	if err != nil {
		t.Fatalf(
			"list workflow executions by workflow: %v",
			err,
		)
	}

	assertWorkflowExecutionPageIDs(
		t,
		workflowPage.Items(),
		[]execution.WorkflowExecutionID{
			records[3].ID(),
			records[1].ID(),
			records[0].ID(),
		},
	)

	combinedFilter, err :=
		repository.NewWorkflowExecutionFilter(
			workflowA,
			execution.WorkflowExecutionStatusFailed,
		)
	if err != nil {
		t.Fatalf(
			"create combined filter: %v",
			err,
		)
	}

	combinedPage, err :=
		store.ListWorkflowExecutions(
			ctx,
			companyA,
			combinedFilter,
			fullPageRequest,
		)
	if err != nil {
		t.Fatalf(
			"list combined workflow filter: %v",
			err,
		)
	}

	assertWorkflowExecutionPageIDs(
		t,
		combinedPage.Items(),
		[]execution.WorkflowExecutionID{
			records[3].ID(),
		},
	)

	tenantBPage, err :=
		store.ListWorkflowExecutions(
			ctx,
			companyB,
			emptyFilter,
			fullPageRequest,
		)
	if err != nil {
		t.Fatalf(
			"list tenant B executions: %v",
			err,
		)
	}

	if actualCount :=
		len(tenantBPage.Items()); actualCount != 0 {
		t.Fatalf(
			"tenant B execution count = %d, want 0",
			actualCount,
		)
	}
}

func newWorkflowExecutionReadSnapshot(
	t *testing.T,
	companyID workflow.CompanyID,
	workflowID workflow.WorkflowID,
	snapshotID repository.DefinitionSnapshotID,
) repository.DefinitionSnapshot {
	t.Helper()

	snapshot, err :=
		repository.NewDefinitionSnapshot(
			snapshotID,
			companyID,
			workflowID,
			1,
			"Workflow Execution Reader",
			[]byte(
				`{"nodes":[],"edges":[]}`,
			),
			time.Date(
				2026,
				time.July,
				17,
				17,
				0,
				0,
				0,
				time.UTC,
			),
		)
	if err != nil {
		t.Fatalf(
			"NewDefinitionSnapshot() error: %v",
			err,
		)
	}

	return snapshot
}

func newWorkflowExecutionReadRecord(
	t *testing.T,
	id execution.WorkflowExecutionID,
	companyID workflow.CompanyID,
	workflowID workflow.WorkflowID,
	snapshotID repository.DefinitionSnapshotID,
	status execution.WorkflowExecutionStatus,
	createdAt time.Time,
) repository.WorkflowExecutionRecord {
	t.Helper()

	params := repository.WorkflowExecutionRecordParams{
		ID:                 id,
		CompanyID:          companyID,
		WorkflowID:         workflowID,
		WorkflowRevision:   1,
		SnapshotID:         snapshotID,
		Mode:               execution.ExecutionModeSync,
		CorrelationID:      "correlation-" + id.String(),
		Status:             status,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
		TerminalOutputs:    []byte(`{}`),
		IsStalled:          false,
		NextSequenceNumber: repository.SequenceNumber(5),
		LockVersion:        2,
	}

	switch status {
	case execution.WorkflowExecutionStatusCreated:
		params.CorrelationID = ""

	case execution.WorkflowExecutionStatusRunning:
		params.ValidatingAt =
			createdAt.Add(5 * time.Second)

		params.StartedAt =
			createdAt.Add(10 * time.Second)

		params.UpdatedAt =
			params.StartedAt

	case execution.WorkflowExecutionStatusFailed:
		params.ValidatingAt =
			createdAt.Add(5 * time.Second)

		params.StartedAt =
			createdAt.Add(10 * time.Second)

		params.FinishedAt =
			createdAt.Add(15 * time.Second)

		params.UpdatedAt =
			params.FinishedAt

		params.FailureSummary =
			[]byte(
				`{"code":"WORKFLOW_FAILED"}`,
			)

	default:
		t.Fatalf(
			"unsupported workflow execution test status: %q",
			status,
		)
	}

	record, err :=
		repository.NewWorkflowExecutionRecord(
			params,
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() error: %v",
			err,
		)
	}

	return record
}

func insertWorkflowExecutionReadFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	record repository.WorkflowExecutionRecord,
) {
	t.Helper()

	var correlationID any

	if value, exists :=
		record.CorrelationID(); exists {
		correlationID = value
	}

	var validatingAt any

	if value, exists :=
		record.ValidatingAt(); exists {
		validatingAt = value
	}

	var queuedAt any

	if value, exists :=
		record.QueuedAt(); exists {
		queuedAt = value
	}

	var startedAt any

	if value, exists :=
		record.StartedAt(); exists {
		startedAt = value
	}

	var finishedAt any

	if value, exists :=
		record.FinishedAt(); exists {
		finishedAt = value
	}

	var failureSummary any

	if value, exists :=
		record.FailureSummary(); exists {
		failureSummary =
			value.String()
	}

	_, err := store.pool.Exec(
		ctx,
		`
INSERT INTO workflow_executions (
	workflow_execution_id,
	company_id,
	workflow_id,
	workflow_revision,
	snapshot_id,
	mode,
	correlation_id,
	status,
	created_at,
	validating_at,
	queued_at,
	started_at,
	finished_at,
	updated_at,
	terminal_outputs,
	failure_summary,
	is_stalled,
	next_sequence_number,
	lock_version
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10,
	$11,
	$12,
	$13,
	$14,
	$15::jsonb,
	$16::jsonb,
	$17,
	$18,
	$19
)
`,
		record.ID().String(),
		record.CompanyID().String(),
		record.WorkflowID().String(),
		int64(record.WorkflowRevision()),
		record.SnapshotID().String(),
		record.Mode().String(),
		correlationID,
		record.Status().String(),
		record.CreatedAt(),
		validatingAt,
		queuedAt,
		startedAt,
		finishedAt,
		record.UpdatedAt(),
		record.TerminalOutputs().String(),
		failureSummary,
		record.IsStalled(),
		record.NextSequenceNumber().Int64(),
		record.LockVersion(),
	)
	if err != nil {
		t.Fatalf(
			"insert workflow execution fixture: %v",
			err,
		)
	}
}

func assertWorkflowExecutionPageIDs(
	t *testing.T,
	records []repository.WorkflowExecutionRecord,
	expected []execution.WorkflowExecutionID,
) {
	t.Helper()

	if len(records) != len(expected) {
		t.Fatalf(
			"record count = %d, want %d",
			len(records),
			len(expected),
		)
	}

	for index, expectedID := range expected {
		if records[index].ID() != expectedID {
			t.Fatalf(
				"record[%d] ID = %q, want %q",
				index,
				records[index].ID(),
				expectedID,
			)
		}
	}
}

func assertWorkflowExecutionRecordsEqual(
	t *testing.T,
	actual repository.WorkflowExecutionRecord,
	expected repository.WorkflowExecutionRecord,
) {
	t.Helper()

	if actual.ID() != expected.ID() {
		t.Fatalf(
			"execution ID = %q, want %q",
			actual.ID(),
			expected.ID(),
		)
	}

	if actual.CompanyID() != expected.CompanyID() {
		t.Fatalf(
			"company ID = %q, want %q",
			actual.CompanyID(),
			expected.CompanyID(),
		)
	}

	if actual.WorkflowID() != expected.WorkflowID() {
		t.Fatalf(
			"workflow ID = %q, want %q",
			actual.WorkflowID(),
			expected.WorkflowID(),
		)
	}

	if actual.WorkflowRevision() !=
		expected.WorkflowRevision() {
		t.Fatalf(
			"workflow revision = %d, want %d",
			actual.WorkflowRevision(),
			expected.WorkflowRevision(),
		)
	}

	if actual.SnapshotID() != expected.SnapshotID() {
		t.Fatalf(
			"snapshot ID = %q, want %q",
			actual.SnapshotID(),
			expected.SnapshotID(),
		)
	}

	if actual.Mode() != expected.Mode() {
		t.Fatalf(
			"mode = %q, want %q",
			actual.Mode(),
			expected.Mode(),
		)
	}

	actualCorrelation, actualHasCorrelation :=
		actual.CorrelationID()

	expectedCorrelation, expectedHasCorrelation :=
		expected.CorrelationID()

	if actualHasCorrelation != expectedHasCorrelation ||
		actualCorrelation != expectedCorrelation {
		t.Fatalf(
			"correlation ID = %q, %t; want %q, %t",
			actualCorrelation,
			actualHasCorrelation,
			expectedCorrelation,
			expectedHasCorrelation,
		)
	}

	if actual.Status() != expected.Status() {
		t.Fatalf(
			"status = %q, want %q",
			actual.Status(),
			expected.Status(),
		)
	}

	if !actual.CreatedAt().Equal(
		expected.CreatedAt(),
	) {
		t.Fatalf(
			"createdAt = %v, want %v",
			actual.CreatedAt(),
			expected.CreatedAt(),
		)
	}

	assertWorkflowExecutionOptionalTime(
		t,
		"validatingAt",
		actual.ValidatingAt,
		expected.ValidatingAt,
	)

	assertWorkflowExecutionOptionalTime(
		t,
		"queuedAt",
		actual.QueuedAt,
		expected.QueuedAt,
	)

	assertWorkflowExecutionOptionalTime(
		t,
		"startedAt",
		actual.StartedAt,
		expected.StartedAt,
	)

	assertWorkflowExecutionOptionalTime(
		t,
		"finishedAt",
		actual.FinishedAt,
		expected.FinishedAt,
	)

	if !actual.UpdatedAt().Equal(
		expected.UpdatedAt(),
	) {
		t.Fatalf(
			"updatedAt = %v, want %v",
			actual.UpdatedAt(),
			expected.UpdatedAt(),
		)
	}

	assertEquivalentJSON(
		t,
		actual.TerminalOutputs().Bytes(),
		expected.TerminalOutputs().Bytes(),
	)

	actualFailure, actualHasFailure :=
		actual.FailureSummary()

	expectedFailure, expectedHasFailure :=
		expected.FailureSummary()

	if actualHasFailure != expectedHasFailure {
		t.Fatalf(
			"failure summary exists = %t, want %t",
			actualHasFailure,
			expectedHasFailure,
		)
	}

	if actualHasFailure {
		assertEquivalentJSON(
			t,
			actualFailure.Bytes(),
			expectedFailure.Bytes(),
		)
	}

	if actual.IsStalled() != expected.IsStalled() {
		t.Fatalf(
			"isStalled = %t, want %t",
			actual.IsStalled(),
			expected.IsStalled(),
		)
	}

	if actual.NextSequenceNumber() !=
		expected.NextSequenceNumber() {
		t.Fatalf(
			"next sequence = %d, want %d",
			actual.NextSequenceNumber(),
			expected.NextSequenceNumber(),
		)
	}

	if actual.LockVersion() !=
		expected.LockVersion() {
		t.Fatalf(
			"lock version = %d, want %d",
			actual.LockVersion(),
			expected.LockVersion(),
		)
	}
}

func assertWorkflowExecutionOptionalTime(
	t *testing.T,
	field string,
	actualGetter func() (time.Time, bool),
	expectedGetter func() (time.Time, bool),
) {
	t.Helper()

	actual, actualExists :=
		actualGetter()

	expected, expectedExists :=
		expectedGetter()

	if actualExists != expectedExists {
		t.Fatalf(
			"%s exists = %t, want %t",
			field,
			actualExists,
			expectedExists,
		)
	}

	if actualExists &&
		!actual.Equal(expected) {
		t.Fatalf(
			"%s = %v, want %v",
			field,
			actual,
			expected,
		)
	}
}

func TestWorkflowExecutionReaderContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.WorkflowExecutionReader = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement WorkflowExecutionReader",
		)
	}
}

func TestWorkflowExecutionReaderRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	_, err := store.GetWorkflowExecution(
		context.Background(),
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID(
			"execution-1",
		),
	)
	if err == nil {
		t.Fatal(
			"GetWorkflowExecution() returned nil error for nil store",
		)
	}

	filter, err :=
		repository.NewWorkflowExecutionFilter(
			"",
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() error: %v",
			err,
		)
	}

	pageRequest, err :=
		repository.NewPageRequest(
			10,
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() error: %v",
			err,
		)
	}

	_, err = store.ListWorkflowExecutions(
		context.Background(),
		workflow.CompanyID("company-1"),
		filter,
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListWorkflowExecutions() returned nil error for nil store",
		)
	}
}

func TestWorkflowExecutionReaderRejectsNilContexts(
	t *testing.T,
) {
	store :=
		newUnconnectedWorkflowExecutionReaderTestStore(
			t,
		)

	_, err := store.GetWorkflowExecution(
		nil,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID(
			"execution-1",
		),
	)
	if err == nil {
		t.Fatal(
			"GetWorkflowExecution() accepted nil context",
		)
	}

	filter, err :=
		repository.NewWorkflowExecutionFilter(
			"",
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() error: %v",
			err,
		)
	}

	pageRequest, err :=
		repository.NewPageRequest(
			10,
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() error: %v",
			err,
		)
	}

	_, err = store.ListWorkflowExecutions(
		nil,
		workflow.CompanyID("company-1"),
		filter,
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListWorkflowExecutions() accepted nil context",
		)
	}
}

func TestWorkflowExecutionReaderPreservesCancelledContext(
	t *testing.T,
) {
	store :=
		newUnconnectedWorkflowExecutionReaderTestStore(
			t,
		)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	_, err := store.GetWorkflowExecution(
		ctx,
		workflow.CompanyID("company-1"),
		execution.WorkflowExecutionID(
			"execution-1",
		),
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"GetWorkflowExecution() error = %v, want context.Canceled",
			err,
		)
	}

	filter, err :=
		repository.NewWorkflowExecutionFilter(
			"",
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() error: %v",
			err,
		)
	}

	pageRequest, err :=
		repository.NewPageRequest(
			10,
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() error: %v",
			err,
		)
	}

	_, err = store.ListWorkflowExecutions(
		ctx,
		workflow.CompanyID("company-1"),
		filter,
		pageRequest,
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"ListWorkflowExecutions() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestWorkflowExecutionReaderRejectsInvalidIdentifiers(
	t *testing.T,
) {
	store :=
		newUnconnectedWorkflowExecutionReaderTestStore(
			t,
		)

	tests := []struct {
		name                string
		companyID           workflow.CompanyID
		workflowExecutionID execution.WorkflowExecutionID
	}{
		{
			name:      "blank company ID",
			companyID: workflow.CompanyID(" "),
			workflowExecutionID: execution.WorkflowExecutionID(
				"execution-1",
			),
		},
		{
			name:                "blank execution ID",
			companyID:           workflow.CompanyID("company-1"),
			workflowExecutionID: execution.WorkflowExecutionID(" "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err :=
				store.GetWorkflowExecution(
					context.Background(),
					test.companyID,
					test.workflowExecutionID,
				)

			if err == nil {
				t.Fatal(
					"GetWorkflowExecution() accepted invalid identifiers",
				)
			}
		})
	}
}

func TestWorkflowExecutionReaderRejectsInvalidListCompanyID(
	t *testing.T,
) {
	store :=
		newUnconnectedWorkflowExecutionReaderTestStore(
			t,
		)

	filter, err :=
		repository.NewWorkflowExecutionFilter(
			"",
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() error: %v",
			err,
		)
	}

	pageRequest, err :=
		repository.NewPageRequest(
			10,
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() error: %v",
			err,
		)
	}

	_, err = store.ListWorkflowExecutions(
		context.Background(),
		workflow.CompanyID(" "),
		filter,
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListWorkflowExecutions() accepted blank company ID",
		)
	}
}

func TestWorkflowExecutionReaderRejectsMalformedCursor(
	t *testing.T,
) {
	store :=
		newUnconnectedWorkflowExecutionReaderTestStore(
			t,
		)

	filter, err :=
		repository.NewWorkflowExecutionFilter(
			"",
			"",
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() error: %v",
			err,
		)
	}

	pageRequest, err :=
		repository.NewPageRequest(
			10,
			repository.PageToken("***"),
		)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() error: %v",
			err,
		)
	}

	_, err = store.ListWorkflowExecutions(
		context.Background(),
		workflow.CompanyID("company-1"),
		filter,
		pageRequest,
	)
	if err == nil {
		t.Fatal(
			"ListWorkflowExecutions() accepted malformed cursor",
		)
	}

	var validationError *repository.ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"ListWorkflowExecutions() error type = %T, want *repository.ValidationError",
			err,
		)
	}

	if validationError.Field !=
		"pageToken" {

		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			"pageToken",
		)
	}
}

func TestBuildWorkflowExecutionListQueryUsesTenantFiltersAndCursor(
	t *testing.T,
) {
	filter, err :=
		repository.NewWorkflowExecutionFilter(
			workflow.WorkflowID(
				"workflow-1",
			),
			execution.WorkflowExecutionStatusFailed,
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() error: %v",
			err,
		)
	}

	cursorTime := time.Date(
		2026,
		time.July,
		17,
		18,
		0,
		0,
		0,
		time.UTC,
	)

	cursor, err := encodeTimestampCursor(
		cursorKindWorkflowExecutions,
		cursorTime,
		"execution-9",
	)
	if err != nil {
		t.Fatalf(
			"encodeTimestampCursor() error: %v",
			err,
		)
	}

	pageRequest, err :=
		repository.NewPageRequest(
			25,
			cursor,
		)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() error: %v",
			err,
		)
	}

	query, arguments, err :=
		buildWorkflowExecutionListQuery(
			workflow.CompanyID(
				"company-1",
			),
			filter,
			pageRequest,
		)
	if err != nil {
		t.Fatalf(
			"buildWorkflowExecutionListQuery() error: %v",
			err,
		)
	}

	requiredFragments := []string{
		"WHERE company_id = $1",
		"workflow_id = $2",
		"status = $3",
		"(created_at, workflow_execution_id) < ($4, $5)",
		"ORDER BY created_at DESC, workflow_execution_id DESC",
		"LIMIT $6",
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(
			query,
			fragment,
		) {
			t.Fatalf(
				"query does not contain %q:\n%s",
				fragment,
				query,
			)
		}
	}

	if len(arguments) != 6 {
		t.Fatalf(
			"argument count = %d, want 6",
			len(arguments),
		)
	}

	if arguments[0] != "company-1" {
		t.Fatalf(
			"company argument = %#v",
			arguments[0],
		)
	}

	if arguments[1] != "workflow-1" {
		t.Fatalf(
			"workflow argument = %#v",
			arguments[1],
		)
	}

	if arguments[2] != "FAILED" {
		t.Fatalf(
			"status argument = %#v",
			arguments[2],
		)
	}

	if arguments[4] != "execution-9" {
		t.Fatalf(
			"execution cursor argument = %#v",
			arguments[4],
		)
	}

	if arguments[5] != 26 {
		t.Fatalf(
			"limit argument = %#v, want 26",
			arguments[5],
		)
	}
}

func newUnconnectedWorkflowExecutionReaderTestStore(
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

const workflowTransitionExpectedLockVersion int64 = 5

var workflowTransitionExpectedSequence = repository.SequenceNumber(20)

func TestWorkflowTransitionStoreIntegration(
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

	requirePostgreSQLIntegrationTable(
		t,
		ctx,
		store,
		"execution_errors",
	)

	suffix := fmt.Sprintf(
		"%d",
		time.Now().UTC().UnixNano(),
	)

	successFixture :=
		newWorkflowTransitionIntegrationFixture(
			t,
			"success-"+suffix,
			"",
		)

	sharedErrorID :=
		repository.ExecutionErrorID(
			"shared-workflow-transition-error-" +
				suffix,
		)

	rollbackHolder :=
		newWorkflowTransitionIntegrationFixture(
			t,
			"rollback-holder-"+suffix,
			sharedErrorID.String(),
		)

	rollbackTarget :=
		newWorkflowTransitionIntegrationFixture(
			t,
			"rollback-target-"+suffix,
			sharedErrorID.String(),
		)

	fixtures := []workflowTransitionIntegrationFixture{
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
		"persists workflow event log and structured error atomically",
		func(t *testing.T) {
			prepareWorkflowTransitionIntegrationFixture(
				t,
				ctx,
				store,
				successFixture,
			)

			err := store.ApplyWorkflowTransition(
				ctx,
				successFixture.command,
			)
			if err != nil {
				t.Fatalf(
					"ApplyWorkflowTransition() returned an error: %v",
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
				execution.WorkflowExecutionStatusFailed {
				t.Fatalf(
					"workflow status = %q, want FAILED",
					actualWorkflow.Status(),
				)
			}

			if actualWorkflow.LockVersion() !=
				workflowTransitionExpectedLockVersion+1 {
				t.Fatalf(
					"workflow lock version = %d, want %d",
					actualWorkflow.LockVersion(),
					workflowTransitionExpectedLockVersion+1,
				)
			}

			expectedNextSequence :=
				repository.SequenceNumber(
					workflowTransitionExpectedSequence.
						Int64() + 2,
				)

			if actualWorkflow.NextSequenceNumber() !=
				expectedNextSequence {
				t.Fatalf(
					"workflow next sequence = %d, want %d",
					actualWorkflow.NextSequenceNumber(),
					expectedNextSequence,
				)
			}

			failureSummary, exists :=
				actualWorkflow.FailureSummary()
			if !exists {
				t.Fatal(
					"workflow failure summary was not persisted",
				)
			}

			assertEquivalentJSON(
				t,
				failureSummary.Bytes(),
				[]byte(
					`{"code":"WORKFLOW_FAILED"}`,
				),
			)

			finishedAt, exists :=
				actualWorkflow.FinishedAt()
			if !exists {
				t.Fatal(
					"workflow finished time was not persisted",
				)
			}

			if !finishedAt.Equal(
				successFixture.transitionAt,
			) {
				t.Fatalf(
					"workflow finished time = %v, want %v",
					finishedAt,
					successFixture.transitionAt,
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
					successFixture.workflowExecutionID,
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

			eventRecord := eventPage.Items()[0]

			if eventRecord.ID() !=
				successFixture.eventID {
				t.Fatalf(
					"event ID = %q, want %q",
					eventRecord.ID(),
					successFixture.eventID,
				)
			}

			if eventRecord.SequenceNumber() !=
				workflowTransitionExpectedSequence {
				t.Fatalf(
					"event sequence = %d, want %d",
					eventRecord.SequenceNumber(),
					workflowTransitionExpectedSequence,
				)
			}

			if eventRecord.Type() !=
				repository.ExecutionEventTypeWorkflowFailed {
				t.Fatalf(
					"event type = %q, want WORKFLOW_FAILED",
					eventRecord.Type(),
				)
			}

			logPage, err :=
				store.ListExecutionLogs(
					ctx,
					successFixture.companyID,
					successFixture.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionLogs() returned an error: %v",
					err,
				)
			}

			if len(logPage.Items()) != 1 {
				t.Fatalf(
					"log count = %d, want 1",
					len(logPage.Items()),
				)
			}

			if logPage.Items()[0].
				SequenceNumber() !=
				repository.SequenceNumber(
					workflowTransitionExpectedSequence.
						Int64()+1,
				) {
				t.Fatalf(
					"log sequence = %d, want %d",
					logPage.Items()[0].
						SequenceNumber(),
					workflowTransitionExpectedSequence.
						Int64()+1,
				)
			}

			errorPage, err :=
				store.ListExecutionErrors(
					ctx,
					successFixture.companyID,
					successFixture.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionErrors() returned an error: %v",
					err,
				)
			}

			if len(errorPage.Items()) != 1 {
				t.Fatalf(
					"error count = %d, want 1",
					len(errorPage.Items()),
				)
			}

			errorRecord := errorPage.Items()[0]

			if errorRecord.ID() !=
				successFixture.errorID {
				t.Fatalf(
					"error ID = %q, want %q",
					errorRecord.ID(),
					successFixture.errorID,
				)
			}

			if errorRecord.Category() !=
				runtimefailure.FailureCategoryExecution {
				t.Fatalf(
					"error category = %q, want EXECUTION",
					errorRecord.Category(),
				)
			}

			if errorRecord.SafeMessage() !=
				"Workflow execution failed" {
				t.Fatalf(
					"safe message = %q",
					errorRecord.SafeMessage(),
				)
			}

			technicalDetail, exists :=
				errorRecord.TechnicalDetail()
			if !exists {
				t.Fatal(
					"technical detail was not persisted",
				)
			}

			if technicalDetail !=
				"controlled workflow transition integration failure" {
				t.Fatalf(
					"technical detail = %q",
					technicalDetail,
				)
			}

			relatedEventID, exists :=
				errorRecord.RelatedEventID()
			if !exists ||
				relatedEventID !=
					successFixture.eventID {
				t.Fatalf(
					"related event ID = %q, %t; want %q",
					relatedEventID,
					exists,
					successFixture.eventID,
				)
			}

			if _, hasNode :=
				errorRecord.NodeExecutionID(); hasNode {
				t.Fatal(
					"workflow transition error unexpectedly references a node",
				)
			}

			staleErr :=
				store.ApplyWorkflowTransition(
					ctx,
					successFixture.command,
				)
			if !repository.IsStaleWrite(
				staleErr,
			) {
				t.Fatalf(
					"repeated ApplyWorkflowTransition() error = %v, want STALE_WRITE",
					staleErr,
				)
			}
		},
	)

	t.Run(
		"rolls back workflow timeline and error when error insert fails",
		func(t *testing.T) {
			prepareWorkflowTransitionIntegrationFixture(
				t,
				ctx,
				store,
				rollbackHolder,
			)

			prepareWorkflowTransitionIntegrationFixture(
				t,
				ctx,
				store,
				rollbackTarget,
			)

			if err := store.ApplyWorkflowTransition(
				ctx,
				rollbackHolder.command,
			); err != nil {
				t.Fatalf(
					"apply rollback holder transition: %v",
					err,
				)
			}

			transitionErr :=
				store.ApplyWorkflowTransition(
					ctx,
					rollbackTarget.command,
				)
			if !repository.IsConflict(
				transitionErr,
			) {
				t.Fatalf(
					"ApplyWorkflowTransition() rollback error = %v, want CONFLICT",
					transitionErr,
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

			if actualWorkflow.Status() !=
				execution.WorkflowExecutionStatusRunning {
				t.Fatalf(
					"workflow status after rollback = %q, want RUNNING",
					actualWorkflow.Status(),
				)
			}

			if actualWorkflow.LockVersion() !=
				workflowTransitionExpectedLockVersion {
				t.Fatalf(
					"workflow lock version after rollback = %d, want %d",
					actualWorkflow.LockVersion(),
					workflowTransitionExpectedLockVersion,
				)
			}

			if actualWorkflow.NextSequenceNumber() !=
				workflowTransitionExpectedSequence {
				t.Fatalf(
					"workflow next sequence after rollback = %d, want %d",
					actualWorkflow.NextSequenceNumber(),
					workflowTransitionExpectedSequence,
				)
			}

			if _, exists :=
				actualWorkflow.FinishedAt(); exists {
				t.Fatal(
					"workflow finished time remained after rollback",
				)
			}

			if _, exists :=
				actualWorkflow.FailureSummary(); exists {
				t.Fatal(
					"workflow failure summary remained after rollback",
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

			logPage, err :=
				store.ListExecutionLogs(
					ctx,
					rollbackTarget.companyID,
					rollbackTarget.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionLogs() after rollback: %v",
					err,
				)
			}

			if len(logPage.Items()) != 0 {
				t.Fatalf(
					"log count after rollback = %d, want 0",
					len(logPage.Items()),
				)
			}

			errorPage, err :=
				store.ListExecutionErrors(
					ctx,
					rollbackTarget.companyID,
					rollbackTarget.workflowExecutionID,
					pageRequest,
				)
			if err != nil {
				t.Fatalf(
					"ListExecutionErrors() after rollback: %v",
					err,
				)
			}

			if len(errorPage.Items()) != 0 {
				t.Fatalf(
					"error count after rollback = %d, want 0",
					len(errorPage.Items()),
				)
			}

			assertWorkflowTransitionIntegrationRowCount(
				t,
				ctx,
				store,
				"execution_errors",
				"error_id",
				sharedErrorID.String(),
				1,
			)
		},
	)
}

type workflowTransitionIntegrationFixture struct {
	command repository.WorkflowTransitionCommand

	snapshot repository.DefinitionSnapshot

	currentWorkflow repository.WorkflowExecutionRecord

	companyID workflow.CompanyID

	workflowExecutionID execution.WorkflowExecutionID

	eventID repository.ExecutionEventID
	logID   repository.ExecutionLogID
	errorID repository.ExecutionErrorID

	transitionAt time.Time
}

func newWorkflowTransitionIntegrationFixture(
	t *testing.T,
	suffix string,
	errorIDOverride string,
) workflowTransitionIntegrationFixture {
	t.Helper()

	companyID := workflow.CompanyID(
		"company-workflow-transition-" + suffix,
	)

	workflowID := workflow.WorkflowID(
		"workflow-workflow-transition-" + suffix,
	)

	snapshotID :=
		repository.DefinitionSnapshotID(
			"snapshot-workflow-transition-" +
				suffix,
		)

	workflowExecutionID :=
		execution.WorkflowExecutionID(
			"execution-workflow-transition-" +
				suffix,
		)

	createdAt := time.Date(
		2026,
		time.July,
		17,
		21,
		0,
		0,
		0,
		time.UTC,
	).Add(
		time.Duration(len(suffix)) *
			time.Millisecond,
	)

	validatingAt :=
		createdAt.Add(time.Second)

	startedAt :=
		createdAt.Add(2 * time.Second)

	transitionAt :=
		createdAt.Add(10 * time.Second)

	snapshot, err :=
		repository.NewDefinitionSnapshot(
			snapshotID,
			companyID,
			workflowID,
			8,
			"Workflow Transition Integration",
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

	currentWorkflow, err :=
		repository.NewWorkflowExecutionRecord(
			repository.WorkflowExecutionRecordParams{
				ID:                 workflowExecutionID,
				CompanyID:          companyID,
				WorkflowID:         workflowID,
				WorkflowRevision:   8,
				SnapshotID:         snapshotID,
				Mode:               execution.ExecutionModeSync,
				CorrelationID:      "correlation-" + suffix,
				Status:             execution.WorkflowExecutionStatusRunning,
				CreatedAt:          createdAt,
				ValidatingAt:       validatingAt,
				StartedAt:          startedAt,
				UpdatedAt:          startedAt,
				TerminalOutputs:    []byte(`{}`),
				NextSequenceNumber: workflowTransitionExpectedSequence,
				LockVersion:        workflowTransitionExpectedLockVersion,
			},
		)
	if err != nil {
		t.Fatalf(
			"create current workflow record: %v",
			err,
		)
	}

	updatedWorkflow, err :=
		repository.NewWorkflowExecutionRecord(
			repository.WorkflowExecutionRecordParams{
				ID:               workflowExecutionID,
				CompanyID:        companyID,
				WorkflowID:       workflowID,
				WorkflowRevision: 8,
				SnapshotID:       snapshotID,
				Mode:             execution.ExecutionModeSync,
				CorrelationID:    "correlation-" + suffix,
				Status:           execution.WorkflowExecutionStatusFailed,
				CreatedAt:        createdAt,
				ValidatingAt:     validatingAt,
				StartedAt:        startedAt,
				FinishedAt:       transitionAt,
				UpdatedAt:        transitionAt,
				TerminalOutputs:  []byte(`{}`),
				FailureSummary: []byte(
					`{"code":"WORKFLOW_FAILED"}`,
				),
				NextSequenceNumber: repository.SequenceNumber(
					workflowTransitionExpectedSequence.
						Int64() + 2,
				),
				LockVersion: workflowTransitionExpectedLockVersion + 1,
			},
		)
	if err != nil {
		t.Fatalf(
			"create updated workflow record: %v",
			err,
		)
	}

	eventID := repository.ExecutionEventID(
		"event-workflow-failed-" + suffix,
	)

	eventDraft, err :=
		repository.NewExecutionEventDraft(
			repository.ExecutionEventDraftParams{
				ID:                  eventID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				Type:                repository.ExecutionEventTypeWorkflowFailed,
				PreviousStatus: execution.WorkflowExecutionStatusRunning.
					String(),
				NewStatus: execution.WorkflowExecutionStatusFailed.
					String(),
				CorrelationID: "correlation-" + suffix,
				CausationID:   "workflow-run-" + suffix,
				SafeMessage:   "Workflow execution failed",
				Metadata: []byte(
					`{"source":"integration-test"}`,
				),
				CreatedAt: transitionAt,
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

	logID := repository.ExecutionLogID(
		"log-workflow-failed-" + suffix,
	)

	logDraft, err :=
		repository.NewExecutionLogDraft(
			repository.ExecutionLogDraftParams{
				ID:                  logID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				Level:               repository.ExecutionLogLevelError,
				Message:             "Workflow execution entered FAILED state",
				Metadata: []byte(
					`{"status":"FAILED"}`,
				),
				CreatedAt: transitionAt.Add(time.Second),
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an error: %v",
			err,
		)
	}

	logEntry, err :=
		repository.NewLogTimelineEntry(
			logDraft,
		)
	if err != nil {
		t.Fatalf(
			"NewLogTimelineEntry() returned an error: %v",
			err,
		)
	}

	errorID := repository.ExecutionErrorID(
		"error-workflow-failed-" + suffix,
	)

	if errorIDOverride != "" {
		errorID =
			repository.ExecutionErrorID(
				errorIDOverride,
			)
	}

	errorRecord, err :=
		repository.NewExecutionErrorRecord(
			repository.ExecutionErrorRecordParams{
				ID:                  errorID,
				WorkflowExecutionID: workflowExecutionID,
				CompanyID:           companyID,
				RelatedEventID:      eventID,
				Category:            runtimefailure.FailureCategoryExecution,
				Code:                "WORKFLOW_EXECUTION_FAILED",
				SafeMessage:         "Workflow execution failed",
				TechnicalDetail:     "controlled workflow transition integration failure",
				Retryable:           false,
				Details: []byte(
					`{"component":"workflow-transition"}`,
				),
				CreatedAt: transitionAt.Add(
					2 * time.Second,
				),
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an error: %v",
			err,
		)
	}

	command, err :=
		repository.NewWorkflowTransitionCommand(
			repository.WorkflowTransitionCommandParams{
				CompanyID:                  companyID,
				WorkflowExecutionID:        workflowExecutionID,
				ExpectedStatus:             execution.WorkflowExecutionStatusRunning,
				ExpectedLockVersion:        workflowTransitionExpectedLockVersion,
				ExpectedNextSequenceNumber: workflowTransitionExpectedSequence,
				WorkflowExecution:          updatedWorkflow,
				Timeline: []repository.TimelineEntry{
					eventEntry,
					logEntry,
				},
				Errors: []repository.ExecutionErrorRecord{
					errorRecord,
				},
			},
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowTransitionCommand() returned an error: %v",
			err,
		)
	}

	return workflowTransitionIntegrationFixture{
		command:             command,
		snapshot:            snapshot,
		currentWorkflow:     currentWorkflow,
		companyID:           companyID,
		workflowExecutionID: workflowExecutionID,
		eventID:             eventID,
		logID:               logID,
		errorID:             errorID,
		transitionAt:        transitionAt,
	}
}

func prepareWorkflowTransitionIntegrationFixture(
	t *testing.T,
	ctx context.Context,
	store *Store,
	fixture workflowTransitionIntegrationFixture,
) {
	t.Helper()

	if err := store.Create(
		ctx,
		fixture.snapshot,
	); err != nil {
		t.Fatalf(
			"create workflow transition snapshot: %v",
			err,
		)
	}

	insertWorkflowExecutionReadFixture(
		t,
		ctx,
		store,
		fixture.currentWorkflow,
	)
}

func assertWorkflowTransitionIntegrationRowCount(
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

func TestWorkflowTransitionStoreContractIsImplemented(
	t *testing.T,
) {
	var implementation repository.WorkflowTransitionStore = &Store{}

	if implementation == nil {
		t.Fatal(
			"PostgreSQL store does not implement WorkflowTransitionStore",
		)
	}
}

func TestApplyWorkflowTransitionRejectsInvalidStore(
	t *testing.T,
) {
	var store *Store

	err := store.ApplyWorkflowTransition(
		context.Background(),
		repository.WorkflowTransitionCommand{},
	)
	if err == nil {
		t.Fatal(
			"ApplyWorkflowTransition() returned nil error for nil store",
		)
	}
}

func TestApplyWorkflowTransitionRejectsNilContext(
	t *testing.T,
) {
	store := newUnconnectedWorkflowTransitionStore(t)

	err := store.ApplyWorkflowTransition(
		nil,
		repository.WorkflowTransitionCommand{},
	)
	if err == nil {
		t.Fatal(
			"ApplyWorkflowTransition() accepted nil context",
		)
	}
}

func TestApplyWorkflowTransitionPreservesCancelledContext(
	t *testing.T,
) {
	store := newUnconnectedWorkflowTransitionStore(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err := store.ApplyWorkflowTransition(
		ctx,
		repository.WorkflowTransitionCommand{},
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"ApplyWorkflowTransition() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestApplyWorkflowTransitionRejectsInvalidCommand(
	t *testing.T,
) {
	store := newUnconnectedWorkflowTransitionStore(t)

	err := store.ApplyWorkflowTransition(
		context.Background(),
		repository.WorkflowTransitionCommand{},
	)
	if err == nil {
		t.Fatal(
			"ApplyWorkflowTransition() accepted an invalid command",
		)
	}
}

func newUnconnectedWorkflowTransitionStore(
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
