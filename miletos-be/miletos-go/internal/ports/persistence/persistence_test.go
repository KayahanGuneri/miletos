package repository

import (
	bytes "bytes"
	context "context"
	errors "errors"
	math "math"
	runtime "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	reflect "reflect"
	strings "strings"
	testing "testing"
	time "time"
)

func validContextVariableParams(t *testing.T) AsyncContextVariableParams {
	t.Helper()
	value, err := runtime.NewRuntimeValue([]byte(`{"count":1}`))
	if err != nil {
		t.Fatalf("NewRuntimeValue() error = %v", err)
	}
	return AsyncContextVariableParams{
		CompanyID:           workflow.CompanyID("company-1"),
		WorkflowExecutionID: execution.WorkflowExecutionID("execution-1"),
		Key:                 "counter",
		Value:               value,
		CreatedAt:           asyncTestTime,
		UpdatedAt:           asyncTestTime,
		Version:             1,
	}
}

func TestNewAsyncContextVariableAcceptsValidValue(t *testing.T) {
	variable, err := NewAsyncContextVariable(validContextVariableParams(t))
	if err != nil {
		t.Fatalf("NewAsyncContextVariable() error = %v", err)
	}
	if !variable.IsValid() || variable.IdentityKey() == "" {
		t.Fatal("valid context variable was rejected")
	}
	if variable.Key() != "counter" || variable.Version() != 1 {
		t.Fatalf("key/version = %q/%d", variable.Key(), variable.Version())
	}
}

func TestAsyncContextVariableRejectsInvalidKeyValueAndVersion(t *testing.T) {
	oversizedJSON := append([]byte{'"'}, bytes.Repeat([]byte{'x'}, MaximumAsyncContextValueBytes)...)
	oversizedJSON = append(oversizedJSON, '"')
	oversizedValue, err := runtime.NewRuntimeValue(oversizedJSON)
	if err != nil {
		t.Fatalf("oversized fixture error = %v", err)
	}

	tests := map[string]func(*AsyncContextVariableParams){
		"company":       func(p *AsyncContextVariableParams) { p.CompanyID = "" },
		"execution":     func(p *AsyncContextVariableParams) { p.WorkflowExecutionID = "" },
		"blank key":     func(p *AsyncContextVariableParams) { p.Key = " " },
		"protected key": func(p *AsyncContextVariableParams) { p.Key = runtime.ProtectedKeyCompanyID },
		"long key": func(p *AsyncContextVariableParams) {
			p.Key = strings.Repeat("x", maximumAsyncContextKeyCharacters+1)
		},
		"zero value":      func(p *AsyncContextVariableParams) { p.Value = runtime.RuntimeValue{} },
		"oversized value": func(p *AsyncContextVariableParams) { p.Value = oversizedValue },
		"zero created":    func(p *AsyncContextVariableParams) { p.CreatedAt = time.Time{} },
		"time order":      func(p *AsyncContextVariableParams) { p.UpdatedAt = p.CreatedAt.Add(-time.Second) },
		"version":         func(p *AsyncContextVariableParams) { p.Version = 0 },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params := validContextVariableParams(t)
			mutate(&params)
			if _, err := NewAsyncContextVariable(params); err == nil {
				t.Fatal("NewAsyncContextVariable() returned nil error")
			}
		})
	}
}

func TestAsyncContextWriteDefinesCompareAndSwapPolicy(t *testing.T) {
	insertVariable, err := NewAsyncContextVariable(validContextVariableParams(t))
	if err != nil {
		t.Fatalf("insert variable error = %v", err)
	}
	insert, err := NewAsyncContextWrite(insertVariable, 0)
	if err != nil || !insert.IsInsert() || !insert.IsValid() {
		t.Fatalf("insert write = %#v, error = %v", insert, err)
	}

	updateParams := validContextVariableParams(t)
	updateParams.Version = 4
	updateParams.UpdatedAt = updateParams.CreatedAt.Add(time.Second)
	updateVariable, err := NewAsyncContextVariable(updateParams)
	if err != nil {
		t.Fatalf("update variable error = %v", err)
	}
	update, err := NewAsyncContextWrite(updateVariable, 3)
	if err != nil || update.IsInsert() || update.ExpectedVersion() != 3 {
		t.Fatalf("update write = %#v, error = %v", update, err)
	}

	if _, err := NewAsyncContextWrite(updateVariable, 2); err == nil {
		t.Fatal("non-sequential update version accepted")
	}
	if _, err := NewAsyncContextWrite(insertVariable, -1); err == nil {
		t.Fatal("negative expected version accepted")
	}
}

func TestAsyncContextDeleteRequiresExistingVersion(t *testing.T) {
	deletion, err := NewAsyncContextDelete(
		"company-1",
		"execution-1",
		"counter",
		3,
		asyncTestTime,
	)
	if err != nil || !deletion.IsValid() {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := NewAsyncContextDelete(
		"company-1",
		"execution-1",
		"counter",
		0,
		asyncTestTime,
	); err == nil {
		t.Fatal("delete without expected version accepted")
	}
}

func TestAsyncContextValueIsDefensivelyCopied(t *testing.T) {
	variable, err := NewAsyncContextVariable(validContextVariableParams(t))
	if err != nil {
		t.Fatalf("NewAsyncContextVariable() error = %v", err)
	}
	first := variable.Value().Bytes()
	first[0] = 'X'
	if bytes.Equal(first, variable.Value().Bytes()) {
		t.Fatal("context value getter exposed mutable storage")
	}
}

func TestAsyncContextZeroValuesAreInvalid(t *testing.T) {
	if (AsyncContextVariable{}).IsValid() {
		t.Fatal("zero AsyncContextVariable is valid")
	}
	if (AsyncContextWrite{}).IsValid() {
		t.Fatal("zero AsyncContextWrite is valid")
	}
	if (AsyncContextDelete{}).IsValid() {
		t.Fatal("zero AsyncContextDelete is valid")
	}
}

func validAsyncNodeInputParams(t *testing.T) AsyncNodeInputParams {
	t.Helper()
	return AsyncNodeInputParams{
		CompanyID:             workflow.CompanyID("company-1"),
		WorkflowExecutionID:   execution.WorkflowExecutionID("execution-1"),
		TargetNodeExecutionID: execution.NodeExecutionID("target-execution-1"),
		SourceNodeExecutionID: execution.NodeExecutionID("source-execution-1"),
		TargetNodeID:          workflow.NodeID("target-node"),
		SourceNodeID:          workflow.NodeID("source-node"),
		EdgeID:                workflow.EdgeID("edge-1"),
		SourceOutputPort:      "result",
		TargetInputPort:       "input",
		SourceAttempt:         1,
		Payload:               validInlinePayload(t, []byte(`{"value":1}`)),
		CreatedAt:             asyncTestTime,
	}
}

func TestNewAsyncNodeInputAcceptsValidInput(t *testing.T) {
	input, err := NewAsyncNodeInput(validAsyncNodeInputParams(t))
	if err != nil {
		t.Fatalf("NewAsyncNodeInput() error = %v", err)
	}
	if !input.IsValid() || input.IdentityKey() == "" {
		t.Fatal("valid async input was rejected")
	}
	if input.EdgeID() != "edge-1" || input.SourceAttempt() != 1 {
		t.Fatalf("identity = %q/%d", input.EdgeID(), input.SourceAttempt())
	}
}

func TestAsyncNodeInputIdentityPreventsLogicalDuplicates(t *testing.T) {
	first, err := NewAsyncNodeInput(validAsyncNodeInputParams(t))
	if err != nil {
		t.Fatalf("first input error = %v", err)
	}
	secondParams := validAsyncNodeInputParams(t)
	secondParams.Payload = validInlinePayload(t, []byte(`{"redelivered":true}`))
	second, err := NewAsyncNodeInput(secondParams)
	if err != nil {
		t.Fatalf("second input error = %v", err)
	}
	if first.IdentityKey() != second.IdentityKey() {
		t.Fatal("redelivery changed logical input identity")
	}

	secondParams.EdgeID = "edge-2"
	fanOut, err := NewAsyncNodeInput(secondParams)
	if err != nil {
		t.Fatalf("fan-out input error = %v", err)
	}
	if first.IdentityKey() == fanOut.IdentityKey() {
		t.Fatal("different fan-out edge reused logical input identity")
	}
}

func TestAsyncNodeInputRejectsInvalidIdentityAndPayload(t *testing.T) {
	oversized := validInlinePayload(t, make([]byte, MaximumAsyncEncodedPayloadBytes+1))
	tests := map[string]func(*AsyncNodeInputParams){
		"company":          func(p *AsyncNodeInputParams) { p.CompanyID = "" },
		"execution":        func(p *AsyncNodeInputParams) { p.WorkflowExecutionID = "" },
		"target execution": func(p *AsyncNodeInputParams) { p.TargetNodeExecutionID = "" },
		"source execution": func(p *AsyncNodeInputParams) { p.SourceNodeExecutionID = "" },
		"target node":      func(p *AsyncNodeInputParams) { p.TargetNodeID = "" },
		"source node":      func(p *AsyncNodeInputParams) { p.SourceNodeID = "" },
		"edge":             func(p *AsyncNodeInputParams) { p.EdgeID = "" },
		"source port":      func(p *AsyncNodeInputParams) { p.SourceOutputPort = "" },
		"target port":      func(p *AsyncNodeInputParams) { p.TargetInputPort = "" },
		"attempt":          func(p *AsyncNodeInputParams) { p.SourceAttempt = 0 },
		"payload":          func(p *AsyncNodeInputParams) { p.Payload = runtime.Payload{} },
		"oversized":        func(p *AsyncNodeInputParams) { p.Payload = oversized },
		"created":          func(p *AsyncNodeInputParams) { p.CreatedAt = time.Time{} },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params := validAsyncNodeInputParams(t)
			mutate(&params)
			if _, err := NewAsyncNodeInput(params); err == nil {
				t.Fatal("NewAsyncNodeInput() returned nil error")
			}
		})
	}
}

func TestAsyncNodeInputDefensivelyCopiesPayload(t *testing.T) {
	data := []byte(`{"value":1}`)
	params := validAsyncNodeInputParams(t)
	params.Payload = validInlinePayload(t, data)
	input, err := NewAsyncNodeInput(params)
	if err != nil {
		t.Fatalf("NewAsyncNodeInput() error = %v", err)
	}
	data[0] = 'X'
	first, _ := input.Payload().InlineData()
	first[0] = 'Y'
	second, _ := input.Payload().InlineData()
	if bytes.Equal(first, second) {
		t.Fatal("payload getter exposed mutable storage")
	}
}

func validWorkerResultParams(t *testing.T) DurableWorkerResultParams {
	t.Helper()
	changes, err := runtime.NewContextChanges(nil, nil)
	if err != nil {
		t.Fatalf("NewContextChanges() error = %v", err)
	}
	result, err := runtime.NewNodeSuccessResult(
		map[string][]runtime.Payload{
			"result": {validInlinePayload(t, []byte(`{"ok":true}`))},
		},
		changes,
	)
	if err != nil {
		t.Fatalf("NewNodeSuccessResult() error = %v", err)
	}
	return DurableWorkerResultParams{
		CompanyID:           "company-1",
		WorkflowExecutionID: "execution-1",
		NodeExecutionID:     "node-execution-1",
		NodeID:              "node-1",
		Attempt:             1,
		Status:              DurableWorkerResultSucceeded,
		Result:              result,
		StartedAt:           asyncTestTime,
		FinishedAt:          asyncTestTime.Add(time.Second),
		CreatedAt:           asyncTestTime.Add(2 * time.Second),
	}
}

func TestDurableWorkerResultAcceptsRuntimeResultForms(t *testing.T) {
	success, err := NewDurableWorkerResult(validWorkerResultParams(t))
	if err != nil || !success.IsValid() {
		t.Fatalf("success error = %v", err)
	}
	if result, exists := success.Result(); !exists || !result.IsSuccess() {
		t.Fatal("success result is unavailable")
	}

	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryExecution,
		"FAILED",
		"safe failure",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("failure fixture error = %v", err)
	}
	failedResult, err := runtime.NewNodeFailureResult(failure)
	if err != nil {
		t.Fatalf("failed result fixture error = %v", err)
	}
	failedParams := validWorkerResultParams(t)
	failedParams.Status = DurableWorkerResultFailed
	failedParams.Result = failedResult
	failed, err := NewDurableWorkerResult(failedParams)
	if err != nil || !failed.IsValid() {
		t.Fatalf("failed result error = %v", err)
	}

	for status, category := range map[DurableWorkerResultStatus]runtime.FailureCategory{
		DurableWorkerResultCancelled: runtime.FailureCategoryCanceled,
		DurableWorkerResultTimedOut:  runtime.FailureCategoryTimeout,
	} {
		params := validWorkerResultParams(t)
		params.Status = status
		params.Result = runtime.NodeResult{}
		params.Failure, _ = runtime.NewRuntimeFailure(category, "INTERRUPTED", "interrupted", false, nil)
		stored, err := NewDurableWorkerResult(params)
		if err != nil || !stored.IsValid() {
			t.Fatalf("%s error = %v", status, err)
		}
		if _, exists := stored.Failure(); !exists {
			t.Fatalf("%s failure unavailable", status)
		}
	}
}

func TestDurableWorkerResultPreservesTerminalOutputAndContextChanges(t *testing.T) {
	value, err := runtime.NewRuntimeValue([]byte(`{"step":2}`))
	if err != nil {
		t.Fatalf("context value error = %v", err)
	}
	changes, err := runtime.NewContextChanges(
		map[string]runtime.RuntimeValue{"progress": value},
		nil,
	)
	if err != nil {
		t.Fatalf("context changes error = %v", err)
	}
	terminal := validInlinePayload(t, []byte(`{"complete":true}`))
	runtimeResult, err := runtime.NewTerminalNodeSuccessResult(terminal, changes)
	if err != nil {
		t.Fatalf("terminal result error = %v", err)
	}

	params := validWorkerResultParams(t)
	params.Result = runtimeResult
	stored, err := NewDurableWorkerResult(params)
	if err != nil {
		t.Fatalf("NewDurableWorkerResult() error = %v", err)
	}
	result, exists := stored.Result()
	if !exists || !result.HasTerminalOutput() || result.HasRoutedOutputs() {
		t.Fatal("terminal result shape was not preserved")
	}
	storedTerminal, exists := result.TerminalOutput()
	if !exists {
		t.Fatal("terminal output is unavailable")
	}
	data, inline := storedTerminal.InlineData()
	if !inline || !bytes.Equal(data, []byte(`{"complete":true}`)) {
		t.Fatalf("terminal payload = %q, inline = %v", data, inline)
	}
	storedValue, exists, err := result.ContextChanges().SetValue("progress")
	if err != nil || !exists || !bytes.Equal(storedValue.Bytes(), value.Bytes()) {
		t.Fatalf("context value = %q, exists = %v, error = %v", storedValue.Bytes(), exists, err)
	}
}

func TestDurableWorkerResultRejectsMismatchedAndInvalidState(t *testing.T) {
	tests := map[string]func(*DurableWorkerResultParams){
		"company":        func(p *DurableWorkerResultParams) { p.CompanyID = "" },
		"execution":      func(p *DurableWorkerResultParams) { p.WorkflowExecutionID = "" },
		"node":           func(p *DurableWorkerResultParams) { p.NodeExecutionID = "" },
		"attempt":        func(p *DurableWorkerResultParams) { p.Attempt = 0 },
		"status":         func(p *DurableWorkerResultParams) { p.Status = "UNKNOWN" },
		"missing result": func(p *DurableWorkerResultParams) { p.Result = runtime.NodeResult{} },
		"time order":     func(p *DurableWorkerResultParams) { p.FinishedAt = p.StartedAt.Add(-time.Second) },
		"creation order": func(p *DurableWorkerResultParams) { p.CreatedAt = p.FinishedAt.Add(-time.Second) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params := validWorkerResultParams(t)
			mutate(&params)
			if _, err := NewDurableWorkerResult(params); err == nil {
				t.Fatal("NewDurableWorkerResult() returned nil error")
			}
		})
	}

	params := validWorkerResultParams(t)
	params.Status = DurableWorkerResultCancelled
	params.Result = runtime.NodeResult{}
	params.Failure, _ = runtime.NewRuntimeFailure(
		runtime.FailureCategoryTimeout,
		"WRONG",
		"wrong category",
		false,
		nil,
	)
	if _, err := NewDurableWorkerResult(params); err == nil {
		t.Fatal("mismatched interruption failure accepted")
	}
}

func TestDurableWorkerResultIdentityIsDeterministic(t *testing.T) {
	first, err := NewDurableWorkerResult(validWorkerResultParams(t))
	if err != nil {
		t.Fatalf("first result error = %v", err)
	}
	secondParams := validWorkerResultParams(t)
	second, err := NewDurableWorkerResult(secondParams)
	if err != nil {
		t.Fatalf("second result error = %v", err)
	}
	if first.IdentityKey() != second.IdentityKey() {
		t.Fatal("same node attempt produced different worker-result identity")
	}
	secondParams.Attempt = 2
	third, err := NewDurableWorkerResult(secondParams)
	if err != nil {
		t.Fatalf("third result error = %v", err)
	}
	if first.IdentityKey() == third.IdentityKey() {
		t.Fatal("different node attempts produced the same worker-result identity")
	}
}

func TestAsyncPayloadZeroValuesAreInvalid(t *testing.T) {
	if (AsyncNodeInput{}).IsValid() {
		t.Fatal("zero AsyncNodeInput is valid")
	}
	if (DurableWorkerResult{}).IsValid() {
		t.Fatal("zero DurableWorkerResult is valid")
	}
	if (DurableWorkerResultStatus("UNKNOWN")).IsValid() {
		t.Fatal("unsupported worker result status is valid")
	}
}

type fakeAsyncPersistenceTransaction struct{}

func (fakeAsyncPersistenceTransaction) CreateOutboxMessage(context.Context, OutboxMessage) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) ClaimPublishableOutboxMessages(
	context.Context,
	OutboxClaimRequest,
) ([]OutboxMessage, error) {
	return nil, nil
}
func (fakeAsyncPersistenceTransaction) MarkOutboxMessagePublished(
	context.Context,
	MarkOutboxPublishedCommand,
) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) ReleaseStaleOutboxClaims(
	context.Context,
	ReleaseStaleOutboxClaimsRequest,
) (int64, error) {
	return 0, nil
}
func (fakeAsyncPersistenceTransaction) HasProcessedInboxMessage(
	context.Context,
	ConsumerIdentity,
	MessageID,
) (bool, error) {
	return false, nil
}
func (fakeAsyncPersistenceTransaction) RecordInboxMessage(context.Context, InboxMessage) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) CreateAsyncNodeInput(context.Context, AsyncNodeInput) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) ListAsyncNodeInputs(
	context.Context,
	workflow.CompanyID,
	execution.WorkflowExecutionID,
	execution.NodeExecutionID,
) ([]AsyncNodeInput, error) {
	return nil, nil
}
func (fakeAsyncPersistenceTransaction) CreateDurableWorkerResult(
	context.Context,
	DurableWorkerResult,
) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) GetDurableWorkerResult(
	context.Context,
	workflow.CompanyID,
	execution.WorkflowExecutionID,
	execution.NodeExecutionID,
	int16,
) (DurableWorkerResult, error) {
	return DurableWorkerResult{}, nil
}
func (fakeAsyncPersistenceTransaction) GetAsyncContextVariable(
	context.Context,
	workflow.CompanyID,
	execution.WorkflowExecutionID,
	string,
) (AsyncContextVariable, error) {
	return AsyncContextVariable{}, nil
}
func (fakeAsyncPersistenceTransaction) ListAsyncContextVariables(
	context.Context,
	workflow.CompanyID,
	execution.WorkflowExecutionID,
) ([]AsyncContextVariable, error) {
	return nil, nil
}
func (fakeAsyncPersistenceTransaction) CompareAndSwapAsyncContextVariable(
	context.Context,
	AsyncContextWrite,
) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) DeleteAsyncContextVariable(
	context.Context,
	AsyncContextDelete,
) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) ScheduleAsyncNodeExecution(context.Context, AsyncNodeSchedule) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) ClaimAsyncNodeExecution(context.Context, AsyncNodeClaim) (int64, error) {
	return 1, nil
}
func (fakeAsyncPersistenceTransaction) CompleteAsyncNodeExecution(context.Context, AsyncNodeCompletion) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) FinalizeAsyncNodeExecution(context.Context, AsyncNodeFinalization) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) LockAsyncNodeCoordination(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, workflow.NodeID) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) LockAsyncWorkflowCoordination(context.Context, workflow.CompanyID, execution.WorkflowExecutionID) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) GetAsyncNodeExecution(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, execution.NodeExecutionID) (NodeExecutionRecord, error) {
	return NodeExecutionRecord{}, nil
}
func (fakeAsyncPersistenceTransaction) GetAsyncWorkflowExecution(context.Context, workflow.CompanyID, execution.WorkflowExecutionID) (WorkflowExecutionRecord, error) {
	return WorkflowExecutionRecord{}, nil
}
func (fakeAsyncPersistenceTransaction) ListAsyncNodeExecutions(context.Context, workflow.CompanyID, execution.WorkflowExecutionID) ([]NodeExecutionRecord, error) {
	return nil, nil
}
func (fakeAsyncPersistenceTransaction) SkipAsyncNodeExecution(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, execution.NodeExecutionID, int64, time.Time) error {
	return nil
}
func (fakeAsyncPersistenceTransaction) CompleteAsyncWorkflow(context.Context, AsyncWorkflowCompletion) error {
	return nil
}

var _ AsyncPersistenceTransaction = fakeAsyncPersistenceTransaction{}

type fakeAsyncTransactor struct {
	transaction AsyncPersistenceTransaction
}

func (transactor fakeAsyncTransactor) WithinAsyncPersistenceTransaction(
	ctx context.Context,
	work AsyncPersistenceWork,
) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if work == nil {
		return errors.New("nil work")
	}
	return work(ctx, transactor.transaction)
}

var _ AsyncPersistenceTransactor = fakeAsyncTransactor{}

func TestAsyncPersistenceContractsRemainSmallAndComposable(t *testing.T) {
	transaction := fakeAsyncPersistenceTransaction{}
	var outbox OutboxStore = transaction
	var publication OutboxPublicationStore = transaction
	var inbox InboxStore = transaction
	var input AsyncInputStore = transaction
	var result DurableWorkerResultStore = transaction
	var contextStore AsyncContextStore = transaction

	if outbox == nil || publication == nil || inbox == nil || input == nil || result == nil || contextStore == nil {
		t.Fatal("focused contracts are unavailable")
	}
}

func TestOutboxPublicationContractHasNoDriverOrTransportTypes(t *testing.T) {
	contract := reflect.TypeOf((*OutboxPublicationStore)(nil)).Elem()
	if contract.NumMethod() != 3 {
		t.Fatalf("method count = %d, want 3", contract.NumMethod())
	}

	for methodIndex := 0; methodIndex < contract.NumMethod(); methodIndex++ {
		method := contract.Method(methodIndex)
		forbidden := []string{"pgx", "kafka", "franz"}
		for _, fragment := range forbidden {
			if strings.Contains(strings.ToLower(method.Type.String()), fragment) {
				t.Fatalf("method %s leaked transport or driver type %q", method.Name, method.Type)
			}
		}
	}
}

func TestAsyncPersistenceTransactorPropagatesWorkError(t *testing.T) {
	expected := errors.New("rollback requested")
	transactor := fakeAsyncTransactor{transaction: fakeAsyncPersistenceTransaction{}}
	actual := transactor.WithinAsyncPersistenceTransaction(
		context.Background(),
		func(ctx context.Context, transaction AsyncPersistenceTransaction) error {
			if ctx == nil || transaction == nil {
				t.Fatal("transaction work received nil dependency")
			}
			return expected
		},
	)
	if !errors.Is(actual, expected) {
		t.Fatalf("error = %v, want %v", actual, expected)
	}
}

func TestNormalizeAsyncExecutionScopeRejectsMissingTenantIdentity(t *testing.T) {
	if _, _, err := normalizeAsyncExecutionScope("", "execution-1"); err == nil {
		t.Fatal("empty company accepted")
	}
	if _, _, err := normalizeAsyncExecutionScope("company-1", ""); err == nil {
		t.Fatal("empty workflow execution accepted")
	}
}

type executionCreationStoreContractStub struct{}

var _ ExecutionCreationStore = (*executionCreationStoreContractStub)(nil)

func (
	*executionCreationStoreContractStub,
) CreateExecution(
	context.Context,
	CreateExecutionCommand,
) error {
	return nil
}

func (
	*executionCreationStoreContractStub,
) CreateNodeExecutions(
	context.Context,
	CreateNodeExecutionsCommand,
) error {
	return nil
}

func TestNewCreateExecutionCommandStoresCreatedExecution(
	t *testing.T,
) {
	params := validCreateExecutionCommandParams(t)

	command, err := NewCreateExecutionCommand(params)
	if err != nil {
		t.Fatalf(
			"NewCreateExecutionCommand() returned an unexpected error: %v",
			err,
		)
	}

	if command.Snapshot().ID().String() !=
		"snapshot-1" {
		t.Fatalf(
			"snapshot ID = %q",
			command.Snapshot().ID(),
		)
	}

	if command.WorkflowExecution().Status() !=
		execution.WorkflowExecutionStatusCreated {
		t.Fatalf(
			"workflow status = %q, want CREATED",
			command.WorkflowExecution().Status(),
		)
	}

	if actual := len(command.Timeline()); actual != 2 {
		t.Fatalf(
			"timeline count = %d, want 2",
			actual,
		)
	}

	if !command.IsValid() {
		t.Fatal(
			"valid CreateExecutionCommand was reported as invalid",
		)
	}
}

func TestNewCreateExecutionCommandRejectsInvalidCoreState(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(
			*CreateExecutionCommandParams,
			*testing.T,
		)
	}{
		{
			name:  "invalid snapshot",
			field: "snapshot",
			mutate: func(
				params *CreateExecutionCommandParams,
				_ *testing.T,
			) {
				params.Snapshot =
					DefinitionSnapshot{}
			},
		},
		{
			name:  "invalid workflow execution",
			field: "workflowExecution",
			mutate: func(
				params *CreateExecutionCommandParams,
				_ *testing.T,
			) {
				params.WorkflowExecution =
					WorkflowExecutionRecord{}
			},
		},
		{
			name:  "workflow is not created",
			field: "workflowExecution",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.WorkflowExecution =
					newCreatedWorkflowRecord(
						t,
						func(
							recordParams *WorkflowExecutionRecordParams,
						) {
							recordParams.Status =
								execution.WorkflowExecutionStatusValidating

							recordParams.ValidatingAt =
								recordParams.CreatedAt.Add(
									time.Minute,
								)

							recordParams.UpdatedAt =
								recordParams.ValidatingAt
						},
					)
			},
		},
		{
			name:  "nonzero lock version",
			field: "workflowExecution",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.WorkflowExecution =
					newCreatedWorkflowRecord(
						t,
						func(
							recordParams *WorkflowExecutionRecordParams,
						) {
							recordParams.LockVersion = 1
						},
					)
			},
		},
		{
			name:  "sequence does not begin at one",
			field: "workflowExecution",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.WorkflowExecution =
					newCreatedWorkflowRecord(
						t,
						func(
							recordParams *WorkflowExecutionRecordParams,
						) {
							recordParams.NextSequenceNumber = 2
						},
					)
			},
		},
		{
			name:  "snapshot company mismatch",
			field: "snapshot",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Snapshot =
					newCreationSnapshot(
						t,
						workflow.CompanyID("company-2"),
						workflow.WorkflowID("workflow-1"),
						3,
						DefinitionSnapshotID("snapshot-1"),
						params.WorkflowExecution.
							CreatedAt().
							Add(-time.Minute),
					)
			},
		},
		{
			name:  "snapshot workflow mismatch",
			field: "snapshot",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Snapshot =
					newCreationSnapshot(
						t,
						workflow.CompanyID("company-1"),
						workflow.WorkflowID("workflow-2"),
						3,
						DefinitionSnapshotID("snapshot-1"),
						params.WorkflowExecution.
							CreatedAt().
							Add(-time.Minute),
					)
			},
		},
		{
			name:  "snapshot revision mismatch",
			field: "snapshot",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Snapshot =
					newCreationSnapshot(
						t,
						workflow.CompanyID("company-1"),
						workflow.WorkflowID("workflow-1"),
						4,
						DefinitionSnapshotID("snapshot-1"),
						params.WorkflowExecution.
							CreatedAt().
							Add(-time.Minute),
					)
			},
		},
		{
			name:  "snapshot created after execution",
			field: "snapshot",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Snapshot =
					newCreationSnapshot(
						t,
						workflow.CompanyID("company-1"),
						workflow.WorkflowID("workflow-1"),
						3,
						DefinitionSnapshotID("snapshot-1"),
						params.WorkflowExecution.
							CreatedAt().
							Add(time.Minute),
					)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validCreateExecutionCommandParams(t)

			test.mutate(&params, t)

			_, err := NewCreateExecutionCommand(params)
			if err == nil {
				t.Fatal(
					"NewCreateExecutionCommand() returned nil error",
				)
			}

			requireCreateExecutionValidationField(
				t,
				err,
				test.field,
			)
		})
	}
}

func TestNewCreateExecutionCommandRejectsInvalidTimeline(
	t *testing.T,
) {
	tests := []struct {
		name   string
		mutate func(
			*CreateExecutionCommandParams,
			*testing.T,
		)
	}{
		{
			name: "empty timeline",
			mutate: func(
				params *CreateExecutionCommandParams,
				_ *testing.T,
			) {
				params.Timeline = nil
			},
		},
		{
			name: "first entry is a log",
			mutate: func(
				params *CreateExecutionCommandParams,
				_ *testing.T,
			) {
				params.Timeline[0] =
					params.Timeline[1]
			},
		},
		{
			name: "wrong first event type",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Timeline[0] =
					newCreationWorkflowEventEntry(
						t,
						params.WorkflowExecution,
						ExecutionEventTypeWorkflowValidating,
						execution.WorkflowExecutionStatusCreated.String(),
						execution.WorkflowExecutionStatusValidating.String(),
						params.WorkflowExecution.CreatedAt(),
					)
			},
		},
		{
			name: "created event has previous status",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Timeline[0] =
					newCreationWorkflowEventEntry(
						t,
						params.WorkflowExecution,
						ExecutionEventTypeWorkflowCreated,
						execution.WorkflowExecutionStatusCreated.String(),
						execution.WorkflowExecutionStatusCreated.String(),
						params.WorkflowExecution.CreatedAt(),
					)
			},
		},
		{
			name: "created event has wrong new status",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Timeline[0] =
					newCreationWorkflowEventEntry(
						t,
						params.WorkflowExecution,
						ExecutionEventTypeWorkflowCreated,
						"",
						execution.WorkflowExecutionStatusValidating.String(),
						params.WorkflowExecution.CreatedAt(),
					)
			},
		},
		{
			name: "created event timestamp mismatch",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Timeline[0] =
					newCreationWorkflowEventEntry(
						t,
						params.WorkflowExecution,
						ExecutionEventTypeWorkflowCreated,
						"",
						execution.WorkflowExecutionStatusCreated.String(),
						params.WorkflowExecution.
							CreatedAt().
							Add(time.Second),
					)
			},
		},
		{
			name: "additional event after created event",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Timeline = append(
					params.Timeline,
					newCreationWorkflowEventEntry(
						t,
						params.WorkflowExecution,
						ExecutionEventTypeWorkflowValidating,
						execution.WorkflowExecutionStatusCreated.String(),
						execution.WorkflowExecutionStatusValidating.String(),
						params.WorkflowExecution.
							CreatedAt().
							Add(time.Second),
					),
				)
			},
		},
		{
			name: "node scoped creation log",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Timeline[1] =
					newCreationWorkflowLogEntry(
						t,
						params.WorkflowExecution,
						ExecutionLogID("log-node"),
						execution.NodeExecutionID(
							"node-execution-1",
						),
						params.WorkflowExecution.CreatedAt(),
					)
			},
		},
		{
			name: "duplicate log ID",
			mutate: func(
				params *CreateExecutionCommandParams,
				_ *testing.T,
			) {
				params.Timeline = append(
					params.Timeline,
					params.Timeline[1],
				)
			},
		},
		{
			name: "log before workflow creation",
			mutate: func(
				params *CreateExecutionCommandParams,
				t *testing.T,
			) {
				params.Timeline[1] =
					newCreationWorkflowLogEntry(
						t,
						params.WorkflowExecution,
						ExecutionLogID("log-before"),
						"",
						params.WorkflowExecution.
							CreatedAt().
							Add(-time.Second),
					)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validCreateExecutionCommandParams(t)

			test.mutate(&params, t)

			_, err := NewCreateExecutionCommand(params)
			if err == nil {
				t.Fatal(
					"NewCreateExecutionCommand() returned nil error",
				)
			}

			requireCreateExecutionValidationField(
				t,
				err,
				"timeline",
			)
		})
	}
}

func TestCreateExecutionCommandReturnsDefensiveTimelineCopy(
	t *testing.T,
) {
	command, err := NewCreateExecutionCommand(
		validCreateExecutionCommandParams(t),
	)
	if err != nil {
		t.Fatalf(
			"NewCreateExecutionCommand() returned an unexpected error: %v",
			err,
		)
	}

	timeline := command.Timeline()
	timeline[0] = TimelineEntry{}

	if !command.Timeline()[0].IsValid() {
		t.Fatal(
			"Timeline() allowed command mutation",
		)
	}
}

func TestCreateExecutionCommandZeroValueIsInvalid(
	t *testing.T,
) {
	var command CreateExecutionCommand

	if command.IsValid() {
		t.Fatal(
			"zero-value CreateExecutionCommand was reported as valid",
		)
	}
}

func validCreateExecutionCommandParams(
	t *testing.T,
) CreateExecutionCommandParams {
	t.Helper()

	workflowExecution :=
		newCreatedWorkflowRecord(t, nil)

	return CreateExecutionCommandParams{
		Snapshot: newCreationSnapshot(
			t,
			workflowExecution.CompanyID(),
			workflowExecution.WorkflowID(),
			workflowExecution.WorkflowRevision(),
			workflowExecution.SnapshotID(),
			workflowExecution.CreatedAt().
				Add(-time.Minute),
		),
		WorkflowExecution: workflowExecution,
		Timeline: []TimelineEntry{
			newCreationWorkflowEventEntry(
				t,
				workflowExecution,
				ExecutionEventTypeWorkflowCreated,
				"",
				execution.WorkflowExecutionStatusCreated.String(),
				workflowExecution.CreatedAt(),
			),
			newCreationWorkflowLogEntry(
				t,
				workflowExecution,
				ExecutionLogID(
					"log-workflow-created",
				),
				"",
				workflowExecution.CreatedAt(),
			),
		},
	}
}

func newCreatedWorkflowRecord(
	t *testing.T,
	mutate func(
		*WorkflowExecutionRecordParams,
	),
) WorkflowExecutionRecord {
	t.Helper()

	params := validWorkflowExecutionRecordParams()

	params.Status =
		execution.WorkflowExecutionStatusCreated

	params.ValidatingAt = time.Time{}
	params.QueuedAt = time.Time{}
	params.StartedAt = time.Time{}
	params.FinishedAt = time.Time{}
	params.UpdatedAt = params.CreatedAt

	params.TerminalOutputs = nil
	params.FailureSummary = nil
	params.IsStalled = false
	params.NextSequenceNumber = 1
	params.LockVersion = 0

	if mutate != nil {
		mutate(&params)
	}

	record, err := NewWorkflowExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func newCreationSnapshot(
	t *testing.T,
	companyID workflow.CompanyID,
	workflowID workflow.WorkflowID,
	revision uint64,
	snapshotID DefinitionSnapshotID,
	createdAt time.Time,
) DefinitionSnapshot {
	t.Helper()

	snapshot, err := NewDefinitionSnapshot(
		snapshotID,
		companyID,
		workflowID,
		revision,
		"Initial Workflow",
		[]byte(
			`{"nodes":[],"edges":[]}`,
		),
		createdAt,
	)
	if err != nil {
		t.Fatalf(
			"NewDefinitionSnapshot() returned an unexpected error: %v",
			err,
		)
	}

	return snapshot
}

func newCreationWorkflowEventEntry(
	t *testing.T,
	record WorkflowExecutionRecord,
	eventType ExecutionEventType,
	previousStatus string,
	newStatus string,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	eventDraft, err := NewExecutionEventDraft(
		ExecutionEventDraftParams{
			ID: ExecutionEventID(
				"event-" +
					eventType.String(),
			),
			WorkflowExecutionID: record.ID(),
			CompanyID:           record.CompanyID(),
			Type:                eventType,
			PreviousStatus:      previousStatus,
			NewStatus:           newStatus,
			SafeMessage:         "Workflow execution created",
			CreatedAt:           createdAt,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventDraft() returned an unexpected error: %v",
			err,
		)
	}

	entry, err := NewEventTimelineEntry(
		eventDraft,
	)
	if err != nil {
		t.Fatalf(
			"NewEventTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	return entry
}

func newCreationWorkflowLogEntry(
	t *testing.T,
	record WorkflowExecutionRecord,
	logID ExecutionLogID,
	nodeExecutionID execution.NodeExecutionID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	logDraft, err := NewExecutionLogDraft(
		ExecutionLogDraftParams{
			ID:                  logID,
			WorkflowExecutionID: record.ID(),
			CompanyID:           record.CompanyID(),
			NodeExecutionID:     nodeExecutionID,
			Level:               ExecutionLogLevelInfo,
			Message:             "Workflow execution created",
			CreatedAt:           createdAt,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an unexpected error: %v",
			err,
		)
	}

	entry, err := NewLogTimelineEntry(
		logDraft,
	)
	if err != nil {
		t.Fatalf(
			"NewLogTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	return entry
}

func requireCreateExecutionValidationField(
	t *testing.T,
	err error,
	expectedField string,
) {
	t.Helper()

	var validationError *ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field !=
		expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}
}

func TestNewCreateNodeExecutionsCommandStoresInitialNodes(
	t *testing.T,
) {
	params :=
		validCreateNodeExecutionsCommandParams(t)

	command, err :=
		NewCreateNodeExecutionsCommand(params)
	if err != nil {
		t.Fatalf(
			"NewCreateNodeExecutionsCommand() returned an unexpected error: %v",
			err,
		)
	}

	if command.CompanyID().String() !=
		"company-1" {
		t.Fatalf(
			"company ID = %q",
			command.CompanyID(),
		)
	}

	if command.WorkflowExecutionID().String() !=
		"execution-1" {
		t.Fatalf(
			"workflow execution ID = %q",
			command.WorkflowExecutionID(),
		)
	}

	if command.ExpectedWorkflowStatus() !=
		execution.WorkflowExecutionStatusValidating {
		t.Fatalf(
			"expected workflow status = %q",
			command.ExpectedWorkflowStatus(),
		)
	}

	if actual := len(command.NodeExecutions()); actual != 2 {
		t.Fatalf(
			"node count = %d, want 2",
			actual,
		)
	}

	if actual := len(command.Timeline()); actual != 2 {
		t.Fatalf(
			"timeline count = %d, want 2",
			actual,
		)
	}

	if !command.IsValid() {
		t.Fatal(
			"valid CreateNodeExecutionsCommand was reported as invalid",
		)
	}
}

func TestNewCreateNodeExecutionsCommandRejectsInvalidCoreState(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(
			*CreateNodeExecutionsCommandParams,
			*testing.T,
		)
	}{
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.CompanyID =
					workflow.CompanyID(" ")
			},
		},
		{
			name:  "blank workflow execution ID",
			field: "workflowExecutionID",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.WorkflowExecutionID =
					execution.WorkflowExecutionID(" ")
			},
		},
		{
			name:  "workflow status is not validating",
			field: "expectedWorkflowStatus",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.ExpectedWorkflowStatus =
					execution.WorkflowExecutionStatusRunning
			},
		},
		{
			name:  "negative workflow lock version",
			field: "expectedWorkflowLockVersion",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.ExpectedWorkflowLockVersion = -1
			},
		},
		{
			name:  "invalid expected sequence",
			field: "expectedNextSequenceNumber",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.ExpectedNextSequenceNumber = 0
			},
		},
		{
			name:  "empty node set",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.NodeExecutions = nil
				params.Timeline = nil
			},
		},
		{
			name:  "duplicate node execution ID",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.NodeExecutions[1] =
					params.NodeExecutions[0]
			},
		},
		{
			name:  "duplicate node ID",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.NodeExecutions[1] =
					newPendingNodeCreationRecord(
						t,
						"node-execution-2",
						"node-1",
						nil,
					)
			},
		},
		{
			name:  "node company mismatch",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.NodeExecutions[0] =
					newPendingNodeCreationRecord(
						t,
						"node-execution-1",
						"node-1",
						func(
							recordParams *NodeExecutionRecordParams,
						) {
							recordParams.CompanyID =
								workflow.CompanyID(
									"company-2",
								)
						},
					)
			},
		},
		{
			name:  "node workflow execution mismatch",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.NodeExecutions[0] =
					newPendingNodeCreationRecord(
						t,
						"node-execution-1",
						"node-1",
						func(
							recordParams *NodeExecutionRecordParams,
						) {
							recordParams.WorkflowExecutionID =
								execution.WorkflowExecutionID(
									"execution-2",
								)
						},
					)
			},
		},
		{
			name:  "node is not pending",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.NodeExecutions[0] =
					newPendingNodeCreationRecord(
						t,
						"node-execution-1",
						"node-1",
						func(
							recordParams *NodeExecutionRecordParams,
						) {
							recordParams.Status =
								execution.NodeExecutionStatusReady

							recordParams.ReadyAt =
								recordParams.CreatedAt.Add(
									time.Minute,
								)

							recordParams.UpdatedAt =
								recordParams.ReadyAt
						},
					)
			},
		},
		{
			name:  "initial node attempt is not one",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.NodeExecutions[0] =
					newPendingNodeCreationRecord(
						t,
						"node-execution-1",
						"node-1",
						func(
							recordParams *NodeExecutionRecordParams,
						) {
							recordParams.Attempt = 2
						},
					)
			},
		},
		{
			name:  "initial node lock version is nonzero",
			field: "nodeExecutions",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.NodeExecutions[0] =
					newPendingNodeCreationRecord(
						t,
						"node-execution-1",
						"node-1",
						func(
							recordParams *NodeExecutionRecordParams,
						) {
							recordParams.LockVersion = 1
						},
					)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validCreateNodeExecutionsCommandParams(t)

			test.mutate(&params, t)

			_, err :=
				NewCreateNodeExecutionsCommand(params)
			if err == nil {
				t.Fatal(
					"NewCreateNodeExecutionsCommand() returned nil error",
				)
			}

			requireCreateNodeExecutionsValidationField(
				t,
				err,
				test.field,
			)
		})
	}
}

func TestNewCreateNodeExecutionsCommandRejectsInvalidTimeline(
	t *testing.T,
) {
	tests := []struct {
		name   string
		mutate func(
			*CreateNodeExecutionsCommandParams,
			*testing.T,
		)
	}{
		{
			name: "timeline count mismatch",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.Timeline =
					params.Timeline[:1]
			},
		},
		{
			name: "timeline contains log",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				logDraft, err :=
					NewExecutionLogDraft(
						ExecutionLogDraftParams{
							ID: ExecutionLogID(
								"log-node-created",
							),
							WorkflowExecutionID: params.WorkflowExecutionID,
							CompanyID:           params.CompanyID,
							NodeExecutionID:     params.NodeExecutions[0].ID(),
							Level:               ExecutionLogLevelInfo,
							Message:             "Node execution created",
							CreatedAt:           params.NodeExecutions[0].CreatedAt(),
						},
					)
				if err != nil {
					t.Fatalf(
						"NewExecutionLogDraft() error: %v",
						err,
					)
				}

				entry, err :=
					NewLogTimelineEntry(logDraft)
				if err != nil {
					t.Fatalf(
						"NewLogTimelineEntry() error: %v",
						err,
					)
				}

				params.Timeline[0] = entry
			},
		},
		{
			name: "wrong event type",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.Timeline[0] =
					newNodeCreationEventEntry(
						t,
						params.NodeExecutions[0],
						ExecutionEventTypeNodeReady,
						params.NodeExecutions[0].ID(),
						params.NodeExecutions[0].CreatedAt(),
					)
			},
		},
		{
			name: "event order does not match nodes",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				_ *testing.T,
			) {
				params.Timeline[0],
					params.Timeline[1] =
					params.Timeline[1],
					params.Timeline[0]
			},
		},
		{
			name: "event timestamp mismatch",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.Timeline[0] =
					newNodeCreationEventEntry(
						t,
						params.NodeExecutions[0],
						ExecutionEventTypeNodeCreated,
						params.NodeExecutions[0].ID(),
						params.NodeExecutions[0].
							CreatedAt().
							Add(time.Second),
					)
			},
		},
		{
			name: "duplicate event ID",
			mutate: func(
				params *CreateNodeExecutionsCommandParams,
				t *testing.T,
			) {
				params.Timeline[1] =
					newNodeCreationEventEntryWithID(
						t,
						params.NodeExecutions[1],
						ExecutionEventID(
							"event-node-execution-1",
						),
						ExecutionEventTypeNodeCreated,
						params.NodeExecutions[1].ID(),
						params.NodeExecutions[1].CreatedAt(),
					)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validCreateNodeExecutionsCommandParams(t)

			test.mutate(&params, t)

			_, err :=
				NewCreateNodeExecutionsCommand(params)
			if err == nil {
				t.Fatal(
					"NewCreateNodeExecutionsCommand() returned nil error",
				)
			}

			requireCreateNodeExecutionsValidationField(
				t,
				err,
				"timeline",
			)
		})
	}
}

func TestCreateNodeExecutionsCommandReturnsDefensiveCopies(
	t *testing.T,
) {
	command, err :=
		NewCreateNodeExecutionsCommand(
			validCreateNodeExecutionsCommandParams(t),
		)
	if err != nil {
		t.Fatalf(
			"NewCreateNodeExecutionsCommand() returned an unexpected error: %v",
			err,
		)
	}

	nodes := command.NodeExecutions()
	nodes[0] = NodeExecutionRecord{}

	if !command.NodeExecutions()[0].IsValid() {
		t.Fatal(
			"NodeExecutions() allowed command mutation",
		)
	}

	timeline := command.Timeline()
	timeline[0] = TimelineEntry{}

	if !command.Timeline()[0].IsValid() {
		t.Fatal(
			"Timeline() allowed command mutation",
		)
	}
}

func TestCreateNodeExecutionsCommandZeroValueIsInvalid(
	t *testing.T,
) {
	var command CreateNodeExecutionsCommand

	if command.IsValid() {
		t.Fatal(
			"zero-value CreateNodeExecutionsCommand was reported as valid",
		)
	}
}

func validCreateNodeExecutionsCommandParams(
	t *testing.T,
) CreateNodeExecutionsCommandParams {
	t.Helper()

	nodes := []NodeExecutionRecord{
		newPendingNodeCreationRecord(
			t,
			"node-execution-1",
			"node-1",
			nil,
		),
		newPendingNodeCreationRecord(
			t,
			"node-execution-2",
			"node-2",
			nil,
		),
	}

	return CreateNodeExecutionsCommandParams{
		CompanyID:                   workflow.CompanyID("company-1"),
		WorkflowExecutionID:         execution.WorkflowExecutionID("execution-1"),
		ExpectedWorkflowStatus:      execution.WorkflowExecutionStatusValidating,
		ExpectedWorkflowLockVersion: 1,
		ExpectedNextSequenceNumber:  SequenceNumber(4),
		NodeExecutions:              nodes,
		Timeline: []TimelineEntry{
			newNodeCreationEventEntry(
				t,
				nodes[0],
				ExecutionEventTypeNodeCreated,
				nodes[0].ID(),
				nodes[0].CreatedAt(),
			),
			newNodeCreationEventEntry(
				t,
				nodes[1],
				ExecutionEventTypeNodeCreated,
				nodes[1].ID(),
				nodes[1].CreatedAt(),
			),
		},
	}
}

func newPendingNodeCreationRecord(
	t *testing.T,
	nodeExecutionID string,
	nodeID string,
	mutate func(
		*NodeExecutionRecordParams,
	),
) NodeExecutionRecord {
	t.Helper()

	params := validNodeExecutionRecordParams()

	params.ID =
		execution.NodeExecutionID(nodeExecutionID)

	params.WorkflowExecutionID =
		execution.WorkflowExecutionID("execution-1")

	params.CompanyID =
		workflow.CompanyID("company-1")

	params.NodeID =
		workflow.NodeID(nodeID)

	params.Status =
		execution.NodeExecutionStatusPending

	params.Attempt = 1
	params.CreatedAt =
		workflowExecutionRecordTestTime(10, 2)

	params.ReadyAt = time.Time{}
	params.QueuedAt = time.Time{}
	params.StartedAt = time.Time{}
	params.FinishedAt = time.Time{}
	params.UpdatedAt = params.CreatedAt

	params.InputSummary = nil
	params.OutputSummary = nil
	params.FailureSummary = nil
	params.LockVersion = 0

	if mutate != nil {
		mutate(&params)
	}

	record, err := NewNodeExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func newNodeCreationEventEntry(
	t *testing.T,
	nodeExecution NodeExecutionRecord,
	eventType ExecutionEventType,
	eventNodeExecutionID execution.NodeExecutionID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	return newNodeCreationEventEntryWithID(
		t,
		nodeExecution,
		ExecutionEventID(
			"event-"+
				nodeExecution.ID().String(),
		),
		eventType,
		eventNodeExecutionID,
		createdAt,
	)
}

func newNodeCreationEventEntryWithID(
	t *testing.T,
	nodeExecution NodeExecutionRecord,
	eventID ExecutionEventID,
	eventType ExecutionEventType,
	eventNodeExecutionID execution.NodeExecutionID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	eventDraft, err :=
		NewExecutionEventDraft(
			ExecutionEventDraftParams{
				ID:                  eventID,
				WorkflowExecutionID: nodeExecution.WorkflowExecutionID(),
				CompanyID:           nodeExecution.CompanyID(),
				NodeExecutionID:     eventNodeExecutionID,
				Type:                eventType,
				NewStatus:           execution.NodeExecutionStatusPending.String(),
				SafeMessage:         "Node execution created",
				CreatedAt:           createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventDraft() returned an unexpected error: %v",
			err,
		)
	}

	entry, err :=
		NewEventTimelineEntry(eventDraft)
	if err != nil {
		t.Fatalf(
			"NewEventTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	return entry
}

func requireCreateNodeExecutionsValidationField(
	t *testing.T,
	err error,
	expectedField string,
) {
	t.Helper()

	var validationError *ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field !=
		expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}
}

func TestNewDefinitionSnapshotStoresNormalizedValues(
	t *testing.T,
) {
	createdAt := time.Date(
		2026,
		time.July,
		17,
		19,
		30,
		0,
		0,
		time.FixedZone(
			"TRT",
			3*60*60,
		),
	)

	snapshot, err := NewDefinitionSnapshot(
		DefinitionSnapshotID(" snapshot-1 "),
		workflow.CompanyID(" company-1 "),
		workflow.WorkflowID(" workflow-1 "),
		3,
		" Customer Import ",
		[]byte(`  {"nodes":[],"edges":[]}  `),
		createdAt,
	)
	if err != nil {
		t.Fatalf(
			"NewDefinitionSnapshot() returned an unexpected error: %v",
			err,
		)
	}

	if actual := snapshot.ID().String(); actual != "snapshot-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"snapshot-1",
		)
	}

	if actual := snapshot.CompanyID().String(); actual != "company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := snapshot.WorkflowID().String(); actual != "workflow-1" {
		t.Fatalf(
			"WorkflowID() = %q, want %q",
			actual,
			"workflow-1",
		)
	}

	if actual := snapshot.WorkflowRevision(); actual != 3 {
		t.Fatalf(
			"WorkflowRevision() = %d, want %d",
			actual,
			3,
		)
	}

	if actual := snapshot.WorkflowName(); actual != "Customer Import" {
		t.Fatalf(
			"WorkflowName() = %q, want %q",
			actual,
			"Customer Import",
		)
	}

	if actual := snapshot.DefinitionJSON().String(); actual !=
		`{"nodes":[],"edges":[]}` {
		t.Fatalf(
			"DefinitionJSON() = %q, want %q",
			actual,
			`{"nodes":[],"edges":[]}`,
		)
	}

	if actual := snapshot.CreatedAt(); !actual.Equal(
		createdAt.UTC(),
	) {
		t.Fatalf(
			"CreatedAt() = %s, want %s",
			actual,
			createdAt.UTC(),
		)
	}

	if snapshot.CreatedAt().Location() != time.UTC {
		t.Fatalf(
			"CreatedAt() location = %s, want UTC",
			snapshot.CreatedAt().Location(),
		)
	}

	if !snapshot.IsValid() {
		t.Fatal(
			"valid DefinitionSnapshot was reported as invalid",
		)
	}
}

func TestNewDefinitionSnapshotRejectsInvalidFields(
	t *testing.T,
) {
	validTime := time.Date(
		2026,
		time.July,
		17,
		16,
		0,
		0,
		0,
		time.UTC,
	)

	tests := []struct {
		name         string
		id           DefinitionSnapshotID
		companyID    workflow.CompanyID
		workflowID   workflow.WorkflowID
		revision     uint64
		workflowName string
		definition   []byte
		createdAt    time.Time
		field        string
	}{
		{
			name:         "blank snapshot ID",
			id:           DefinitionSnapshotID(" "),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     1,
			workflowName: "Workflow",
			definition:   []byte(`{"nodes":[]}`),
			createdAt:    validTime,
			field:        "definitionSnapshotID",
		},
		{
			name:         "blank company ID",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID(" "),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     1,
			workflowName: "Workflow",
			definition:   []byte(`{"nodes":[]}`),
			createdAt:    validTime,
			field:        "companyID",
		},
		{
			name:         "blank workflow ID",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID(" "),
			revision:     1,
			workflowName: "Workflow",
			definition:   []byte(`{"nodes":[]}`),
			createdAt:    validTime,
			field:        "workflowID",
		},
		{
			name:         "zero revision",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     0,
			workflowName: "Workflow",
			definition:   []byte(`{"nodes":[]}`),
			createdAt:    validTime,
			field:        "workflowRevision",
		},
		{
			name:         "revision above PostgreSQL BIGINT",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     uint64(math.MaxInt64) + 1,
			workflowName: "Workflow",
			definition:   []byte(`{"nodes":[]}`),
			createdAt:    validTime,
			field:        "workflowRevision",
		},
		{
			name:         "blank workflow name",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     1,
			workflowName: " ",
			definition:   []byte(`{"nodes":[]}`),
			createdAt:    validTime,
			field:        "workflowName",
		},
		{
			name:         "blank definition",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     1,
			workflowName: "Workflow",
			definition:   nil,
			createdAt:    validTime,
			field:        "definitionJSON",
		},
		{
			name:         "invalid definition JSON",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     1,
			workflowName: "Workflow",
			definition:   []byte(`{"nodes":`),
			createdAt:    validTime,
			field:        "definitionJSON",
		},
		{
			name:         "definition root is array",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     1,
			workflowName: "Workflow",
			definition:   []byte(`[]`),
			createdAt:    validTime,
			field:        "definitionJSON",
		},
		{
			name:         "zero creation time",
			id:           DefinitionSnapshotID("snapshot-1"),
			companyID:    workflow.CompanyID("company-1"),
			workflowID:   workflow.WorkflowID("workflow-1"),
			revision:     1,
			workflowName: "Workflow",
			definition:   []byte(`{"nodes":[]}`),
			createdAt:    time.Time{},
			field:        "createdAt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewDefinitionSnapshot(
				test.id,
				test.companyID,
				test.workflowID,
				test.revision,
				test.workflowName,
				test.definition,
				test.createdAt,
			)
			if err == nil {
				t.Fatal(
					"NewDefinitionSnapshot() returned nil error for an invalid field",
				)
			}

			var validationError *ValidationError
			if !errors.As(
				err,
				&validationError,
			) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestDefinitionSnapshotDefensivelyCopiesDefinitionInput(
	t *testing.T,
) {
	source := []byte(`{"nodes":[]}`)
	expected := bytes.Clone(source)

	snapshot := newTestDefinitionSnapshot(
		t,
		source,
	)

	source[2] = 'X'

	if actual := snapshot.DefinitionJSON().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"stored definition = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestDefinitionSnapshotDefinitionReturnsDefensiveCopy(
	t *testing.T,
) {
	expected := []byte(`{"nodes":[]}`)

	snapshot := newTestDefinitionSnapshot(
		t,
		expected,
	)

	first := snapshot.DefinitionJSON()
	firstBytes := first.Bytes()
	firstBytes[2] = 'X'

	second := snapshot.DefinitionJSON()

	if actual := second.Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"stored definition = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestDefinitionSnapshotZeroValueIsInvalid(
	t *testing.T,
) {
	var snapshot DefinitionSnapshot

	if snapshot.IsValid() {
		t.Fatal(
			"zero-value DefinitionSnapshot was reported as valid",
		)
	}
}

func newTestDefinitionSnapshot(
	t *testing.T,
	definition []byte,
) DefinitionSnapshot {
	t.Helper()

	snapshot, err := NewDefinitionSnapshot(
		DefinitionSnapshotID("snapshot-1"),
		workflow.CompanyID("company-1"),
		workflow.WorkflowID("workflow-1"),
		1,
		"Workflow",
		definition,
		time.Date(
			2026,
			time.July,
			17,
			16,
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

func TestNewExecutionErrorRecordStoresNormalizedValues(
	t *testing.T,
) {
	params := validExecutionErrorRecordParams()

	params.ID = ExecutionErrorID(" error-1 ")
	params.WorkflowExecutionID =
		execution.WorkflowExecutionID(" workflow-execution-1 ")
	params.CompanyID = workflow.CompanyID(" company-1 ")
	params.NodeExecutionID =
		execution.NodeExecutionID(" node-execution-1 ")
	params.RelatedEventID =
		ExecutionEventID(" event-1 ")
	params.Category =
		runtime.FailureCategory(" dependency ")
	params.Code = " UPSTREAM_UNAVAILABLE "
	params.SafeMessage =
		" Billing service is unavailable "
	params.TechnicalDetail =
		" dial tcp: connection refused "

	record, err := NewExecutionErrorRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an unexpected error: %v",
			err,
		)
	}

	if actual := record.ID().String(); actual != "error-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"error-1",
		)
	}

	if actual := record.WorkflowExecutionID().String(); actual !=
		"workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q",
			actual,
		)
	}

	if actual := record.CompanyID().String(); actual !=
		"company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	nodeExecutionID, exists := record.NodeExecutionID()
	if !exists {
		t.Fatal(
			"NodeExecutionID() reported no value",
		)
	}

	if actual := nodeExecutionID.String(); actual !=
		"node-execution-1" {
		t.Fatalf(
			"NodeExecutionID() = %q",
			actual,
		)
	}

	relatedEventID, exists := record.RelatedEventID()
	if !exists {
		t.Fatal(
			"RelatedEventID() reported no value",
		)
	}

	if actual := relatedEventID.String(); actual != "event-1" {
		t.Fatalf(
			"RelatedEventID() = %q, want %q",
			actual,
			"event-1",
		)
	}

	if record.Category() != runtime.FailureCategoryDependency {
		t.Fatalf(
			"Category() = %q, want %q",
			record.Category(),
			runtime.FailureCategoryDependency,
		)
	}

	if actual := record.Code(); actual !=
		"UPSTREAM_UNAVAILABLE" {
		t.Fatalf(
			"Code() = %q",
			actual,
		)
	}

	if actual := record.SafeMessage(); actual !=
		"Billing service is unavailable" {
		t.Fatalf(
			"SafeMessage() = %q",
			actual,
		)
	}

	technicalDetail, exists := record.TechnicalDetail()
	if !exists {
		t.Fatal(
			"TechnicalDetail() reported no value",
		)
	}

	if technicalDetail != "dial tcp: connection refused" {
		t.Fatalf(
			"TechnicalDetail() = %q",
			technicalDetail,
		)
	}

	if !record.Retryable() {
		t.Fatal(
			"Retryable() = false, want true",
		)
	}

	if record.CreatedAt().Location() != time.UTC {
		t.Fatalf(
			"CreatedAt() location = %s, want UTC",
			record.CreatedAt().Location(),
		)
	}

	if !record.IsValid() {
		t.Fatal(
			"valid ExecutionErrorRecord was reported as invalid",
		)
	}
}

func TestNewExecutionErrorRecordAllowsAbsentOptionalFields(
	t *testing.T,
) {
	params := validExecutionErrorRecordParams()

	params.NodeExecutionID = ""
	params.RelatedEventID = ""
	params.TechnicalDetail = ""
	params.Details = nil
	params.Retryable = false

	record, err := NewExecutionErrorRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an unexpected error: %v",
			err,
		)
	}

	if _, exists := record.NodeExecutionID(); exists {
		t.Fatal(
			"NodeExecutionID() reported an absent value",
		)
	}

	if _, exists := record.RelatedEventID(); exists {
		t.Fatal(
			"RelatedEventID() reported an absent value",
		)
	}

	if _, exists := record.TechnicalDetail(); exists {
		t.Fatal(
			"TechnicalDetail() reported an absent value",
		)
	}

	if actual := record.Details().String(); actual != "{}" {
		t.Fatalf(
			"Details() = %q, want %q",
			actual,
			"{}",
		)
	}

	if record.Retryable() {
		t.Fatal(
			"Retryable() = true, want false",
		)
	}
}

func TestNewExecutionErrorRecordRejectsInvalidFields(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(*ExecutionErrorRecordParams)
	}{
		{
			name:  "blank error ID",
			field: "executionErrorID",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.ID = ExecutionErrorID(" ")
			},
		},
		{
			name:  "blank workflow execution ID",
			field: "workflowExecutionID",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.WorkflowExecutionID =
					execution.WorkflowExecutionID(" ")
			},
		},
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.CompanyID = workflow.CompanyID(" ")
			},
		},
		{
			name:  "blank provided node execution ID",
			field: "nodeExecutionID",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.NodeExecutionID =
					execution.NodeExecutionID("   ")
			},
		},
		{
			name:  "blank provided related event ID",
			field: "relatedEventID",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.RelatedEventID =
					ExecutionEventID("   ")
			},
		},
		{
			name:  "invalid category",
			field: "category",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.Category =
					runtime.FailureCategory("NETWORK")
			},
		},
		{
			name:  "blank code",
			field: "code",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.Code = "   "
			},
		},
		{
			name:  "code above maximum",
			field: "code",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.Code = strings.Repeat(
					"a",
					maximumExecutionErrorCodeCharacters+1,
				)
			},
		},
		{
			name:  "blank safe message",
			field: "safeMessage",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.SafeMessage = "   "
			},
		},
		{
			name:  "safe message above maximum",
			field: "safeMessage",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.SafeMessage = strings.Repeat(
					"a",
					maximumExecutionErrorSafeMessageCharacters+1,
				)
			},
		},
		{
			name:  "blank provided technical detail",
			field: "technicalDetail",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.TechnicalDetail = "   "
			},
		},
		{
			name:  "technical detail above maximum",
			field: "technicalDetail",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.TechnicalDetail = strings.Repeat(
					"a",
					maximumExecutionErrorTechnicalDetailCharacters+1,
				)
			},
		},
		{
			name:  "invalid details",
			field: "details",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.Details = []byte(`[]`)
			},
		},
		{
			name:  "zero creation time",
			field: "createdAt",
			mutate: func(params *ExecutionErrorRecordParams) {
				params.CreatedAt = time.Time{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validExecutionErrorRecordParams()
			test.mutate(&params)

			_, err := NewExecutionErrorRecord(params)
			if err == nil {
				t.Fatal(
					"NewExecutionErrorRecord() returned nil error for invalid parameters",
				)
			}

			var validationError *ValidationError
			if !errors.As(
				err,
				&validationError,
			) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestExecutionErrorRecordDefensivelyCopiesDetails(
	t *testing.T,
) {
	details := []byte(
		`{"service":"billing","operation":"fetch-customer"}`,
	)
	expected := bytes.Clone(details)

	params := validExecutionErrorRecordParams()
	params.Details = details

	record, err := NewExecutionErrorRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an unexpected error: %v",
			err,
		)
	}

	details[2] = 'X'

	if actual := record.Details().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"Details() = %q, want %q",
			actual,
			expected,
		)
	}

	returned := record.Details().Bytes()
	returned[2] = 'X'

	if actual := record.Details().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"Details() changed through getter: %q",
			actual,
		)
	}
}

func TestExecutionErrorRecordZeroValueIsInvalid(
	t *testing.T,
) {
	var record ExecutionErrorRecord

	if record.IsValid() {
		t.Fatal(
			"zero-value ExecutionErrorRecord was reported as valid",
		)
	}
}

func validExecutionErrorRecordParams() ExecutionErrorRecordParams {
	return ExecutionErrorRecordParams{
		ID: ExecutionErrorID(
			"error-1",
		),
		WorkflowExecutionID: execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		CompanyID: workflow.CompanyID(
			"company-1",
		),
		NodeExecutionID: execution.NodeExecutionID(
			"node-execution-1",
		),
		RelatedEventID: ExecutionEventID(
			"event-1",
		),
		Category:        runtime.FailureCategoryDependency,
		Code:            "UPSTREAM_UNAVAILABLE",
		SafeMessage:     "Billing service is unavailable",
		TechnicalDetail: "dial tcp: connection refused",
		Retryable:       true,
		Details: []byte(
			`{"service":"billing"}`,
		),
		CreatedAt: executionErrorRecordTestTime(
			10,
			0,
		),
	}
}

func executionErrorRecordTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		17,
		hour,
		minute,
		0,
		0,
		time.FixedZone(
			"TRT",
			3*60*60,
		),
	)
}

func TestNewExecutionEventRecordStoresNormalizedWorkflowEvent(
	t *testing.T,
) {
	params := validWorkflowExecutionEventRecordParams()

	params.ID = ExecutionEventID(" event-1 ")
	params.WorkflowExecutionID =
		execution.WorkflowExecutionID(" workflow-execution-1 ")
	params.CompanyID = workflow.CompanyID(" company-1 ")
	params.Type = ExecutionEventType(" workflow_started ")
	params.PreviousStatus = " validating "
	params.NewStatus = " running "
	params.CorrelationID = " correlation-1 "
	params.CausationID = " command-1 "
	params.SafeMessage = " Workflow execution started "

	record, err := NewExecutionEventRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventRecord() returned an unexpected error: %v",
			err,
		)
	}

	if actual := record.ID().String(); actual != "event-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"event-1",
		)
	}

	if actual := record.WorkflowExecutionID().String(); actual !=
		"workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q",
			actual,
		)
	}

	if _, exists := record.NodeExecutionID(); exists {
		t.Fatal(
			"NodeExecutionID() reported a value for a workflow event",
		)
	}

	if record.Type() != ExecutionEventTypeWorkflowStarted {
		t.Fatalf(
			"Type() = %q, want %q",
			record.Type(),
			ExecutionEventTypeWorkflowStarted,
		)
	}

	if actual, exists := record.PreviousStatus(); !exists ||
		actual != execution.WorkflowExecutionStatusValidating.String() {
		t.Fatalf(
			"PreviousStatus() = %q, %t",
			actual,
			exists,
		)
	}

	if actual, exists := record.NewStatus(); !exists ||
		actual != execution.WorkflowExecutionStatusRunning.String() {
		t.Fatalf(
			"NewStatus() = %q, %t",
			actual,
			exists,
		)
	}

	if actual, exists := record.SafeMessage(); !exists ||
		actual != "Workflow execution started" {
		t.Fatalf(
			"SafeMessage() = %q, %t",
			actual,
			exists,
		)
	}

	if record.CreatedAt().Location() != time.UTC {
		t.Fatalf(
			"CreatedAt() location = %s, want UTC",
			record.CreatedAt().Location(),
		)
	}

	if !record.IsValid() {
		t.Fatal(
			"valid ExecutionEventRecord was reported as invalid",
		)
	}
}

func TestNewExecutionEventRecordStoresNodeScopedEvent(
	t *testing.T,
) {
	params := validWorkflowExecutionEventRecordParams()

	params.NodeExecutionID =
		execution.NodeExecutionID("node-execution-1")
	params.Type = ExecutionEventTypeNodeSucceeded
	params.PreviousStatus =
		execution.NodeExecutionStatusRunning.String()
	params.NewStatus =
		execution.NodeExecutionStatusSucceeded.String()

	record, err := NewExecutionEventRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventRecord() returned an unexpected error: %v",
			err,
		)
	}

	nodeExecutionID, exists := record.NodeExecutionID()
	if !exists {
		t.Fatal(
			"NodeExecutionID() reported no value for a node event",
		)
	}

	if actual := nodeExecutionID.String(); actual !=
		"node-execution-1" {
		t.Fatalf(
			"NodeExecutionID() = %q",
			actual,
		)
	}

	if !record.Type().IsNodeScoped() {
		t.Fatal(
			"node event was not classified as node-scoped",
		)
	}
}

func TestNewExecutionEventRecordAllowsAbsentOptionalFields(
	t *testing.T,
) {
	params := validWorkflowExecutionEventRecordParams()

	params.Type = ExecutionEventTypeWorkflowCreated
	params.PreviousStatus = ""
	params.NewStatus = ""
	params.CorrelationID = ""
	params.CausationID = ""
	params.SafeMessage = ""
	params.Metadata = nil

	record, err := NewExecutionEventRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventRecord() returned an unexpected error: %v",
			err,
		)
	}

	if _, exists := record.PreviousStatus(); exists {
		t.Fatal(
			"PreviousStatus() reported an absent value",
		)
	}

	if _, exists := record.NewStatus(); exists {
		t.Fatal(
			"NewStatus() reported an absent value",
		)
	}

	if _, exists := record.CorrelationID(); exists {
		t.Fatal(
			"CorrelationID() reported an absent value",
		)
	}

	if _, exists := record.CausationID(); exists {
		t.Fatal(
			"CausationID() reported an absent value",
		)
	}

	if _, exists := record.SafeMessage(); exists {
		t.Fatal(
			"SafeMessage() reported an absent value",
		)
	}

	if actual := record.Metadata().String(); actual != "{}" {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			"{}",
		)
	}
}

func TestNewExecutionEventRecordRejectsInvalidFields(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(*ExecutionEventRecordParams)
	}{
		{
			name:  "blank event ID",
			field: "executionEventID",
			mutate: func(params *ExecutionEventRecordParams) {
				params.ID = ExecutionEventID(" ")
			},
		},
		{
			name:  "blank workflow execution ID",
			field: "workflowExecutionID",
			mutate: func(params *ExecutionEventRecordParams) {
				params.WorkflowExecutionID =
					execution.WorkflowExecutionID(" ")
			},
		},
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(params *ExecutionEventRecordParams) {
				params.CompanyID = workflow.CompanyID(" ")
			},
		},
		{
			name:  "zero sequence",
			field: "sequenceNumber",
			mutate: func(params *ExecutionEventRecordParams) {
				params.SequenceNumber = 0
			},
		},
		{
			name:  "invalid event type",
			field: "eventType",
			mutate: func(params *ExecutionEventRecordParams) {
				params.Type = ExecutionEventType("UNKNOWN")
			},
		},
		{
			name:  "node event without node execution ID",
			field: "nodeExecutionID",
			mutate: func(params *ExecutionEventRecordParams) {
				params.Type = ExecutionEventTypeNodeStarted
				params.NodeExecutionID = ""
				params.PreviousStatus =
					execution.NodeExecutionStatusReady.String()
				params.NewStatus =
					execution.NodeExecutionStatusRunning.String()
			},
		},
		{
			name:  "workflow event with node execution ID",
			field: "nodeExecutionID",
			mutate: func(params *ExecutionEventRecordParams) {
				params.NodeExecutionID =
					execution.NodeExecutionID("node-execution-1")
			},
		},
		{
			name:  "workflow event with node status",
			field: "previousStatus",
			mutate: func(params *ExecutionEventRecordParams) {
				params.PreviousStatus =
					execution.NodeExecutionStatusPending.String()
			},
		},
		{
			name:  "node event with workflow status",
			field: "newStatus",
			mutate: func(params *ExecutionEventRecordParams) {
				params.Type = ExecutionEventTypeNodeStarted
				params.NodeExecutionID =
					execution.NodeExecutionID("node-execution-1")
				params.PreviousStatus =
					execution.NodeExecutionStatusReady.String()
				params.NewStatus =
					execution.WorkflowExecutionStatusValidating.String()
			},
		},
		{
			name:  "blank provided correlation ID",
			field: "correlationID",
			mutate: func(params *ExecutionEventRecordParams) {
				params.CorrelationID = "   "
			},
		},
		{
			name:  "blank provided causation ID",
			field: "causationID",
			mutate: func(params *ExecutionEventRecordParams) {
				params.CausationID = "   "
			},
		},
		{
			name:  "blank provided safe message",
			field: "safeMessage",
			mutate: func(params *ExecutionEventRecordParams) {
				params.SafeMessage = "   "
			},
		},
		{
			name:  "safe message above maximum",
			field: "safeMessage",
			mutate: func(params *ExecutionEventRecordParams) {
				params.SafeMessage = strings.Repeat(
					"a",
					maximumExecutionEventMessageCharacters+1,
				)
			},
		},
		{
			name:  "invalid metadata",
			field: "metadata",
			mutate: func(params *ExecutionEventRecordParams) {
				params.Metadata = []byte(`[]`)
			},
		},
		{
			name:  "zero creation time",
			field: "createdAt",
			mutate: func(params *ExecutionEventRecordParams) {
				params.CreatedAt = time.Time{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validWorkflowExecutionEventRecordParams()
			test.mutate(&params)

			_, err := NewExecutionEventRecord(params)
			if err == nil {
				t.Fatal(
					"NewExecutionEventRecord() returned nil error for invalid parameters",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestExecutionEventRecordDefensivelyCopiesMetadata(
	t *testing.T,
) {
	metadata := []byte(`{"source":"sync-runner"}`)
	expected := bytes.Clone(metadata)

	params := validWorkflowExecutionEventRecordParams()
	params.Metadata = metadata

	record, err := NewExecutionEventRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventRecord() returned an unexpected error: %v",
			err,
		)
	}

	metadata[2] = 'X'

	if actual := record.Metadata().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			expected,
		)
	}

	returned := record.Metadata().Bytes()
	returned[2] = 'X'

	if actual := record.Metadata().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"Metadata() changed through getter: %q",
			actual,
		)
	}
}

func TestExecutionEventRecordZeroValueIsInvalid(
	t *testing.T,
) {
	var record ExecutionEventRecord

	if record.IsValid() {
		t.Fatal(
			"zero-value ExecutionEventRecord was reported as valid",
		)
	}
}

func validWorkflowExecutionEventRecordParams() ExecutionEventRecordParams {
	return ExecutionEventRecordParams{
		ID: ExecutionEventID(
			"event-1",
		),
		WorkflowExecutionID: execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		CompanyID: workflow.CompanyID(
			"company-1",
		),
		SequenceNumber: SequenceNumber(3),
		Type:           ExecutionEventTypeWorkflowStarted,
		PreviousStatus: execution.WorkflowExecutionStatusValidating.String(),
		NewStatus:      execution.WorkflowExecutionStatusRunning.String(),
		CorrelationID:  "correlation-1",
		CausationID:    "command-1",
		SafeMessage:    "Workflow execution started",
		Metadata: []byte(
			`{"source":"sync-runner"}`,
		),
		CreatedAt: executionEventRecordTestTime(
			10,
			0,
		),
	}
}

func executionEventRecordTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		17,
		hour,
		minute,
		0,
		0,
		time.FixedZone(
			"TRT",
			3*60*60,
		),
	)
}

func TestNewExecutionLogRecordStoresNormalizedValues(
	t *testing.T,
) {
	params := validExecutionLogRecordParams()

	params.ID = ExecutionLogID(" log-1 ")
	params.WorkflowExecutionID =
		execution.WorkflowExecutionID(" workflow-execution-1 ")
	params.CompanyID = workflow.CompanyID(" company-1 ")
	params.NodeExecutionID =
		execution.NodeExecutionID(" node-execution-1 ")
	params.Level = ExecutionLogLevel(" info ")
	params.Message = " Node execution completed "

	record, err := NewExecutionLogRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogRecord() returned an unexpected error: %v",
			err,
		)
	}

	if actual := record.ID().String(); actual != "log-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"log-1",
		)
	}

	if actual := record.WorkflowExecutionID().String(); actual !=
		"workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q",
			actual,
		)
	}

	if actual := record.CompanyID().String(); actual !=
		"company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	nodeExecutionID, exists := record.NodeExecutionID()
	if !exists {
		t.Fatal(
			"NodeExecutionID() reported no value",
		)
	}

	if actual := nodeExecutionID.String(); actual !=
		"node-execution-1" {
		t.Fatalf(
			"NodeExecutionID() = %q",
			actual,
		)
	}

	if record.SequenceNumber() != SequenceNumber(5) {
		t.Fatalf(
			"SequenceNumber() = %d, want 5",
			record.SequenceNumber(),
		)
	}

	if record.Level() != ExecutionLogLevelInfo {
		t.Fatalf(
			"Level() = %q, want %q",
			record.Level(),
			ExecutionLogLevelInfo,
		)
	}

	if actual := record.Message(); actual !=
		"Node execution completed" {
		t.Fatalf(
			"Message() = %q",
			actual,
		)
	}

	if record.CreatedAt().Location() != time.UTC {
		t.Fatalf(
			"CreatedAt() location = %s, want UTC",
			record.CreatedAt().Location(),
		)
	}

	if !record.IsValid() {
		t.Fatal(
			"valid ExecutionLogRecord was reported as invalid",
		)
	}
}

func TestNewExecutionLogRecordAllowsWorkflowScopedLog(
	t *testing.T,
) {
	params := validExecutionLogRecordParams()

	params.NodeExecutionID = ""
	params.Metadata = nil

	record, err := NewExecutionLogRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogRecord() returned an unexpected error: %v",
			err,
		)
	}

	if _, exists := record.NodeExecutionID(); exists {
		t.Fatal(
			"NodeExecutionID() reported a value for a workflow-scoped log",
		)
	}

	if actual := record.Metadata().String(); actual != "{}" {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			"{}",
		)
	}
}

func TestNewExecutionLogRecordRejectsInvalidFields(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(*ExecutionLogRecordParams)
	}{
		{
			name:  "blank log ID",
			field: "executionLogID",
			mutate: func(params *ExecutionLogRecordParams) {
				params.ID = ExecutionLogID(" ")
			},
		},
		{
			name:  "blank workflow execution ID",
			field: "workflowExecutionID",
			mutate: func(params *ExecutionLogRecordParams) {
				params.WorkflowExecutionID =
					execution.WorkflowExecutionID(" ")
			},
		},
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(params *ExecutionLogRecordParams) {
				params.CompanyID = workflow.CompanyID(" ")
			},
		},
		{
			name:  "blank provided node execution ID",
			field: "nodeExecutionID",
			mutate: func(params *ExecutionLogRecordParams) {
				params.NodeExecutionID =
					execution.NodeExecutionID("   ")
			},
		},
		{
			name:  "zero sequence",
			field: "sequenceNumber",
			mutate: func(params *ExecutionLogRecordParams) {
				params.SequenceNumber = 0
			},
		},
		{
			name:  "invalid level",
			field: "logLevel",
			mutate: func(params *ExecutionLogRecordParams) {
				params.Level =
					ExecutionLogLevel("TRACE")
			},
		},
		{
			name:  "blank message",
			field: "message",
			mutate: func(params *ExecutionLogRecordParams) {
				params.Message = "   "
			},
		},
		{
			name:  "message above maximum",
			field: "message",
			mutate: func(params *ExecutionLogRecordParams) {
				params.Message = strings.Repeat(
					"a",
					maximumExecutionLogMessageCharacters+1,
				)
			},
		},
		{
			name:  "invalid metadata",
			field: "metadata",
			mutate: func(params *ExecutionLogRecordParams) {
				params.Metadata = []byte(`[]`)
			},
		},
		{
			name:  "zero creation time",
			field: "createdAt",
			mutate: func(params *ExecutionLogRecordParams) {
				params.CreatedAt = time.Time{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validExecutionLogRecordParams()
			test.mutate(&params)

			_, err := NewExecutionLogRecord(params)
			if err == nil {
				t.Fatal(
					"NewExecutionLogRecord() returned nil error for invalid parameters",
				)
			}

			var validationError *ValidationError
			if !errors.As(
				err,
				&validationError,
			) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestExecutionLogMessageLimitUsesUnicodeCharacters(
	t *testing.T,
) {
	params := validExecutionLogRecordParams()

	params.Message = strings.Repeat(
		"ğ",
		maximumExecutionLogMessageCharacters,
	)

	record, err := NewExecutionLogRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogRecord() rejected a message at the character limit: %v",
			err,
		)
	}

	if actual := len([]rune(record.Message())); actual !=
		maximumExecutionLogMessageCharacters {
		t.Fatalf(
			"message rune count = %d, want %d",
			actual,
			maximumExecutionLogMessageCharacters,
		)
	}

	params.Message = strings.Repeat(
		"ğ",
		maximumExecutionLogMessageCharacters+1,
	)

	_, err = NewExecutionLogRecord(params)
	if err == nil {
		t.Fatal(
			"NewExecutionLogRecord() accepted a message above the Unicode character limit",
		)
	}
}

func TestExecutionLogRecordDefensivelyCopiesMetadata(
	t *testing.T,
) {
	metadata := []byte(
		`{"pluginType":"core.pass-through"}`,
	)
	expected := bytes.Clone(metadata)

	params := validExecutionLogRecordParams()
	params.Metadata = metadata

	record, err := NewExecutionLogRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogRecord() returned an unexpected error: %v",
			err,
		)
	}

	metadata[2] = 'X'

	if actual := record.Metadata().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			expected,
		)
	}

	returned := record.Metadata().Bytes()
	returned[2] = 'X'

	if actual := record.Metadata().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"Metadata() changed through getter: %q",
			actual,
		)
	}
}

func TestExecutionLogRecordZeroValueIsInvalid(
	t *testing.T,
) {
	var record ExecutionLogRecord

	if record.IsValid() {
		t.Fatal(
			"zero-value ExecutionLogRecord was reported as valid",
		)
	}
}

func validExecutionLogRecordParams() ExecutionLogRecordParams {
	return ExecutionLogRecordParams{
		ID: ExecutionLogID(
			"log-1",
		),
		WorkflowExecutionID: execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		CompanyID: workflow.CompanyID(
			"company-1",
		),
		NodeExecutionID: execution.NodeExecutionID(
			"node-execution-1",
		),
		SequenceNumber: SequenceNumber(5),
		Level:          ExecutionLogLevelInfo,
		Message:        "Node execution completed",
		Metadata: []byte(
			`{"pluginType":"core.pass-through"}`,
		),
		CreatedAt: executionLogRecordTestTime(
			10,
			0,
		),
	}
}

func executionLogRecordTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		17,
		hour,
		minute,
		0,
		0,
		time.FixedZone(
			"TRT",
			3*60*60,
		),
	)
}

const testHTTPIdempotencyFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestNewHTTPIdempotencyReservationStoresNormalizedValues(
	t *testing.T,
) {
	createdAt := time.Date(
		2026,
		time.July,
		20,
		8,
		0,
		0,
		0,
		time.UTC,
	)

	reservation, err :=
		NewHTTPIdempotencyReservation(
			HTTPIdempotencyReservationParams{
				CompanyID: workflow.CompanyID(
					" company-1 ",
				),
				IdempotencyKey: " request-1 ",
				RequestFingerprint: strings.ToUpper(
					testHTTPIdempotencyFingerprint,
				),
				WorkflowExecutionID: execution.WorkflowExecutionID(
					" execution-1 ",
				),
				CreatedAt: createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewHTTPIdempotencyReservation() error = %v",
			err,
		)
	}

	record := reservation.Record()

	if record.CompanyID().String() !=
		"company-1" {
		t.Fatalf(
			"company ID = %q",
			record.CompanyID(),
		)
	}

	if record.IdempotencyKey() !=
		"request-1" {
		t.Fatalf(
			"idempotency key = %q",
			record.IdempotencyKey(),
		)
	}

	if record.RequestFingerprint() !=
		testHTTPIdempotencyFingerprint {
		t.Fatalf(
			"fingerprint = %q",
			record.RequestFingerprint(),
		)
	}

	if record.WorkflowExecutionID().String() !=
		"execution-1" {
		t.Fatalf(
			"workflow execution ID = %q",
			record.WorkflowExecutionID(),
		)
	}

	if record.State() !=
		HTTPIdempotencyStateReserved {
		t.Fatalf(
			"state = %q",
			record.State(),
		)
	}

	if !record.CreatedAt().Equal(
		createdAt,
	) {
		t.Fatalf(
			"createdAt = %v",
			record.CreatedAt(),
		)
	}

	if _, exists := record.AcceptedAt(); exists {
		t.Fatal(
			"acceptedAt exists for RESERVED record",
		)
	}

	if !reservation.IsValid() {
		t.Fatal(
			"reservation is invalid",
		)
	}
}

func TestNewHTTPIdempotencyReservationRejectsInvalidValues(
	t *testing.T,
) {
	createdAt := time.Date(
		2026,
		time.July,
		20,
		8,
		0,
		0,
		0,
		time.UTC,
	)

	tests := []struct {
		name   string
		params HTTPIdempotencyReservationParams
	}{
		{
			name: "blank company",
			params: HTTPIdempotencyReservationParams{
				CompanyID:           " ",
				IdempotencyKey:      "request-1",
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: "execution-1",
				CreatedAt:           createdAt,
			},
		},
		{
			name: "blank idempotency key",
			params: HTTPIdempotencyReservationParams{
				CompanyID:           "company-1",
				IdempotencyKey:      " ",
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: "execution-1",
				CreatedAt:           createdAt,
			},
		},
		{
			name: "oversized idempotency key",
			params: HTTPIdempotencyReservationParams{
				CompanyID: "company-1",
				IdempotencyKey: strings.Repeat(
					"a",
					MaximumHTTPIdempotencyKeyCharacters+1,
				),
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: "execution-1",
				CreatedAt:           createdAt,
			},
		},
		{
			name: "invalid fingerprint",
			params: HTTPIdempotencyReservationParams{
				CompanyID:           "company-1",
				IdempotencyKey:      "request-1",
				RequestFingerprint:  "invalid",
				WorkflowExecutionID: "execution-1",
				CreatedAt:           createdAt,
			},
		},
		{
			name: "blank execution",
			params: HTTPIdempotencyReservationParams{
				CompanyID:           "company-1",
				IdempotencyKey:      "request-1",
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: " ",
				CreatedAt:           createdAt,
			},
		},
		{
			name: "zero created time",
			params: HTTPIdempotencyReservationParams{
				CompanyID:           "company-1",
				IdempotencyKey:      "request-1",
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: "execution-1",
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err :=
					NewHTTPIdempotencyReservation(
						test.params,
					)

				if err == nil {
					t.Fatal(
						"expected validation error",
					)
				}
			},
		)
	}
}

func TestNewHTTPIdempotencyRecordEnforcesAcceptanceState(
	t *testing.T,
) {
	createdAt := time.Date(
		2026,
		time.July,
		20,
		8,
		0,
		0,
		0,
		time.UTC,
	)

	acceptedAt :=
		createdAt.Add(
			time.Second,
		)

	record, err :=
		NewHTTPIdempotencyRecord(
			HTTPIdempotencyRecordParams{
				CompanyID:           "company-1",
				IdempotencyKey:      "request-1",
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: "execution-1",
				State:               HTTPIdempotencyStateAccepted,
				CreatedAt:           createdAt,
				AcceptedAt:          acceptedAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewHTTPIdempotencyRecord() error = %v",
			err,
		)
	}

	if record.State() !=
		HTTPIdempotencyStateAccepted {
		t.Fatalf(
			"state = %q",
			record.State(),
		)
	}

	actualAcceptedAt, exists :=
		record.AcceptedAt()

	if !exists {
		t.Fatal(
			"acceptedAt does not exist",
		)
	}

	if !actualAcceptedAt.Equal(
		acceptedAt,
	) {
		t.Fatalf(
			"acceptedAt = %v",
			actualAcceptedAt,
		)
	}

	_, err =
		NewHTTPIdempotencyRecord(
			HTTPIdempotencyRecordParams{
				CompanyID:           "company-1",
				IdempotencyKey:      "request-1",
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: "execution-1",
				State:               HTTPIdempotencyStateAccepted,
				CreatedAt:           createdAt,
			},
		)

	if err == nil {
		t.Fatal(
			"ACCEPTED record without acceptedAt was accepted",
		)
	}

	_, err =
		NewHTTPIdempotencyRecord(
			HTTPIdempotencyRecordParams{
				CompanyID:           "company-1",
				IdempotencyKey:      "request-1",
				RequestFingerprint:  testHTTPIdempotencyFingerprint,
				WorkflowExecutionID: "execution-1",
				State:               HTTPIdempotencyStateReserved,
				CreatedAt:           createdAt,
				AcceptedAt:          acceptedAt,
			},
		)

	if err == nil {
		t.Fatal(
			"RESERVED record with acceptedAt was accepted",
		)
	}
}

func TestNewHTTPIdempotencyAcceptanceStoresValues(
	t *testing.T,
) {
	acceptedAt := time.Date(
		2026,
		time.July,
		20,
		8,
		1,
		0,
		0,
		time.UTC,
	)

	acceptance, err :=
		NewHTTPIdempotencyAcceptance(
			HTTPIdempotencyAcceptanceParams{
				CompanyID:      " company-1 ",
				IdempotencyKey: " request-1 ",
				RequestFingerprint: strings.ToUpper(
					testHTTPIdempotencyFingerprint,
				),
				WorkflowExecutionID: " execution-1 ",
				AcceptedAt:          acceptedAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewHTTPIdempotencyAcceptance() error = %v",
			err,
		)
	}

	if acceptance.CompanyID().String() !=
		"company-1" {
		t.Fatalf(
			"company ID = %q",
			acceptance.CompanyID(),
		)
	}

	if acceptance.IdempotencyKey() !=
		"request-1" {
		t.Fatalf(
			"idempotency key = %q",
			acceptance.IdempotencyKey(),
		)
	}

	if acceptance.RequestFingerprint() !=
		testHTTPIdempotencyFingerprint {
		t.Fatalf(
			"fingerprint = %q",
			acceptance.RequestFingerprint(),
		)
	}

	if acceptance.WorkflowExecutionID().String() !=
		"execution-1" {
		t.Fatalf(
			"workflow execution ID = %q",
			acceptance.WorkflowExecutionID(),
		)
	}

	if !acceptance.AcceptedAt().Equal(
		acceptedAt,
	) {
		t.Fatalf(
			"acceptedAt = %v",
			acceptance.AcceptedAt(),
		)
	}

	if !acceptance.IsValid() {
		t.Fatal(
			"acceptance is invalid",
		)
	}
}

func TestHTTPIdempotencyStates(
	t *testing.T,
) {
	if !HTTPIdempotencyStateReserved.IsValid() {
		t.Fatal(
			"RESERVED state is invalid",
		)
	}

	if !HTTPIdempotencyStateAccepted.IsValid() {
		t.Fatal(
			"ACCEPTED state is invalid",
		)
	}

	if HTTPIdempotencyState(
		"UNKNOWN",
	).IsValid() {
		t.Fatal(
			"UNKNOWN state is valid",
		)
	}
}

func validInboxParams() InboxMessageParams {
	position, _ := NewInboxSourcePosition("workflow-node-results", 2, 42)
	return InboxMessageParams{
		ConsumerIdentity:    ConsumerIdentity("engine-result-consumer"),
		MessageID:           MessageID("message-1"),
		CompanyID:           workflow.CompanyID("company-1"),
		WorkflowExecutionID: execution.WorkflowExecutionID("execution-1"),
		NodeExecutionID:     execution.NodeExecutionID("node-execution-1"),
		MessageType:         "miletos.node-result",
		MessageVersion:      1,
		ProcessingResult:    InboxProcessingApplied,
		SourcePosition:      &position,
		ReceivedAt:          asyncTestTime,
		ProcessedAt:         asyncTestTime.Add(time.Second),
	}
}

func TestNewInboxMessageAcceptsValidMessage(t *testing.T) {
	message, err := NewInboxMessage(validInboxParams())
	if err != nil {
		t.Fatalf("NewInboxMessage() error = %v", err)
	}
	if !message.IsValid() {
		t.Fatal("message is invalid")
	}
	if message.IdentityKey() == "" {
		t.Fatal("identity key is empty")
	}
	position, exists := message.SourcePosition()
	if !exists || position.DiagnosticKey() != "workflow-node-results:2:42" {
		t.Fatalf("position = %#v, exists = %v", position, exists)
	}
}

func TestInboxIdentityUsesConsumerAndMessageID(t *testing.T) {
	first, err := NewInboxIdentityKey("consumer-a", "message-1")
	if err != nil {
		t.Fatalf("first key error = %v", err)
	}
	second, err := NewInboxIdentityKey("consumer-a", "message-1")
	if err != nil {
		t.Fatalf("second key error = %v", err)
	}
	otherConsumer, _ := NewInboxIdentityKey("consumer-b", "message-1")
	otherMessage, _ := NewInboxIdentityKey("consumer-a", "message-2")
	if first != second {
		t.Fatal("identity key is not deterministic")
	}
	if first == otherConsumer || first == otherMessage {
		t.Fatal("identity key does not distinguish consumer and message")
	}
}

func TestNewInboxMessageRejectsInvalidIdentityAndState(t *testing.T) {
	tests := map[string]func(*InboxMessageParams){
		"consumer":          func(p *InboxMessageParams) { p.ConsumerIdentity = "" },
		"message":           func(p *InboxMessageParams) { p.MessageID = "" },
		"company":           func(p *InboxMessageParams) { p.CompanyID = "" },
		"execution":         func(p *InboxMessageParams) { p.WorkflowExecutionID = "" },
		"message type":      func(p *InboxMessageParams) { p.MessageType = "" },
		"message version":   func(p *InboxMessageParams) { p.MessageVersion = 0 },
		"processing result": func(p *InboxMessageParams) { p.ProcessingResult = "UNKNOWN" },
		"received time":     func(p *InboxMessageParams) { p.ReceivedAt = time.Time{} },
		"processed time":    func(p *InboxMessageParams) { p.ProcessedAt = p.ReceivedAt.Add(-time.Second) },
		"source position": func(p *InboxMessageParams) {
			p.SourcePosition = &InboxSourcePosition{}
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params := validInboxParams()
			mutate(&params)
			if _, err := NewInboxMessage(params); err == nil {
				t.Fatal("NewInboxMessage() returned nil error")
			}
		})
	}
}

func TestInboxSourcePositionRejectsNegativeValues(t *testing.T) {
	if _, err := NewInboxSourcePosition("source", -1, 0); err == nil {
		t.Fatal("negative partition accepted")
	}
	if _, err := NewInboxSourcePosition("source", 0, -1); err == nil {
		t.Fatal("negative offset accepted")
	}
	if (InboxSourcePosition{}).IsValid() {
		t.Fatal("zero source position is valid")
	}
}

func TestInboxDiagnosticPositionDoesNotAffectIdempotency(t *testing.T) {
	firstParams := validInboxParams()
	first, err := NewInboxMessage(firstParams)
	if err != nil {
		t.Fatalf("first message error = %v", err)
	}
	position, _ := NewInboxSourcePosition("another-source", 9, 999)
	secondParams := validInboxParams()
	secondParams.SourcePosition = &position
	second, err := NewInboxMessage(secondParams)
	if err != nil {
		t.Fatalf("second message error = %v", err)
	}
	if first.IdentityKey() != second.IdentityKey() {
		t.Fatal("diagnostic source position changed MessageID idempotency identity")
	}
}

func TestInboxZeroValueIsInvalid(t *testing.T) {
	if (InboxMessage{}).IsValid() {
		t.Fatal("zero InboxMessage is valid")
	}
	if (InboxProcessingResult("UNKNOWN")).IsValid() {
		t.Fatal("unsupported processing result is valid")
	}
}

func TestNewJSONObjectAcceptsObject(
	t *testing.T,
) {
	object, err := NewJSONObject(
		[]byte(`  {"nodes":[],"edges":[]}  `),
	)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	expected := `{"nodes":[],"edges":[]}`

	if actual := object.String(); actual != expected {
		t.Fatalf(
			"String() = %q, want %q",
			actual,
			expected,
		)
	}

	if !object.IsValid() {
		t.Fatal(
			"valid JSONObject was reported as invalid",
		)
	}
}

func TestNewJSONObjectUsesEmptyObjectForBlankInput(
	t *testing.T,
) {
	tests := map[string][]byte{
		"nil":        nil,
		"empty":      {},
		"whitespace": []byte(" \n\t "),
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			object, err := NewJSONObject(value)
			if err != nil {
				t.Fatalf(
					"NewJSONObject() returned an unexpected error: %v",
					err,
				)
			}

			if actual := object.String(); actual != "{}" {
				t.Fatalf(
					"String() = %q, want %q",
					actual,
					"{}",
				)
			}
		})
	}
}

func TestNewJSONObjectRejectsInvalidOrNonObjectValues(
	t *testing.T,
) {
	tests := map[string][]byte{
		"invalid JSON": []byte(`{"nodes":`),
		"array":        []byte(`[]`),
		"string":       []byte(`"workflow"`),
		"number":       []byte(`42`),
		"boolean":      []byte(`true`),
		"null":         []byte(`null`),
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewJSONObject(value)
			if err == nil {
				t.Fatal(
					"NewJSONObject() returned nil error for an invalid value",
				)
			}

			var validationError *ValidationError
			if !errors.As(
				err,
				&validationError,
			) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != "jsonObject" {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					"jsonObject",
				)
			}
		})
	}
}

func TestNewJSONObjectDefensivelyCopiesInput(
	t *testing.T,
) {
	source := []byte(`{"enabled":true}`)
	expected := bytes.Clone(source)

	object, err := NewJSONObject(source)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	source[2] = 'X'

	if actual := object.Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"stored bytes = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestJSONObjectBytesReturnsDefensiveCopy(
	t *testing.T,
) {
	object, err := NewJSONObject(
		[]byte(`{"enabled":true}`),
	)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	first := object.Bytes()
	first[2] = 'X'

	second := object.Bytes()
	expected := []byte(`{"enabled":true}`)

	if !bytes.Equal(
		second,
		expected,
	) {
		t.Fatalf(
			"stored bytes = %q, want %q",
			second,
			expected,
		)
	}
}

func TestJSONObjectZeroValueBehavesAsEmptyObject(
	t *testing.T,
) {
	var object JSONObject

	if actual := object.String(); actual != "{}" {
		t.Fatalf(
			"String() = %q, want %q",
			actual,
			"{}",
		)
	}

	if actual := object.Bytes(); !bytes.Equal(
		actual,
		[]byte(`{}`),
	) {
		t.Fatalf(
			"Bytes() = %q, want %q",
			actual,
			[]byte(`{}`),
		)
	}

	if !object.IsValid() {
		t.Fatal(
			"zero-value JSONObject was reported as invalid",
		)
	}
}

type completeExecutionLifecycleStoreStub struct{}

var _ ExecutionLifecycleStore = (*completeExecutionLifecycleStoreStub)(nil)

func (store *completeExecutionLifecycleStoreStub) IsValid() bool {
	return store != nil
}

func (
	*completeExecutionLifecycleStoreStub,
) CreateExecution(
	context.Context,
	CreateExecutionCommand,
) error {
	return nil
}

func (
	*completeExecutionLifecycleStoreStub,
) CreateNodeExecutions(
	context.Context,
	CreateNodeExecutionsCommand,
) error {
	return nil
}

func (
	*completeExecutionLifecycleStoreStub,
) ApplyWorkflowTransition(
	context.Context,
	WorkflowTransitionCommand,
) error {
	return nil
}

func (
	*completeExecutionLifecycleStoreStub,
) ApplyNodeTransition(
	context.Context,
	NodeTransitionCommand,
) error {
	return nil
}

func TestNewNodeExecutionRecordStoresNormalizedValues(
	t *testing.T,
) {
	params := validNodeExecutionRecordParams()

	params.ID = execution.NodeExecutionID(" node-execution-1 ")
	params.WorkflowExecutionID =
		execution.WorkflowExecutionID(" workflow-execution-1 ")
	params.CompanyID = workflow.CompanyID(" company-1 ")
	params.NodeID = workflow.NodeID(" node-1 ")
	params.PluginType = workflow.PluginType(" core.pass-through ")
	params.PluginVersion = workflow.PluginVersion(" v1 ")
	params.LockVersion = 4

	record, err := NewNodeExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	if actual := record.ID().String(); actual != "node-execution-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"node-execution-1",
		)
	}

	if actual := record.WorkflowExecutionID().String(); actual !=
		"workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q",
			actual,
		)
	}

	if actual := record.CompanyID().String(); actual != "company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := record.NodeID().String(); actual != "node-1" {
		t.Fatalf(
			"NodeID() = %q, want %q",
			actual,
			"node-1",
		)
	}

	if actual := record.PluginType().String(); actual !=
		"core.pass-through" {
		t.Fatalf(
			"PluginType() = %q",
			actual,
		)
	}

	if actual := record.PluginVersion().String(); actual != "v1" {
		t.Fatalf(
			"PluginVersion() = %q, want %q",
			actual,
			"v1",
		)
	}

	if record.Status() != execution.NodeExecutionStatusSucceeded {
		t.Fatalf(
			"Status() = %q, want %q",
			record.Status(),
			execution.NodeExecutionStatusSucceeded,
		)
	}

	if record.Attempt() != 1 {
		t.Fatalf(
			"Attempt() = %d, want 1",
			record.Attempt(),
		)
	}

	if record.CreatedAt().Location() != time.UTC {
		t.Fatalf(
			"CreatedAt() location = %s, want UTC",
			record.CreatedAt().Location(),
		)
	}

	if record.LockVersion() != 4 {
		t.Fatalf(
			"LockVersion() = %d, want 4",
			record.LockVersion(),
		)
	}

	if !record.IsValid() {
		t.Fatal(
			"valid NodeExecutionRecord was reported as invalid",
		)
	}
}

func TestNewNodeExecutionRecordAllowsAbsentOptionalFields(
	t *testing.T,
) {
	params := validNodeExecutionRecordParams()

	params.Status = execution.NodeExecutionStatusPending
	params.ReadyAt = time.Time{}
	params.QueuedAt = time.Time{}
	params.StartedAt = time.Time{}
	params.FinishedAt = time.Time{}
	params.InputSummary = nil
	params.OutputSummary = nil
	params.FailureSummary = nil

	record, err := NewNodeExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	if _, exists := record.ReadyAt(); exists {
		t.Fatal(
			"ReadyAt() reported a value for a pending node",
		)
	}

	if _, exists := record.StartedAt(); exists {
		t.Fatal(
			"StartedAt() reported a value for a pending node",
		)
	}

	if _, exists := record.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() reported a value for a pending node",
		)
	}

	if _, exists := record.InputSummary(); exists {
		t.Fatal(
			"InputSummary() reported an absent summary",
		)
	}

	if _, exists := record.OutputSummary(); exists {
		t.Fatal(
			"OutputSummary() reported an absent summary",
		)
	}

	if _, exists := record.FailureSummary(); exists {
		t.Fatal(
			"FailureSummary() reported an absent summary",
		)
	}
}

func TestNewNodeExecutionRecordRejectsInvalidFields(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(*NodeExecutionRecordParams)
	}{
		{
			name:  "blank node execution ID",
			field: "nodeExecutionID",
			mutate: func(params *NodeExecutionRecordParams) {
				params.ID = execution.NodeExecutionID(" ")
			},
		},
		{
			name:  "blank workflow execution ID",
			field: "workflowExecutionID",
			mutate: func(params *NodeExecutionRecordParams) {
				params.WorkflowExecutionID =
					execution.WorkflowExecutionID(" ")
			},
		},
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(params *NodeExecutionRecordParams) {
				params.CompanyID = workflow.CompanyID(" ")
			},
		},
		{
			name:  "blank node ID",
			field: "nodeID",
			mutate: func(params *NodeExecutionRecordParams) {
				params.NodeID = workflow.NodeID(" ")
			},
		},
		{
			name:  "blank plugin type",
			field: "pluginType",
			mutate: func(params *NodeExecutionRecordParams) {
				params.PluginType = workflow.PluginType(" ")
			},
		},
		{
			name:  "blank plugin version",
			field: "pluginVersion",
			mutate: func(params *NodeExecutionRecordParams) {
				params.PluginVersion = workflow.PluginVersion(" ")
			},
		},
		{
			name:  "invalid status",
			field: "status",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Status =
					execution.NodeExecutionStatus("UNKNOWN")
			},
		},
		{
			name:  "zero attempt",
			field: "attempt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Attempt = 0
			},
		},
		{
			name:  "zero created time",
			field: "createdAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.CreatedAt = time.Time{}
			},
		},
		{
			name:  "zero updated time",
			field: "updatedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.UpdatedAt = time.Time{}
			},
		},
		{
			name:  "ready before creation",
			field: "readyAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.ReadyAt =
					params.CreatedAt.Add(-time.Second)
			},
		},
		{
			name:  "queued before ready",
			field: "queuedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.QueuedAt =
					params.ReadyAt.Add(-time.Second)
			},
		},
		{
			name:  "started before queued",
			field: "startedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.StartedAt =
					params.QueuedAt.Add(-time.Second)
			},
		},
		{
			name:  "finished before started",
			field: "finishedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.FinishedAt =
					params.StartedAt.Add(-time.Second)
			},
		},
		{
			name:  "updated before transition",
			field: "updatedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.UpdatedAt =
					params.FinishedAt.Add(-time.Second)
			},
		},
		{
			name:  "ready status without ready time",
			field: "readyAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Status = execution.NodeExecutionStatusReady
				params.ReadyAt = time.Time{}
				params.QueuedAt = time.Time{}
				params.StartedAt = time.Time{}
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "queued status without queued time",
			field: "queuedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Status = execution.NodeExecutionStatusQueued
				params.QueuedAt = time.Time{}
				params.StartedAt = time.Time{}
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "running status without started time",
			field: "startedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Status = execution.NodeExecutionStatusRunning
				params.StartedAt = time.Time{}
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "succeeded status without started time",
			field: "startedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Status = execution.NodeExecutionStatusSucceeded
				params.StartedAt = time.Time{}
			},
		},
		{
			name:  "terminal status without finished time",
			field: "finishedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Status = execution.NodeExecutionStatusFailed
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "non-terminal status with finished time",
			field: "finishedAt",
			mutate: func(params *NodeExecutionRecordParams) {
				params.Status = execution.NodeExecutionStatusRunning
			},
		},
		{
			name:  "invalid input summary",
			field: "inputSummary",
			mutate: func(params *NodeExecutionRecordParams) {
				params.InputSummary = []byte(`[]`)
			},
		},
		{
			name:  "invalid output summary",
			field: "outputSummary",
			mutate: func(params *NodeExecutionRecordParams) {
				params.OutputSummary = []byte(`{"size":`)
			},
		},
		{
			name:  "invalid failure summary",
			field: "failureSummary",
			mutate: func(params *NodeExecutionRecordParams) {
				params.FailureSummary = []byte(`"failure"`)
			},
		},
		{
			name:  "negative lock version",
			field: "lockVersion",
			mutate: func(params *NodeExecutionRecordParams) {
				params.LockVersion = -1
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validNodeExecutionRecordParams()
			test.mutate(&params)

			_, err := NewNodeExecutionRecord(params)
			if err == nil {
				t.Fatal(
					"NewNodeExecutionRecord() returned nil error for invalid parameters",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNodeExecutionRecordDefensivelyCopiesSummaryInputs(
	t *testing.T,
) {
	inputSummary := []byte(`{"size":10}`)
	outputSummary := []byte(`{"size":8}`)
	failureSummary := []byte(`{"code":"NODE_FAILED"}`)

	params := validNodeExecutionRecordParams()
	params.InputSummary = inputSummary
	params.OutputSummary = outputSummary
	params.FailureSummary = failureSummary

	record, err := NewNodeExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	expectedInput := bytes.Clone(inputSummary)
	expectedOutput := bytes.Clone(outputSummary)
	expectedFailure := bytes.Clone(failureSummary)

	inputSummary[2] = 'X'
	outputSummary[2] = 'X'
	failureSummary[2] = 'X'

	assertNodeSummary(
		t,
		record.InputSummary,
		expectedInput,
	)

	assertNodeSummary(
		t,
		record.OutputSummary,
		expectedOutput,
	)

	assertNodeSummary(
		t,
		record.FailureSummary,
		expectedFailure,
	)
}

func TestNodeExecutionRecordSummaryGettersReturnDefensiveCopies(
	t *testing.T,
) {
	record := newTestNodeExecutionRecord(t)

	input, exists := record.InputSummary()
	if !exists {
		t.Fatal(
			"InputSummary() reported no value",
		)
	}

	inputBytes := input.Bytes()
	inputBytes[2] = 'X'

	secondInput, exists := record.InputSummary()
	if !exists {
		t.Fatal(
			"InputSummary() reported no value after mutation attempt",
		)
	}

	if actual := secondInput.String(); actual != `{"size":10}` {
		t.Fatalf(
			"InputSummary() = %q",
			actual,
		)
	}
}

func TestNodeExecutionRecordZeroValueIsInvalid(
	t *testing.T,
) {
	var record NodeExecutionRecord

	if record.IsValid() {
		t.Fatal(
			"zero-value NodeExecutionRecord was reported as valid",
		)
	}
}

func assertNodeSummary(
	t *testing.T,
	getter func() (JSONObject, bool),
	expected []byte,
) {
	t.Helper()

	actual, exists := getter()
	if !exists {
		t.Fatal(
			"summary getter reported no value",
		)
	}

	if !bytes.Equal(
		actual.Bytes(),
		expected,
	) {
		t.Fatalf(
			"summary = %q, want %q",
			actual.Bytes(),
			expected,
		)
	}
}

func newTestNodeExecutionRecord(
	t *testing.T,
) NodeExecutionRecord {
	t.Helper()

	record, err := NewNodeExecutionRecord(
		validNodeExecutionRecordParams(),
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func validNodeExecutionRecordParams() NodeExecutionRecordParams {
	createdAt := nodeExecutionRecordTestTime(10, 0)

	return NodeExecutionRecordParams{
		ID: execution.NodeExecutionID(
			"node-execution-1",
		),
		WorkflowExecutionID: execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		CompanyID: workflow.CompanyID(
			"company-1",
		),
		NodeID: workflow.NodeID(
			"node-1",
		),
		PluginType: workflow.PluginType(
			"core.pass-through",
		),
		PluginVersion: workflow.PluginVersion(
			"v1",
		),
		Status:    execution.NodeExecutionStatusSucceeded,
		Attempt:   1,
		CreatedAt: createdAt,
		ReadyAt: createdAt.Add(
			1 * time.Minute,
		),
		QueuedAt: createdAt.Add(
			2 * time.Minute,
		),
		StartedAt: createdAt.Add(
			3 * time.Minute,
		),
		FinishedAt: createdAt.Add(
			4 * time.Minute,
		),
		UpdatedAt: createdAt.Add(
			4 * time.Minute,
		),
		InputSummary: []byte(
			`{"size":10}`,
		),
		OutputSummary: []byte(
			`{"size":8}`,
		),
		FailureSummary: []byte(
			`{"code":"NODE_FAILED"}`,
		),
		LockVersion: 0,
	}
}

func nodeExecutionRecordTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		17,
		hour,
		minute,
		0,
		0,
		time.FixedZone(
			"TRT",
			3*60*60,
		),
	)
}

type nodeTransitionStoreContractStub struct{}

var _ NodeTransitionStore = (*nodeTransitionStoreContractStub)(nil)

func (
	*nodeTransitionStoreContractStub,
) ApplyNodeTransition(
	context.Context,
	NodeTransitionCommand,
) error {
	return nil
}

func TestNewNodeTransitionCommandStoresReadyTransition(
	t *testing.T,
) {
	params :=
		validReadyNodeTransitionCommandParams(t)

	command, err :=
		NewNodeTransitionCommand(params)
	if err != nil {
		t.Fatalf(
			"NewNodeTransitionCommand() returned an unexpected error: %v",
			err,
		)
	}

	if command.CompanyID().String() !=
		"company-1" {
		t.Fatalf(
			"CompanyID() = %q",
			command.CompanyID(),
		)
	}

	if command.WorkflowExecutionID().String() !=
		"workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q",
			command.WorkflowExecutionID(),
		)
	}

	if command.NodeExecutionID().String() !=
		"node-execution-1" {
		t.Fatalf(
			"NodeExecutionID() = %q",
			command.NodeExecutionID(),
		)
	}

	if command.ExpectedWorkflowStatus() !=
		execution.WorkflowExecutionStatusRunning {
		t.Fatalf(
			"ExpectedWorkflowStatus() = %q",
			command.ExpectedWorkflowStatus(),
		)
	}

	if command.ExpectedNodeStatus() !=
		execution.NodeExecutionStatusPending {
		t.Fatalf(
			"ExpectedNodeStatus() = %q",
			command.ExpectedNodeStatus(),
		)
	}

	if command.NodeExecution().Status() !=
		execution.NodeExecutionStatusReady {
		t.Fatalf(
			"target status = %q",
			command.NodeExecution().Status(),
		)
	}

	if actual := len(command.Timeline()); actual != 2 {
		t.Fatalf(
			"timeline count = %d, want 2",
			actual,
		)
	}

	if actual := len(command.Errors()); actual != 0 {
		t.Fatalf(
			"error count = %d, want 0",
			actual,
		)
	}

	if !command.IsValid() {
		t.Fatal(
			"valid NodeTransitionCommand was reported as invalid",
		)
	}
}

func TestNewNodeTransitionCommandStoresFailedTransition(
	t *testing.T,
) {
	params :=
		validFailedNodeTransitionCommandParams(t)

	command, err :=
		NewNodeTransitionCommand(params)
	if err != nil {
		t.Fatalf(
			"NewNodeTransitionCommand() returned an unexpected error: %v",
			err,
		)
	}

	if command.ExpectedNodeStatus() !=
		execution.NodeExecutionStatusRunning {
		t.Fatalf(
			"ExpectedNodeStatus() = %q, want RUNNING",
			command.ExpectedNodeStatus(),
		)
	}

	if command.NodeExecution().Status() !=
		execution.NodeExecutionStatusFailed {
		t.Fatalf(
			"target status = %q, want FAILED",
			command.NodeExecution().Status(),
		)
	}

	if actual := len(command.Errors()); actual != 1 {
		t.Fatalf(
			"error count = %d, want 1",
			actual,
		)
	}
}

func TestNewNodeTransitionCommandRejectsInvalidCoreState(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(
			*NodeTransitionCommandParams,
			*testing.T,
		)
	}{
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.CompanyID =
					workflow.CompanyID(" ")
			},
		},
		{
			name:  "workflow execution ID mismatch",
			field: "nodeExecution",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.WorkflowExecutionID =
					execution.WorkflowExecutionID(
						"workflow-execution-2",
					)
			},
		},
		{
			name:  "node execution ID mismatch",
			field: "nodeExecution",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.NodeExecutionID =
					execution.NodeExecutionID(
						"node-execution-2",
					)
			},
		},
		{
			name:  "workflow is not running",
			field: "expectedWorkflowStatus",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedWorkflowStatus =
					execution.WorkflowExecutionStatusValidating
			},
		},
		{
			name:  "negative workflow lock version",
			field: "expectedWorkflowLockVersion",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedWorkflowLockVersion = -1
			},
		},
		{
			name:  "invalid expected sequence",
			field: "expectedNextSequenceNumber",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedNextSequenceNumber = 0
			},
		},
		{
			name:  "invalid expected node status",
			field: "expectedNodeStatus",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedNodeStatus =
					execution.NodeExecutionStatus(
						"UNKNOWN",
					)
			},
		},
		{
			name:  "illegal node transition",
			field: "expectedNodeStatus",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedNodeStatus =
					execution.NodeExecutionStatusRunning
			},
		},
		{
			name:  "negative node lock version",
			field: "expectedNodeLockVersion",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedNodeLockVersion = -1
			},
		},
		{
			name:  "node lock version did not increment",
			field: "nodeExecution",
			mutate: func(
				params *NodeTransitionCommandParams,
				t *testing.T,
			) {
				params.NodeExecution =
					newReadyNodeTransitionRecord(
						t,
						3,
						nil,
					)
			},
		},
		{
			name:  "invalid updated node record",
			field: "nodeExecution",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.NodeExecution =
					NodeExecutionRecord{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validReadyNodeTransitionCommandParams(t)

			test.mutate(&params, t)

			_, err :=
				NewNodeTransitionCommand(params)
			if err == nil {
				t.Fatal(
					"NewNodeTransitionCommand() returned nil error",
				)
			}

			requireNodeTransitionValidationField(
				t,
				err,
				test.field,
			)
		})
	}
}

func TestNewNodeTransitionCommandRejectsInvalidTimeline(
	t *testing.T,
) {
	tests := []struct {
		name   string
		mutate func(
			*NodeTransitionCommandParams,
			*testing.T,
		)
	}{
		{
			name: "empty timeline",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.Timeline = nil
			},
		},
		{
			name: "first entry is a log",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.Timeline[0] =
					params.Timeline[1]
			},
		},
		{
			name: "wrong transition event type",
			mutate: func(
				params *NodeTransitionCommandParams,
				t *testing.T,
			) {
				record := params.NodeExecution

				params.Timeline[0] =
					newNodeTransitionEventEntry(
						t,
						record,
						params.ExpectedNodeStatus,
						ExecutionEventTypeNodeStarted,
						ExecutionEventID(
							"event-wrong-type",
						),
						record.UpdatedAt(),
					)
			},
		},
		{
			name: "event references different node",
			mutate: func(
				params *NodeTransitionCommandParams,
				t *testing.T,
			) {
				record := params.NodeExecution

				params.Timeline[0] =
					newNodeTransitionEventEntryWithNodeID(
						t,
						record,
						execution.NodeExecutionID(
							"node-execution-2",
						),
						params.ExpectedNodeStatus,
						ExecutionEventTypeNodeReady,
						ExecutionEventID(
							"event-wrong-node",
						),
						record.UpdatedAt(),
					)
			},
		},
		{
			name: "workflow scoped node log",
			mutate: func(
				params *NodeTransitionCommandParams,
				t *testing.T,
			) {
				params.Timeline[1] =
					newNodeTransitionLogEntry(
						t,
						params.NodeExecution,
						ExecutionLogID(
							"log-workflow-scoped",
						),
						"",
						params.NodeExecution.
							UpdatedAt(),
					)
			},
		},
		{
			name: "duplicate log ID",
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.Timeline = append(
					params.Timeline,
					params.Timeline[1],
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validReadyNodeTransitionCommandParams(t)

			test.mutate(&params, t)

			_, err :=
				NewNodeTransitionCommand(params)
			if err == nil {
				t.Fatal(
					"NewNodeTransitionCommand() returned nil error",
				)
			}

			requireNodeTransitionValidationField(
				t,
				err,
				"timeline",
			)
		})
	}
}

func TestNewNodeTransitionCommandRejectsInvalidErrors(
	t *testing.T,
) {
	tests := []struct {
		name   string
		params func(
			*testing.T,
		) NodeTransitionCommandParams
		mutate func(
			*NodeTransitionCommandParams,
			*testing.T,
		)
	}{
		{
			name:   "failed transition without structured error",
			params: validFailedNodeTransitionCommandParams,
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.Errors = nil
			},
		},
		{
			name:   "nonfailure transition with structured error",
			params: validReadyNodeTransitionCommandParams,
			mutate: func(
				params *NodeTransitionCommandParams,
				t *testing.T,
			) {
				event, _ :=
					params.Timeline[0].Event()

				params.Errors =
					[]ExecutionErrorRecord{
						newNodeTransitionError(
							t,
							params.NodeExecution,
							event.ID(),
							nil,
						),
					}
			},
		},
		{
			name:   "error without related event",
			params: validFailedNodeTransitionCommandParams,
			mutate: func(
				params *NodeTransitionCommandParams,
				t *testing.T,
			) {
				params.Errors[0] =
					newNodeTransitionError(
						t,
						params.NodeExecution,
						"",
						nil,
					)
			},
		},
		{
			name:   "error references different node",
			params: validFailedNodeTransitionCommandParams,
			mutate: func(
				params *NodeTransitionCommandParams,
				t *testing.T,
			) {
				event, _ :=
					params.Timeline[0].Event()

				params.Errors[0] =
					newNodeTransitionError(
						t,
						params.NodeExecution,
						event.ID(),
						func(
							errorParams *ExecutionErrorRecordParams,
						) {
							errorParams.NodeExecutionID =
								execution.NodeExecutionID(
									"node-execution-2",
								)
						},
					)
			},
		},
		{
			name:   "duplicate error ID",
			params: validFailedNodeTransitionCommandParams,
			mutate: func(
				params *NodeTransitionCommandParams,
				_ *testing.T,
			) {
				params.Errors = append(
					params.Errors,
					params.Errors[0],
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := test.params(t)
			test.mutate(&params, t)

			_, err :=
				NewNodeTransitionCommand(params)
			if err == nil {
				t.Fatal(
					"NewNodeTransitionCommand() returned nil error",
				)
			}

			requireNodeTransitionValidationField(
				t,
				err,
				"errors",
			)
		})
	}
}

func TestNodeTransitionCommandReturnsDefensiveSliceCopies(
	t *testing.T,
) {
	command, err :=
		NewNodeTransitionCommand(
			validFailedNodeTransitionCommandParams(t),
		)
	if err != nil {
		t.Fatalf(
			"NewNodeTransitionCommand() returned an unexpected error: %v",
			err,
		)
	}

	timeline := command.Timeline()
	timeline[0] = TimelineEntry{}

	if !command.Timeline()[0].IsValid() {
		t.Fatal(
			"Timeline() allowed command mutation",
		)
	}

	executionErrors := command.Errors()
	executionErrors[0] = ExecutionErrorRecord{}

	if !command.Errors()[0].IsValid() {
		t.Fatal(
			"Errors() allowed command mutation",
		)
	}
}

func TestNodeTransitionCommandZeroValueIsInvalid(
	t *testing.T,
) {
	var command NodeTransitionCommand

	if command.IsValid() {
		t.Fatal(
			"zero-value NodeTransitionCommand was reported as valid",
		)
	}
}

func validReadyNodeTransitionCommandParams(
	t *testing.T,
) NodeTransitionCommandParams {
	t.Helper()

	record :=
		newReadyNodeTransitionRecord(
			t,
			4,
			nil,
		)

	return NodeTransitionCommandParams{
		CompanyID:                   record.CompanyID(),
		WorkflowExecutionID:         record.WorkflowExecutionID(),
		NodeExecutionID:             record.ID(),
		ExpectedWorkflowStatus:      execution.WorkflowExecutionStatusRunning,
		ExpectedWorkflowLockVersion: 8,
		ExpectedNextSequenceNumber:  SequenceNumber(30),
		ExpectedNodeStatus:          execution.NodeExecutionStatusPending,
		ExpectedNodeLockVersion:     3,
		NodeExecution:               record,
		Timeline: newNodeTransitionTimeline(
			t,
			record,
			execution.NodeExecutionStatusPending,
			ExecutionEventTypeNodeReady,
			ExecutionEventID(
				"event-node-ready",
			),
			ExecutionLogID(
				"log-node-ready",
			),
		),
	}
}

func validFailedNodeTransitionCommandParams(
	t *testing.T,
) NodeTransitionCommandParams {
	t.Helper()

	record :=
		newFailedNodeTransitionRecord(
			t,
			6,
			nil,
		)

	timeline :=
		newNodeTransitionTimeline(
			t,
			record,
			execution.NodeExecutionStatusRunning,
			ExecutionEventTypeNodeFailed,
			ExecutionEventID(
				"event-node-failed",
			),
			ExecutionLogID(
				"log-node-failed",
			),
		)

	event, _ := timeline[0].Event()

	return NodeTransitionCommandParams{
		CompanyID:                   record.CompanyID(),
		WorkflowExecutionID:         record.WorkflowExecutionID(),
		NodeExecutionID:             record.ID(),
		ExpectedWorkflowStatus:      execution.WorkflowExecutionStatusRunning,
		ExpectedWorkflowLockVersion: 12,
		ExpectedNextSequenceNumber:  SequenceNumber(50),
		ExpectedNodeStatus:          execution.NodeExecutionStatusRunning,
		ExpectedNodeLockVersion:     5,
		NodeExecution:               record,
		Timeline:                    timeline,
		Errors: []ExecutionErrorRecord{
			newNodeTransitionError(
				t,
				record,
				event.ID(),
				nil,
			),
		},
	}
}

func newReadyNodeTransitionRecord(
	t *testing.T,
	lockVersion int64,
	mutate func(
		*NodeExecutionRecordParams,
	),
) NodeExecutionRecord {
	t.Helper()

	params :=
		validNodeExecutionRecordParams()

	params.Status =
		execution.NodeExecutionStatusReady

	params.ReadyAt =
		params.CreatedAt.Add(time.Minute)

	params.QueuedAt = time.Time{}
	params.StartedAt = time.Time{}
	params.FinishedAt = time.Time{}

	params.UpdatedAt =
		params.ReadyAt

	params.InputSummary = nil
	params.OutputSummary = nil
	params.FailureSummary = nil
	params.LockVersion = lockVersion

	if mutate != nil {
		mutate(&params)
	}

	record, err :=
		NewNodeExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func newFailedNodeTransitionRecord(
	t *testing.T,
	lockVersion int64,
	mutate func(
		*NodeExecutionRecordParams,
	),
) NodeExecutionRecord {
	t.Helper()

	params :=
		validNodeExecutionRecordParams()

	params.Status =
		execution.NodeExecutionStatusFailed

	params.ReadyAt =
		params.CreatedAt.Add(time.Minute)

	params.QueuedAt = time.Time{}

	params.StartedAt =
		params.CreatedAt.Add(
			2 * time.Minute,
		)

	params.FinishedAt =
		params.CreatedAt.Add(
			3 * time.Minute,
		)

	params.UpdatedAt =
		params.FinishedAt

	params.InputSummary =
		[]byte(`{"size":10}`)

	params.OutputSummary = nil

	params.FailureSummary =
		[]byte(
			`{"code":"NODE_EXECUTION_FAILED"}`,
		)

	params.LockVersion = lockVersion

	if mutate != nil {
		mutate(&params)
	}

	record, err :=
		NewNodeExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewNodeExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func newNodeTransitionTimeline(
	t *testing.T,
	record NodeExecutionRecord,
	previousStatus execution.NodeExecutionStatus,
	eventType ExecutionEventType,
	eventID ExecutionEventID,
	logID ExecutionLogID,
) []TimelineEntry {
	t.Helper()

	transitionAt :=
		record.UpdatedAt()

	return []TimelineEntry{
		newNodeTransitionEventEntry(
			t,
			record,
			previousStatus,
			eventType,
			eventID,
			transitionAt,
		),
		newNodeTransitionLogEntry(
			t,
			record,
			logID,
			record.ID(),
			transitionAt,
		),
	}
}

func newNodeTransitionEventEntry(
	t *testing.T,
	record NodeExecutionRecord,
	previousStatus execution.NodeExecutionStatus,
	eventType ExecutionEventType,
	eventID ExecutionEventID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	return newNodeTransitionEventEntryWithNodeID(
		t,
		record,
		record.ID(),
		previousStatus,
		eventType,
		eventID,
		createdAt,
	)
}

func newNodeTransitionEventEntryWithNodeID(
	t *testing.T,
	record NodeExecutionRecord,
	nodeExecutionID execution.NodeExecutionID,
	previousStatus execution.NodeExecutionStatus,
	eventType ExecutionEventType,
	eventID ExecutionEventID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	eventDraft, err :=
		NewExecutionEventDraft(
			ExecutionEventDraftParams{
				ID:                  eventID,
				WorkflowExecutionID: record.WorkflowExecutionID(),
				CompanyID:           record.CompanyID(),
				NodeExecutionID:     nodeExecutionID,
				Type:                eventType,
				PreviousStatus:      previousStatus.String(),
				NewStatus:           record.Status().String(),
				SafeMessage:         "Node execution status changed",
				CreatedAt:           createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventDraft() returned an unexpected error: %v",
			err,
		)
	}

	entry, err :=
		NewEventTimelineEntry(eventDraft)
	if err != nil {
		t.Fatalf(
			"NewEventTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	return entry
}

func newNodeTransitionLogEntry(
	t *testing.T,
	record NodeExecutionRecord,
	logID ExecutionLogID,
	nodeExecutionID execution.NodeExecutionID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	logDraft, err :=
		NewExecutionLogDraft(
			ExecutionLogDraftParams{
				ID:                  logID,
				WorkflowExecutionID: record.WorkflowExecutionID(),
				CompanyID:           record.CompanyID(),
				NodeExecutionID:     nodeExecutionID,
				Level:               ExecutionLogLevelInfo,
				Message:             "Node execution status changed",
				CreatedAt:           createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an unexpected error: %v",
			err,
		)
	}

	entry, err :=
		NewLogTimelineEntry(logDraft)
	if err != nil {
		t.Fatalf(
			"NewLogTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	return entry
}

func newNodeTransitionError(
	t *testing.T,
	record NodeExecutionRecord,
	relatedEventID ExecutionEventID,
	mutate func(
		*ExecutionErrorRecordParams,
	),
) ExecutionErrorRecord {
	t.Helper()

	params := ExecutionErrorRecordParams{
		ID: ExecutionErrorID(
			"error-node-transition",
		),
		WorkflowExecutionID: record.WorkflowExecutionID(),
		CompanyID:           record.CompanyID(),
		NodeExecutionID:     record.ID(),
		RelatedEventID:      relatedEventID,
		Category:            runtime.FailureCategoryExecution,
		Code:                "NODE_EXECUTION_FAILED",
		SafeMessage:         "Node execution failed",
		TechnicalDetail:     "node executor returned a failure",
		Retryable:           false,
		Details: []byte(
			`{"scope":"node"}`,
		),
		CreatedAt: record.UpdatedAt(),
	}

	if mutate != nil {
		mutate(&params)
	}

	executionError, err :=
		NewExecutionErrorRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an unexpected error: %v",
			err,
		)
	}

	return executionError
}

func requireNodeTransitionValidationField(
	t *testing.T,
	err error,
	expectedField string,
) {
	t.Helper()

	var validationError *ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field !=
		expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}
}

var asyncTestTime = time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

func validOutboxParams() OutboxMessageParams {
	return OutboxMessageParams{
		MessageID:           MessageID("message-1"),
		CompanyID:           workflow.CompanyID("company-1"),
		WorkflowExecutionID: execution.WorkflowExecutionID("execution-1"),
		NodeExecutionID:     execution.NodeExecutionID("node-execution-1"),
		NodeID:              workflow.NodeID("node-1"),
		Attempt:             1,
		OperationKind:       OutboxOperationNodeCommand,
		MessageType:         "miletos.node-command",
		MessageVersion:      1,
		Destination:         "workflow-node-commands",
		MessageKey:          "execution-1:node-1",
		EncodedPayload:      []byte(`{"version":1}`),
		PublicationState:    OutboxPublicationPending,
		CreatedAt:           asyncTestTime,
	}
}

func TestNewOutboxMessageAcceptsValidPublicationStates(t *testing.T) {
	tests := map[string]func(*OutboxMessageParams){
		"pending": func(params *OutboxMessageParams) {},
		"publishing": func(params *OutboxMessageParams) {
			params.PublicationState = OutboxPublicationPublishing
			params.ClaimedAt = params.CreatedAt.Add(time.Second)
			params.ClaimOwner = "publisher-1"
			params.LockVersion = 1
		},
		"published": func(params *OutboxMessageParams) {
			params.PublicationState = OutboxPublicationPublished
			params.PublishedAt = params.CreatedAt.Add(2 * time.Second)
			params.LockVersion = 2
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params := validOutboxParams()
			mutate(&params)
			message, err := NewOutboxMessage(params)
			if err != nil {
				t.Fatalf("NewOutboxMessage() error = %v", err)
			}
			if !message.IsValid() {
				t.Fatal("message is invalid")
			}
			if message.PublicationState() != params.PublicationState {
				t.Fatalf("state = %q, want %q", message.PublicationState(), params.PublicationState)
			}
		})
	}
}

func TestNewOutboxMessageSupportsWorkflowScopedMessage(t *testing.T) {
	params := validOutboxParams()
	params.OperationKind = OutboxOperationWorkflowEvent
	params.OperationDiscriminator = "workflow-created"
	params.NodeExecutionID = ""
	params.NodeID = ""
	params.Attempt = 0

	message, err := NewOutboxMessage(params)
	if err != nil {
		t.Fatalf("NewOutboxMessage() error = %v", err)
	}
	if _, exists := message.NodeExecutionID(); exists {
		t.Fatal("workflow message unexpectedly has node identity")
	}
}

func TestNewOutboxMessageRejectsInvalidFields(t *testing.T) {
	tests := map[string]func(*OutboxMessageParams){
		"message id":      func(p *OutboxMessageParams) { p.MessageID = "" },
		"company id":      func(p *OutboxMessageParams) { p.CompanyID = "" },
		"execution id":    func(p *OutboxMessageParams) { p.WorkflowExecutionID = "" },
		"node id":         func(p *OutboxMessageParams) { p.NodeID = "" },
		"attempt":         func(p *OutboxMessageParams) { p.Attempt = 0 },
		"operation kind":  func(p *OutboxMessageParams) { p.OperationKind = "UNKNOWN" },
		"message type":    func(p *OutboxMessageParams) { p.MessageType = " " },
		"message version": func(p *OutboxMessageParams) { p.MessageVersion = 0 },
		"destination":     func(p *OutboxMessageParams) { p.Destination = " " },
		"message key":     func(p *OutboxMessageParams) { p.MessageKey = " " },
		"empty payload":   func(p *OutboxMessageParams) { p.EncodedPayload = nil },
		"oversized payload": func(p *OutboxMessageParams) {
			p.EncodedPayload = make([]byte, MaximumAsyncEncodedPayloadBytes+1)
		},
		"publication state": func(p *OutboxMessageParams) { p.PublicationState = "UNKNOWN" },
		"negative version":  func(p *OutboxMessageParams) { p.LockVersion = -1 },
		"pending claim": func(p *OutboxMessageParams) {
			p.ClaimedAt = p.CreatedAt
			p.ClaimOwner = "publisher"
		},
		"publishing without owner": func(p *OutboxMessageParams) {
			p.PublicationState = OutboxPublicationPublishing
			p.ClaimedAt = p.CreatedAt
		},
		"published without time": func(p *OutboxMessageParams) {
			p.PublicationState = OutboxPublicationPublished
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params := validOutboxParams()
			mutate(&params)
			if _, err := NewOutboxMessage(params); err == nil {
				t.Fatal("NewOutboxMessage() returned nil error")
			}
		})
	}
}

func TestOutboxMessageDefensivelyCopiesPayload(t *testing.T) {
	params := validOutboxParams()
	original := append([]byte(nil), params.EncodedPayload...)
	message, err := NewOutboxMessage(params)
	if err != nil {
		t.Fatalf("NewOutboxMessage() error = %v", err)
	}
	params.EncodedPayload[0] = 'X'
	actual := message.EncodedPayload()
	if !bytes.Equal(actual, original) {
		t.Fatalf("stored payload = %q, want %q", actual, original)
	}
	actual[0] = 'Y'
	if !bytes.Equal(message.EncodedPayload(), original) {
		t.Fatal("payload getter exposed mutable storage")
	}
}

func TestOutboxOperationKeyIsDeterministicAndAttemptScoped(t *testing.T) {
	first, err := NewOutboxMessage(validOutboxParams())
	if err != nil {
		t.Fatalf("first message error = %v", err)
	}
	secondParams := validOutboxParams()
	secondParams.MessageID = "message-2"
	second, err := NewOutboxMessage(secondParams)
	if err != nil {
		t.Fatalf("second message error = %v", err)
	}
	if first.OperationKey() != second.OperationKey() {
		t.Fatal("same logical command produced different operation keys")
	}

	secondParams.Attempt = 2
	third, err := NewOutboxMessage(secondParams)
	if err != nil {
		t.Fatalf("third message error = %v", err)
	}
	if first.OperationKey() == third.OperationKey() {
		t.Fatal("different attempt produced the same operation key")
	}
}

func TestWorkflowOutboxOperationDiscriminatorAccessorPreservesIdentity(t *testing.T) {
	params := validOutboxParams()
	params.OperationKind = OutboxOperationWorkflowEvent
	params.OperationDiscriminator = "  workflow-created  "
	params.NodeExecutionID = ""
	params.NodeID = ""
	params.Attempt = 0

	message, err := NewOutboxMessage(params)
	if err != nil {
		t.Fatalf("NewOutboxMessage() error = %v", err)
	}
	operationKey := message.OperationKey()

	discriminator, exists := message.OperationDiscriminator()
	if !exists {
		t.Fatal("OperationDiscriminator() exists = false, want true")
	}
	if discriminator != "workflow-created" {
		t.Fatalf(
			"OperationDiscriminator() = %q, want workflow-created",
			discriminator,
		)
	}
	if message.OperationKey() != operationKey {
		t.Fatal("OperationDiscriminator() changed the operation key")
	}

	secondDiscriminator, secondExists := message.OperationDiscriminator()
	if secondDiscriminator != discriminator || secondExists != exists {
		t.Fatal("OperationDiscriminator() changed model state between calls")
	}

	roundTripParams := params
	roundTripParams.OperationDiscriminator = discriminator
	roundTrip, err := NewOutboxMessage(roundTripParams)
	if err != nil {
		t.Fatalf("workflow-event round trip error = %v", err)
	}
	if roundTrip.OperationKey() != operationKey {
		t.Fatal("workflow-event round trip changed the operation key")
	}
}

func TestNodeOutboxOperationsDoNotExposeOrAcceptDiscriminator(t *testing.T) {
	for _, kind := range []OutboxOperationKind{
		OutboxOperationNodeCommand,
		OutboxOperationNodeResult,
	} {
		t.Run(kind.String(), func(t *testing.T) {
			params := validOutboxParams()
			params.OperationKind = kind
			message, err := NewOutboxMessage(params)
			if err != nil {
				t.Fatalf("NewOutboxMessage() error = %v", err)
			}
			if discriminator, exists := message.OperationDiscriminator(); discriminator != "" || exists {
				t.Fatalf(
					"OperationDiscriminator() = (%q, %t), want empty and false",
					discriminator,
					exists,
				)
			}

			params.OperationDiscriminator = "not-allowed"
			if _, err := NewOutboxMessage(params); err == nil {
				t.Fatal("node operation accepted a discriminator")
			}
		})
	}
}

func TestWorkflowOutboxOperationDiscriminatorValidation(t *testing.T) {
	for name, discriminator := range map[string]string{
		"blank":      " ",
		"over limit": strings.Repeat("x", maximumMessageKeyCharacters+1),
	} {
		t.Run(name, func(t *testing.T) {
			params := validOutboxParams()
			params.OperationKind = OutboxOperationWorkflowEvent
			params.OperationDiscriminator = discriminator
			params.NodeExecutionID = ""
			params.NodeID = ""
			params.Attempt = 0
			if _, err := NewOutboxMessage(params); err == nil {
				t.Fatal("workflow event accepted an invalid discriminator")
			}
		})
	}
}

func TestZeroOutboxMessageOperationDiscriminatorIsSafe(t *testing.T) {
	var message OutboxMessage
	if discriminator, exists := message.OperationDiscriminator(); discriminator != "" || exists {
		t.Fatalf(
			"zero OperationDiscriminator() = (%q, %t), want empty and false",
			discriminator,
			exists,
		)
	}
}

func TestOutboxZeroValuesAndBoundsAreInvalid(t *testing.T) {
	if (OutboxMessage{}).IsValid() {
		t.Fatal("zero OutboxMessage is valid")
	}
	if (OutboxPublicationState("UNKNOWN")).IsValid() {
		t.Fatal("unsupported publication state is valid")
	}
	params := validOutboxParams()
	params.Destination = strings.Repeat("x", maximumDestinationCharacters+1)
	if _, err := NewOutboxMessage(params); err == nil {
		t.Fatal("oversized destination accepted")
	}
}

func TestNewOutboxClaimRequestNormalizesValidValues(t *testing.T) {
	claimedAt := asyncTestTime.In(time.FixedZone("test-offset", 3*60*60))
	request, err := NewOutboxClaimRequest("  publisher-1  ", claimedAt, 25)
	if err != nil {
		t.Fatalf("NewOutboxClaimRequest() error = %v", err)
	}
	if request.ClaimOwner() != "publisher-1" {
		t.Fatalf("ClaimOwner() = %q, want publisher-1", request.ClaimOwner())
	}
	if !request.ClaimedAt().Equal(claimedAt) || request.ClaimedAt().Location() != time.UTC {
		t.Fatalf("ClaimedAt() = %v, want UTC-normalized %v", request.ClaimedAt(), claimedAt)
	}
	if request.BatchLimit() != 25 || !request.IsValid() {
		t.Fatal("valid claim request did not retain its bounded batch limit")
	}
}

func TestNewOutboxClaimRequestRejectsInvalidValues(t *testing.T) {
	tests := map[string]func() (OutboxClaimRequest, error){
		"empty owner": func() (OutboxClaimRequest, error) {
			return NewOutboxClaimRequest(" ", asyncTestTime, 1)
		},
		"oversized owner": func() (OutboxClaimRequest, error) {
			return NewOutboxClaimRequest(
				strings.Repeat("x", maximumClaimOwnerCharacters+1),
				asyncTestTime,
				1,
			)
		},
		"zero time": func() (OutboxClaimRequest, error) {
			return NewOutboxClaimRequest("publisher-1", time.Time{}, 1)
		},
		"zero limit": func() (OutboxClaimRequest, error) {
			return NewOutboxClaimRequest("publisher-1", asyncTestTime, 0)
		},
		"negative limit": func() (OutboxClaimRequest, error) {
			return NewOutboxClaimRequest("publisher-1", asyncTestTime, -1)
		},
		"oversized limit": func() (OutboxClaimRequest, error) {
			return NewOutboxClaimRequest(
				"publisher-1",
				asyncTestTime,
				MaximumOutboxPublicationBatchLimit+1,
			)
		},
	}

	for name, construct := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := construct(); err == nil {
				t.Fatal("constructor returned nil error")
			}
		})
	}
	if (OutboxClaimRequest{}).IsValid() {
		t.Fatal("zero OutboxClaimRequest is valid")
	}
}

func TestNewMarkOutboxPublishedCommandAcceptsClaimVersion(t *testing.T) {
	publishedAt := asyncTestTime.In(time.FixedZone("test-offset", -2*60*60))
	command, err := NewMarkOutboxPublishedCommand(
		"message-1",
		" publisher-1 ",
		1,
		publishedAt,
	)
	if err != nil {
		t.Fatalf("NewMarkOutboxPublishedCommand() error = %v", err)
	}
	if command.MessageID() != MessageID("message-1") ||
		command.ClaimOwner() != "publisher-1" ||
		command.ExpectedLockVersion() != 1 ||
		!command.PublishedAt().Equal(publishedAt) ||
		command.PublishedAt().Location() != time.UTC ||
		!command.IsValid() {
		t.Fatal("valid mark command was not normalized correctly")
	}
}

func TestNewMarkOutboxPublishedCommandRejectsInvalidValues(t *testing.T) {
	tests := map[string]func() (MarkOutboxPublishedCommand, error){
		"empty message id": func() (MarkOutboxPublishedCommand, error) {
			return NewMarkOutboxPublishedCommand("", "publisher-1", 1, asyncTestTime)
		},
		"empty owner": func() (MarkOutboxPublishedCommand, error) {
			return NewMarkOutboxPublishedCommand("message-1", " ", 1, asyncTestTime)
		},
		"zero version": func() (MarkOutboxPublishedCommand, error) {
			return NewMarkOutboxPublishedCommand("message-1", "publisher-1", 0, asyncTestTime)
		},
		"negative version": func() (MarkOutboxPublishedCommand, error) {
			return NewMarkOutboxPublishedCommand("message-1", "publisher-1", -1, asyncTestTime)
		},
		"non-incrementable version": func() (MarkOutboxPublishedCommand, error) {
			return NewMarkOutboxPublishedCommand("message-1", "publisher-1", int64(^uint64(0)>>1), asyncTestTime)
		},
		"zero time": func() (MarkOutboxPublishedCommand, error) {
			return NewMarkOutboxPublishedCommand("message-1", "publisher-1", 1, time.Time{})
		},
	}

	for name, construct := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := construct(); err == nil {
				t.Fatal("constructor returned nil error")
			}
		})
	}
	if (MarkOutboxPublishedCommand{}).IsValid() {
		t.Fatal("zero MarkOutboxPublishedCommand is valid")
	}
}

func TestNewReleaseStaleOutboxClaimsRequestValidatesCutoffAndLimit(t *testing.T) {
	staleBefore := asyncTestTime.In(time.FixedZone("test-offset", 90*60))
	request, err := NewReleaseStaleOutboxClaimsRequest(staleBefore, 10)
	if err != nil {
		t.Fatalf("NewReleaseStaleOutboxClaimsRequest() error = %v", err)
	}
	if !request.StaleBefore().Equal(staleBefore) ||
		request.StaleBefore().Location() != time.UTC ||
		request.BatchLimit() != 10 ||
		!request.IsValid() {
		t.Fatal("valid stale-claim request was not normalized correctly")
	}

	invalid := []struct {
		name   string
		cutoff time.Time
		limit  int
	}{
		{name: "zero cutoff", cutoff: time.Time{}, limit: 1},
		{name: "zero limit", cutoff: asyncTestTime, limit: 0},
		{name: "negative limit", cutoff: asyncTestTime, limit: -1},
		{
			name:   "oversized limit",
			cutoff: asyncTestTime,
			limit:  MaximumOutboxPublicationBatchLimit + 1,
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewReleaseStaleOutboxClaimsRequest(test.cutoff, test.limit); err == nil {
				t.Fatal("constructor returned nil error")
			}
		})
	}
	if (ReleaseStaleOutboxClaimsRequest{}).IsValid() {
		t.Fatal("zero ReleaseStaleOutboxClaimsRequest is valid")
	}
}

func validInlinePayload(t *testing.T, data []byte) runtime.Payload {
	t.Helper()
	payload, err := runtime.NewInlinePayload(
		runtime.ContentTypeApplicationJSON,
		data,
		nil,
		MaximumAsyncEncodedPayloadBytes+1,
	)
	if err != nil {
		t.Fatalf("NewInlinePayload() error = %v", err)
	}
	return payload
}

func TestNewPageRequestUsesDefaultLimit(
	t *testing.T,
) {
	request, err := NewPageRequest(
		0,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	if request.Limit() != DefaultPageLimit {
		t.Fatalf(
			"Limit() = %d, want %d",
			request.Limit(),
			DefaultPageLimit,
		)
	}

	if _, exists := request.After(); exists {
		t.Fatal(
			"After() reported an absent page token",
		)
	}

	if !request.IsValid() {
		t.Fatal(
			"valid PageRequest was reported as invalid",
		)
	}
}

func TestNewPageRequestNormalizesToken(
	t *testing.T,
) {
	request, err := NewPageRequest(
		25,
		PageToken(" token-1 "),
	)
	if err != nil {
		t.Fatalf(
			"NewPageRequest() returned an unexpected error: %v",
			err,
		)
	}

	token, exists := request.After()
	if !exists {
		t.Fatal(
			"After() reported no token",
		)
	}

	if token.String() != "token-1" {
		t.Fatalf(
			"After() = %q, want %q",
			token,
			"token-1",
		)
	}
}

func TestNewPageRequestRejectsInvalidValues(
	t *testing.T,
) {
	tests := []struct {
		name  string
		limit int
		token PageToken
		field string
	}{
		{
			name:  "negative limit",
			limit: -1,
			field: "pageLimit",
		},
		{
			name:  "limit above maximum",
			limit: MaximumPageLimit + 1,
			field: "pageLimit",
		},
		{
			name:  "blank provided token",
			limit: 10,
			token: PageToken("   "),
			field: "pageToken",
		},
		{
			name:  "token above maximum",
			limit: 10,
			token: PageToken(
				strings.Repeat(
					"a",
					maximumPageTokenSize+1,
				),
			),
			field: "pageToken",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewPageRequest(
				test.limit,
				test.token,
			)
			if err == nil {
				t.Fatal(
					"NewPageRequest() returned nil error",
				)
			}

			var validationError *ValidationError
			if !errors.As(
				err,
				&validationError,
			) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestPageDefensivelyCopiesItems(
	t *testing.T,
) {
	source := []string{
		"execution-1",
		"execution-2",
	}

	page, err := NewPage(
		source,
		PageToken("next-token"),
	)
	if err != nil {
		t.Fatalf(
			"NewPage() returned an unexpected error: %v",
			err,
		)
	}

	source[0] = "changed"

	first := page.Items()
	if first[0] != "execution-1" {
		t.Fatalf(
			"Items()[0] = %q, want %q",
			first[0],
			"execution-1",
		)
	}

	first[0] = "mutated"

	second := page.Items()
	if second[0] != "execution-1" {
		t.Fatalf(
			"Items()[0] changed through getter: %q",
			second[0],
		)
	}

	next, exists := page.Next()
	if !exists ||
		next.String() != "next-token" {
		t.Fatalf(
			"Next() = %q, %t",
			next,
			exists,
		)
	}

	if !page.HasNext() {
		t.Fatal(
			"HasNext() = false, want true",
		)
	}
}

func TestStoreErrorClassification(
	t *testing.T,
) {
	cause := errors.New(
		"database operation failed",
	)

	tests := []struct {
		name       string
		err        error
		classifier func(error) bool
	}{
		{
			name: "not found",
			err: NewNotFoundError(
				"get",
				"workflow execution",
				cause,
			),
			classifier: IsNotFound,
		},
		{
			name: "conflict",
			err: NewConflictError(
				"create",
				"workflow execution",
				cause,
			),
			classifier: IsConflict,
		},
		{
			name: "stale write",
			err: NewStaleWriteError(
				"update",
				"node execution",
				cause,
			),
			classifier: IsStaleWrite,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.classifier(test.err) {
				t.Fatal(
					"store error classifier returned false",
				)
			}

			if !errors.Is(
				test.err,
				cause,
			) {
				t.Fatal(
					"store error did not preserve its cause",
				)
			}
		})
	}
}

func TestStoreErrorDoesNotExposeTechnicalCauseInMessage(
	t *testing.T,
) {
	cause := errors.New(
		`duplicate key violates constraint "secret_constraint"`,
	)

	err := NewConflictError(
		"create",
		"workflow execution",
		cause,
	)

	if strings.Contains(
		err.Error(),
		"secret_constraint",
	) {
		t.Fatalf(
			"Error() exposed technical cause: %q",
			err.Error(),
		)
	}

	expected :=
		"repository create workflow execution: conflict"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestStoreErrorClassifiersRejectUnrelatedErrors(
	t *testing.T,
) {
	err := errors.New("unrelated")

	if IsNotFound(err) {
		t.Fatal(
			"IsNotFound() returned true for an unrelated error",
		)
	}

	if IsConflict(err) {
		t.Fatal(
			"IsConflict() returned true for an unrelated error",
		)
	}

	if IsStaleWrite(err) {
		t.Fatal(
			"IsStaleWrite() returned true for an unrelated error",
		)
	}
}

func TestExecutionEventDraftBuildsRecordWithAssignedSequence(
	t *testing.T,
) {
	metadata := []byte(
		`{"source":"persistent-runner"}`,
	)
	expectedMetadata := bytes.Clone(metadata)

	params := validExecutionEventDraftParams()
	params.ID = ExecutionEventID(" event-1 ")
	params.Type = ExecutionEventType(" workflow_started ")
	params.PreviousStatus = " validating "
	params.NewStatus = " running "
	params.SafeMessage = " Workflow execution started "
	params.Metadata = metadata

	draft, err := NewExecutionEventDraft(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventDraft() returned an unexpected error: %v",
			err,
		)
	}

	metadata[2] = 'X'

	record, err := draft.Record(
		SequenceNumber(17),
	)
	if err != nil {
		t.Fatalf(
			"Record() returned an unexpected error: %v",
			err,
		)
	}

	if record.SequenceNumber() != SequenceNumber(17) {
		t.Fatalf(
			"SequenceNumber() = %d, want 17",
			record.SequenceNumber(),
		)
	}

	if record.Type() != ExecutionEventTypeWorkflowStarted {
		t.Fatalf(
			"Type() = %q, want %q",
			record.Type(),
			ExecutionEventTypeWorkflowStarted,
		)
	}

	if actual, exists := record.PreviousStatus(); !exists ||
		actual != execution.WorkflowExecutionStatusValidating.String() {
		t.Fatalf(
			"PreviousStatus() = %q, %t",
			actual,
			exists,
		)
	}

	if actual, exists := record.NewStatus(); !exists ||
		actual != execution.WorkflowExecutionStatusRunning.String() {
		t.Fatalf(
			"NewStatus() = %q, %t",
			actual,
			exists,
		)
	}

	if actual := record.Metadata().Bytes(); !bytes.Equal(
		actual,
		expectedMetadata,
	) {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			expectedMetadata,
		)
	}

	if !draft.IsValid() {
		t.Fatal(
			"valid ExecutionEventDraft was reported as invalid",
		)
	}
}

func TestExecutionEventDraftRejectsInvalidParameters(
	t *testing.T,
) {
	params := validExecutionEventDraftParams()
	params.Type = ExecutionEventTypeNodeStarted
	params.NodeExecutionID = ""
	params.PreviousStatus =
		execution.NodeExecutionStatusReady.String()
	params.NewStatus =
		execution.NodeExecutionStatusRunning.String()

	_, err := NewExecutionEventDraft(params)
	if err == nil {
		t.Fatal(
			"NewExecutionEventDraft() returned nil error for a node event without node execution ID",
		)
	}

	requireTimelineValidationField(
		t,
		err,
		"nodeExecutionID",
	)
}

func TestExecutionEventDraftRejectsInvalidAssignedSequence(
	t *testing.T,
) {
	draft := mustExecutionEventDraft(t)

	_, err := draft.Record(0)
	if err == nil {
		t.Fatal(
			"Record() returned nil error for an invalid sequence",
		)
	}

	requireTimelineValidationField(
		t,
		err,
		"sequenceNumber",
	)
}

func TestExecutionLogDraftBuildsRecordWithAssignedSequence(
	t *testing.T,
) {
	metadata := []byte(
		`{"pluginType":"core.pass-through"}`,
	)
	expectedMetadata := bytes.Clone(metadata)

	params := validExecutionLogDraftParams()
	params.ID = ExecutionLogID(" log-1 ")
	params.Level = ExecutionLogLevel(" info ")
	params.Message = " Node execution completed "
	params.Metadata = metadata

	draft, err := NewExecutionLogDraft(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an unexpected error: %v",
			err,
		)
	}

	metadata[2] = 'X'

	record, err := draft.Record(
		SequenceNumber(18),
	)
	if err != nil {
		t.Fatalf(
			"Record() returned an unexpected error: %v",
			err,
		)
	}

	if record.SequenceNumber() != SequenceNumber(18) {
		t.Fatalf(
			"SequenceNumber() = %d, want 18",
			record.SequenceNumber(),
		)
	}

	if record.Level() != ExecutionLogLevelInfo {
		t.Fatalf(
			"Level() = %q, want %q",
			record.Level(),
			ExecutionLogLevelInfo,
		)
	}

	if record.Message() != "Node execution completed" {
		t.Fatalf(
			"Message() = %q",
			record.Message(),
		)
	}

	if actual := record.Metadata().Bytes(); !bytes.Equal(
		actual,
		expectedMetadata,
	) {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			expectedMetadata,
		)
	}

	if !draft.IsValid() {
		t.Fatal(
			"valid ExecutionLogDraft was reported as invalid",
		)
	}
}

func TestExecutionLogDraftRejectsInvalidParameters(
	t *testing.T,
) {
	params := validExecutionLogDraftParams()
	params.Message = "   "

	_, err := NewExecutionLogDraft(params)
	if err == nil {
		t.Fatal(
			"NewExecutionLogDraft() returned nil error for a blank message",
		)
	}

	requireTimelineValidationField(
		t,
		err,
		"message",
	)
}

func TestExecutionLogDraftRejectsInvalidAssignedSequence(
	t *testing.T,
) {
	draft := mustExecutionLogDraft(t)

	_, err := draft.Record(0)
	if err == nil {
		t.Fatal(
			"Record() returned nil error for an invalid sequence",
		)
	}

	requireTimelineValidationField(
		t,
		err,
		"sequenceNumber",
	)
}

func TestTimelineEntryStoresEventOrLogExclusively(
	t *testing.T,
) {
	eventEntry, err := NewEventTimelineEntry(
		mustExecutionEventDraft(t),
	)
	if err != nil {
		t.Fatalf(
			"NewEventTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	if eventEntry.Kind() != TimelineEntryKindEvent {
		t.Fatalf(
			"Kind() = %q, want %q",
			eventEntry.Kind(),
			TimelineEntryKindEvent,
		)
	}

	if _, exists := eventEntry.Event(); !exists {
		t.Fatal(
			"Event() reported no event for an event entry",
		)
	}

	if _, exists := eventEntry.Log(); exists {
		t.Fatal(
			"Log() reported a log for an event entry",
		)
	}

	if !eventEntry.IsValid() {
		t.Fatal(
			"valid event timeline entry was reported as invalid",
		)
	}

	logEntry, err := NewLogTimelineEntry(
		mustExecutionLogDraft(t),
	)
	if err != nil {
		t.Fatalf(
			"NewLogTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	if logEntry.Kind() != TimelineEntryKindLog {
		t.Fatalf(
			"Kind() = %q, want %q",
			logEntry.Kind(),
			TimelineEntryKindLog,
		)
	}

	if _, exists := logEntry.Log(); !exists {
		t.Fatal(
			"Log() reported no log for a log entry",
		)
	}

	if _, exists := logEntry.Event(); exists {
		t.Fatal(
			"Event() reported an event for a log entry",
		)
	}

	if !logEntry.IsValid() {
		t.Fatal(
			"valid log timeline entry was reported as invalid",
		)
	}
}

func TestTimelineEntryRejectsInvalidDrafts(
	t *testing.T,
) {
	_, err := NewEventTimelineEntry(
		ExecutionEventDraft{},
	)
	if err == nil {
		t.Fatal(
			"NewEventTimelineEntry() returned nil error for a zero-value draft",
		)
	}

	requireTimelineValidationField(
		t,
		err,
		"eventDraft",
	)

	_, err = NewLogTimelineEntry(
		ExecutionLogDraft{},
	)
	if err == nil {
		t.Fatal(
			"NewLogTimelineEntry() returned nil error for a zero-value draft",
		)
	}

	requireTimelineValidationField(
		t,
		err,
		"logDraft",
	)
}

func TestTimelineEntryZeroValueIsInvalid(
	t *testing.T,
) {
	var entry TimelineEntry

	if entry.IsValid() {
		t.Fatal(
			"zero-value TimelineEntry was reported as valid",
		)
	}

	if entry.Kind().IsValid() {
		t.Fatal(
			"zero-value TimelineEntryKind was reported as valid",
		)
	}
}

func mustExecutionEventDraft(
	t *testing.T,
) ExecutionEventDraft {
	t.Helper()

	draft, err := NewExecutionEventDraft(
		validExecutionEventDraftParams(),
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventDraft() returned an unexpected error: %v",
			err,
		)
	}

	return draft
}

func mustExecutionLogDraft(
	t *testing.T,
) ExecutionLogDraft {
	t.Helper()

	draft, err := NewExecutionLogDraft(
		validExecutionLogDraftParams(),
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an unexpected error: %v",
			err,
		)
	}

	return draft
}

func validExecutionEventDraftParams() ExecutionEventDraftParams {
	return ExecutionEventDraftParams{
		ID: ExecutionEventID(
			"event-1",
		),
		WorkflowExecutionID: execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		CompanyID: workflow.CompanyID(
			"company-1",
		),
		Type:           ExecutionEventTypeWorkflowStarted,
		PreviousStatus: execution.WorkflowExecutionStatusValidating.String(),
		NewStatus:      execution.WorkflowExecutionStatusRunning.String(),
		CorrelationID:  "correlation-1",
		CausationID:    "command-1",
		SafeMessage:    "Workflow execution started",
		Metadata: []byte(
			`{"source":"persistent-runner"}`,
		),
		CreatedAt: timelineDraftTestTime(
			10,
			0,
		),
	}
}

func validExecutionLogDraftParams() ExecutionLogDraftParams {
	return ExecutionLogDraftParams{
		ID: ExecutionLogID(
			"log-1",
		),
		WorkflowExecutionID: execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		CompanyID: workflow.CompanyID(
			"company-1",
		),
		NodeExecutionID: execution.NodeExecutionID(
			"node-execution-1",
		),
		Level:   ExecutionLogLevelInfo,
		Message: "Node execution completed",
		Metadata: []byte(
			`{"pluginType":"core.pass-through"}`,
		),
		CreatedAt: timelineDraftTestTime(
			10,
			1,
		),
	}
}

func requireTimelineValidationField(
	t *testing.T,
	err error,
	expectedField string,
) {
	t.Helper()

	var validationError *ValidationError
	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}
}

func timelineDraftTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		17,
		hour,
		minute,
		0,
		0,
		time.FixedZone(
			"TRT",
			3*60*60,
		),
	)
}

type identifierConstructor struct {
	name      string
	field     string
	construct func(string) (string, error)
}

func TestPersistenceIdentifierConstructorsNormalizeValues(
	t *testing.T,
) {
	for _, constructor := range persistenceIdentifierConstructors() {
		t.Run(constructor.name, func(t *testing.T) {
			actual, err := constructor.construct(
				"  record-1  ",
			)
			if err != nil {
				t.Fatalf(
					"constructor returned an unexpected error: %v",
					err,
				)
			}

			if actual != "record-1" {
				t.Fatalf(
					"identifier = %q, want %q",
					actual,
					"record-1",
				)
			}
		})
	}
}

func TestPersistenceIdentifierConstructorsRejectBlankValues(
	t *testing.T,
) {
	for _, constructor := range persistenceIdentifierConstructors() {
		t.Run(constructor.name, func(t *testing.T) {
			_, err := constructor.construct("   ")
			if err == nil {
				t.Fatal(
					"constructor returned nil error for a blank identifier",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != constructor.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					constructor.field,
				)
			}
		})
	}
}

func TestNewSequenceNumberAcceptsPositiveValue(
	t *testing.T,
) {
	number, err := NewSequenceNumber(42)
	if err != nil {
		t.Fatalf(
			"NewSequenceNumber() returned an unexpected error: %v",
			err,
		)
	}

	if actual := number.Int64(); actual != 42 {
		t.Fatalf(
			"Int64() = %d, want %d",
			actual,
			42,
		)
	}

	if !number.IsValid() {
		t.Fatal(
			"positive SequenceNumber was reported as invalid",
		)
	}
}

func TestNewSequenceNumberRejectsNonPositiveValues(
	t *testing.T,
) {
	tests := []int64{
		-1,
		0,
	}

	for _, value := range tests {
		_, err := NewSequenceNumber(value)
		if err == nil {
			t.Fatalf(
				"NewSequenceNumber(%d) returned nil error",
				value,
			)
		}
	}
}

func TestParseExecutionLogLevelNormalizesSupportedValues(
	t *testing.T,
) {
	tests := map[string]ExecutionLogLevel{
		" debug ": ExecutionLogLevelDebug,
		"INFO":    ExecutionLogLevelInfo,
		" Warn ":  ExecutionLogLevelWarn,
		"error":   ExecutionLogLevelError,
	}

	for value, expected := range tests {
		t.Run(value, func(t *testing.T) {
			actual, err := ParseExecutionLogLevel(value)
			if err != nil {
				t.Fatalf(
					"ParseExecutionLogLevel() returned an unexpected error: %v",
					err,
				)
			}

			if actual != expected {
				t.Fatalf(
					"level = %q, want %q",
					actual,
					expected,
				)
			}
		})
	}
}

func TestParseExecutionLogLevelRejectsUnsupportedValue(
	t *testing.T,
) {
	_, err := ParseExecutionLogLevel("TRACE")
	if err == nil {
		t.Fatal(
			"ParseExecutionLogLevel() returned nil error for TRACE",
		)
	}
}

func TestParseExecutionEventTypeNormalizesSupportedValues(
	t *testing.T,
) {
	tests := map[string]ExecutionEventType{
		" workflow_started ": ExecutionEventTypeWorkflowStarted,
		"NODE_SUCCEEDED":     ExecutionEventTypeNodeSucceeded,
		" node_skipped ":     ExecutionEventTypeNodeSkipped,
	}

	for value, expected := range tests {
		t.Run(value, func(t *testing.T) {
			actual, err := ParseExecutionEventType(value)
			if err != nil {
				t.Fatalf(
					"ParseExecutionEventType() returned an unexpected error: %v",
					err,
				)
			}

			if actual != expected {
				t.Fatalf(
					"event type = %q, want %q",
					actual,
					expected,
				)
			}
		})
	}
}

func TestParseExecutionEventTypeRejectsUnsupportedValue(
	t *testing.T,
) {
	_, err := ParseExecutionEventType(
		"WORKFLOW_UNKNOWN",
	)
	if err == nil {
		t.Fatal(
			"ParseExecutionEventType() returned nil error for an unsupported value",
		)
	}
}

func TestExecutionEventTypeScopeClassification(
	t *testing.T,
) {
	if !ExecutionEventTypeWorkflowFailed.IsWorkflowScoped() {
		t.Fatal(
			"WORKFLOW_FAILED was not classified as workflow-scoped",
		)
	}

	if ExecutionEventTypeWorkflowFailed.IsNodeScoped() {
		t.Fatal(
			"WORKFLOW_FAILED was classified as node-scoped",
		)
	}

	if !ExecutionEventTypeNodeFailed.IsNodeScoped() {
		t.Fatal(
			"NODE_FAILED was not classified as node-scoped",
		)
	}

	if ExecutionEventTypeNodeFailed.IsWorkflowScoped() {
		t.Fatal(
			"NODE_FAILED was classified as workflow-scoped",
		)
	}
}

func TestValidationErrorFormatsFieldAndReason(
	t *testing.T,
) {
	err := &ValidationError{
		Field:  "sequenceNumber",
		Reason: "must be greater than zero",
	}

	expected := "sequenceNumber: must be greater than zero"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func persistenceIdentifierConstructors() []identifierConstructor {
	return []identifierConstructor{
		{
			name:  "definition snapshot ID",
			field: "definitionSnapshotID",
			construct: func(value string) (string, error) {
				identifier, err := NewDefinitionSnapshotID(value)
				return identifier.String(), err
			},
		},
		{
			name:  "execution event ID",
			field: "executionEventID",
			construct: func(value string) (string, error) {
				identifier, err := NewExecutionEventID(value)
				return identifier.String(), err
			},
		},
		{
			name:  "execution log ID",
			field: "executionLogID",
			construct: func(value string) (string, error) {
				identifier, err := NewExecutionLogID(value)
				return identifier.String(), err
			},
		},
		{
			name:  "execution error ID",
			field: "executionErrorID",
			construct: func(value string) (string, error) {
				identifier, err := NewExecutionErrorID(value)
				return identifier.String(), err
			},
		},
	}
}

func TestWorkflowExecutionFilterAllowsEmptyFilter(
	t *testing.T,
) {
	filter, err := NewWorkflowExecutionFilter(
		"",
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() returned an unexpected error: %v",
			err,
		)
	}

	if !filter.IsEmpty() {
		t.Fatal(
			"IsEmpty() = false, want true",
		)
	}

	if !filter.IsValid() {
		t.Fatal(
			"valid WorkflowExecutionFilter was reported as invalid",
		)
	}
}

func TestWorkflowExecutionFilterStoresValues(
	t *testing.T,
) {
	filter, err := NewWorkflowExecutionFilter(
		workflow.WorkflowID(" workflow-1 "),
		execution.WorkflowExecutionStatusFailed,
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionFilter() returned an unexpected error: %v",
			err,
		)
	}

	workflowID, exists := filter.WorkflowID()
	if !exists ||
		workflowID.String() != "workflow-1" {
		t.Fatalf(
			"WorkflowID() = %q, %t",
			workflowID,
			exists,
		)
	}

	status, exists := filter.Status()
	if !exists ||
		status != execution.WorkflowExecutionStatusFailed {
		t.Fatalf(
			"Status() = %q, %t",
			status,
			exists,
		)
	}

	if filter.IsEmpty() {
		t.Fatal(
			"IsEmpty() = true, want false",
		)
	}
}

func TestWorkflowExecutionFilterRejectsInvalidValues(
	t *testing.T,
) {
	tests := []struct {
		name       string
		workflowID workflow.WorkflowID
		status     execution.WorkflowExecutionStatus
		field      string
	}{
		{
			name:       "blank provided workflow ID",
			workflowID: workflow.WorkflowID(" "),
			field:      "workflowID",
		},
		{
			name: "invalid status",
			status: execution.WorkflowExecutionStatus(
				"UNKNOWN",
			),
			field: "status",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWorkflowExecutionFilter(
				test.workflowID,
				test.status,
			)
			if err == nil {
				t.Fatal(
					"NewWorkflowExecutionFilter() returned nil error",
				)
			}

			var validationError *ValidationError
			if !errors.As(
				err,
				&validationError,
			) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewWorkflowExecutionRecordStoresNormalizedValues(
	t *testing.T,
) {
	params := validWorkflowExecutionRecordParams()

	params.ID = execution.WorkflowExecutionID(" execution-1 ")
	params.CompanyID = workflow.CompanyID(" company-1 ")
	params.WorkflowID = workflow.WorkflowID(" workflow-1 ")
	params.SnapshotID = DefinitionSnapshotID(" snapshot-1 ")
	params.CorrelationID = " correlation-1 "
	params.IsStalled = true
	params.LockVersion = 3

	record, err := NewWorkflowExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	if actual := record.ID().String(); actual != "execution-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"execution-1",
		)
	}

	if actual := record.CompanyID().String(); actual != "company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := record.WorkflowID().String(); actual != "workflow-1" {
		t.Fatalf(
			"WorkflowID() = %q, want %q",
			actual,
			"workflow-1",
		)
	}

	if actual := record.SnapshotID().String(); actual != "snapshot-1" {
		t.Fatalf(
			"SnapshotID() = %q, want %q",
			actual,
			"snapshot-1",
		)
	}

	if actual, exists := record.CorrelationID(); !exists ||
		actual != "correlation-1" {
		t.Fatalf(
			"CorrelationID() = %q, %t; want %q, true",
			actual,
			exists,
			"correlation-1",
		)
	}

	if record.Status() != execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"Status() = %q, want %q",
			record.Status(),
			execution.WorkflowExecutionStatusSucceeded,
		)
	}

	if record.CreatedAt().Location() != time.UTC {
		t.Fatalf(
			"CreatedAt() location = %s, want UTC",
			record.CreatedAt().Location(),
		)
	}

	if !record.IsStalled() {
		t.Fatal(
			"IsStalled() = false, want true",
		)
	}

	if record.LockVersion() != 3 {
		t.Fatalf(
			"LockVersion() = %d, want 3",
			record.LockVersion(),
		)
	}

	if !record.IsValid() {
		t.Fatal(
			"valid WorkflowExecutionRecord was reported as invalid",
		)
	}
}

func TestNewWorkflowExecutionRecordAllowsAbsentOptionalFields(
	t *testing.T,
) {
	params := validWorkflowExecutionRecordParams()

	params.CorrelationID = ""
	params.Status = execution.WorkflowExecutionStatusCreated
	params.ValidatingAt = time.Time{}
	params.QueuedAt = time.Time{}
	params.StartedAt = time.Time{}
	params.FinishedAt = time.Time{}
	params.TerminalOutputs = nil
	params.FailureSummary = nil
	params.IsStalled = false

	record, err := NewWorkflowExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	if _, exists := record.CorrelationID(); exists {
		t.Fatal(
			"CorrelationID() reported a value for an absent correlation ID",
		)
	}

	if _, exists := record.ValidatingAt(); exists {
		t.Fatal(
			"ValidatingAt() reported a value for an absent timestamp",
		)
	}

	if _, exists := record.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() reported a value for a non-terminal record",
		)
	}

	if _, exists := record.FailureSummary(); exists {
		t.Fatal(
			"FailureSummary() reported a value for an absent summary",
		)
	}

	if actual := record.TerminalOutputs().String(); actual != "{}" {
		t.Fatalf(
			"TerminalOutputs() = %q, want %q",
			actual,
			"{}",
		)
	}
}

func TestNewWorkflowExecutionRecordRejectsInvalidFields(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(*WorkflowExecutionRecordParams)
	}{
		{
			name:  "blank execution ID",
			field: "workflowExecutionID",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.ID = execution.WorkflowExecutionID(" ")
			},
		},
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.CompanyID = workflow.CompanyID(" ")
			},
		},
		{
			name:  "blank workflow ID",
			field: "workflowID",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.WorkflowID = workflow.WorkflowID(" ")
			},
		},
		{
			name:  "zero revision",
			field: "workflowRevision",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.WorkflowRevision = 0
			},
		},
		{
			name:  "revision above PostgreSQL BIGINT",
			field: "workflowRevision",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.WorkflowRevision =
					uint64(math.MaxInt64) + 1
			},
		},
		{
			name:  "blank snapshot ID",
			field: "definitionSnapshotID",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.SnapshotID = DefinitionSnapshotID(" ")
			},
		},
		{
			name:  "invalid mode",
			field: "mode",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.Mode = execution.ExecutionMode("UNKNOWN")
			},
		},
		{
			name:  "blank provided correlation ID",
			field: "correlationID",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.CorrelationID = "   "
			},
		},
		{
			name:  "invalid status",
			field: "status",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.Status =
					execution.WorkflowExecutionStatus("UNKNOWN")
			},
		},
		{
			name:  "zero created time",
			field: "createdAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.CreatedAt = time.Time{}
			},
		},
		{
			name:  "zero updated time",
			field: "updatedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.UpdatedAt = time.Time{}
			},
		},
		{
			name:  "validating before creation",
			field: "validatingAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.ValidatingAt =
					params.CreatedAt.Add(-time.Second)
			},
		},
		{
			name:  "queued before creation",
			field: "queuedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.QueuedAt =
					params.CreatedAt.Add(-time.Second)
			},
		},
		{
			name:  "started before creation",
			field: "startedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.StartedAt =
					params.CreatedAt.Add(-time.Second)
			},
		},
		{
			name:  "finished before creation",
			field: "finishedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.FinishedAt =
					params.CreatedAt.Add(-time.Second)
			},
		},
		{
			name:  "finished before started",
			field: "finishedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.FinishedAt =
					params.StartedAt.Add(-time.Second)
			},
		},
		{
			name:  "updated before creation",
			field: "updatedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.UpdatedAt =
					params.CreatedAt.Add(-time.Second)
			},
		},
		{
			name:  "updated before transition",
			field: "updatedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.UpdatedAt =
					params.FinishedAt.Add(-time.Second)
			},
		},
		{
			name:  "validating without validating time",
			field: "validatingAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.Status =
					execution.WorkflowExecutionStatusValidating
				params.ValidatingAt = time.Time{}
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "queued without queued time",
			field: "queuedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.Status =
					execution.WorkflowExecutionStatusQueued
				params.QueuedAt = time.Time{}
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "running without started time",
			field: "startedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.Status =
					execution.WorkflowExecutionStatusRunning
				params.StartedAt = time.Time{}
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "terminal without finished time",
			field: "finishedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.Status =
					execution.WorkflowExecutionStatusFailed
				params.FinishedAt = time.Time{}
			},
		},
		{
			name:  "non-terminal with finished time",
			field: "finishedAt",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.Status =
					execution.WorkflowExecutionStatusRunning
			},
		},
		{
			name:  "invalid terminal outputs",
			field: "terminalOutputs",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.TerminalOutputs = []byte(`[]`)
			},
		},
		{
			name:  "invalid failure summary",
			field: "failureSummary",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.FailureSummary = []byte(`{"code":`)
			},
		},
		{
			name:  "zero next sequence",
			field: "nextSequenceNumber",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.NextSequenceNumber = 0
			},
		},
		{
			name:  "negative lock version",
			field: "lockVersion",
			mutate: func(params *WorkflowExecutionRecordParams) {
				params.LockVersion = -1
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := validWorkflowExecutionRecordParams()
			test.mutate(&params)

			_, err := NewWorkflowExecutionRecord(params)
			if err == nil {
				t.Fatal(
					"NewWorkflowExecutionRecord() returned nil error for invalid parameters",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestWorkflowExecutionRecordDefensivelyCopiesJSONInputs(
	t *testing.T,
) {
	terminalOutputs := []byte(`{"terminal-node":{"size":12}}`)
	failureSummary := []byte(`{"code":"EXECUTION_FAILED"}`)

	params := validWorkflowExecutionRecordParams()
	params.TerminalOutputs = terminalOutputs
	params.FailureSummary = failureSummary

	record, err := NewWorkflowExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	expectedOutputs := bytes.Clone(terminalOutputs)
	expectedFailure := bytes.Clone(failureSummary)

	terminalOutputs[2] = 'X'
	failureSummary[2] = 'X'

	if actual := record.TerminalOutputs().Bytes(); !bytes.Equal(
		actual,
		expectedOutputs,
	) {
		t.Fatalf(
			"terminal outputs = %q, want %q",
			actual,
			expectedOutputs,
		)
	}

	actualFailure, exists := record.FailureSummary()
	if !exists {
		t.Fatal(
			"FailureSummary() reported no value",
		)
	}

	if actual := actualFailure.Bytes(); !bytes.Equal(
		actual,
		expectedFailure,
	) {
		t.Fatalf(
			"failure summary = %q, want %q",
			actual,
			expectedFailure,
		)
	}
}

func TestWorkflowExecutionRecordJSONGettersReturnDefensiveCopies(
	t *testing.T,
) {
	record := newTestWorkflowExecutionRecord(t)

	outputs := record.TerminalOutputs()
	outputBytes := outputs.Bytes()
	outputBytes[2] = 'X'

	if actual := record.TerminalOutputs().String(); actual !=
		`{"terminal-node":{"size":12}}` {
		t.Fatalf(
			"TerminalOutputs() = %q",
			actual,
		)
	}

	failure, exists := record.FailureSummary()
	if !exists {
		t.Fatal(
			"FailureSummary() reported no value",
		)
	}

	failureBytes := failure.Bytes()
	failureBytes[2] = 'X'

	secondFailure, exists := record.FailureSummary()
	if !exists {
		t.Fatal(
			"FailureSummary() reported no value after mutation attempt",
		)
	}

	if actual := secondFailure.String(); actual !=
		`{"code":"EXECUTION_FAILED"}` {
		t.Fatalf(
			"FailureSummary() = %q",
			actual,
		)
	}
}

func TestWorkflowExecutionRecordZeroValueIsInvalid(
	t *testing.T,
) {
	var record WorkflowExecutionRecord

	if record.IsValid() {
		t.Fatal(
			"zero-value WorkflowExecutionRecord was reported as valid",
		)
	}
}

func newTestWorkflowExecutionRecord(
	t *testing.T,
) WorkflowExecutionRecord {
	t.Helper()

	record, err := NewWorkflowExecutionRecord(
		validWorkflowExecutionRecordParams(),
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func validWorkflowExecutionRecordParams() WorkflowExecutionRecordParams {
	createdAt := workflowExecutionRecordTestTime(10, 0)

	return WorkflowExecutionRecordParams{
		ID: execution.WorkflowExecutionID(
			"execution-1",
		),
		CompanyID: workflow.CompanyID(
			"company-1",
		),
		WorkflowID: workflow.WorkflowID(
			"workflow-1",
		),
		WorkflowRevision: 3,
		SnapshotID: DefinitionSnapshotID(
			"snapshot-1",
		),
		Mode:          execution.ExecutionModeSync,
		CorrelationID: "correlation-1",
		Status:        execution.WorkflowExecutionStatusSucceeded,
		CreatedAt:     createdAt,
		ValidatingAt: createdAt.Add(
			1 * time.Minute,
		),
		StartedAt: createdAt.Add(
			2 * time.Minute,
		),
		FinishedAt: createdAt.Add(
			3 * time.Minute,
		),
		UpdatedAt: createdAt.Add(
			3 * time.Minute,
		),
		TerminalOutputs: []byte(
			`{"terminal-node":{"size":12}}`,
		),
		FailureSummary: []byte(
			`{"code":"EXECUTION_FAILED"}`,
		),
		NextSequenceNumber: SequenceNumber(9),
		LockVersion:        0,
	}
}

func workflowExecutionRecordTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		17,
		hour,
		minute,
		0,
		0,
		time.FixedZone(
			"TRT",
			3*60*60,
		),
	)
}

type workflowTransitionStoreContractStub struct{}

var _ WorkflowTransitionStore = (*workflowTransitionStoreContractStub)(nil)

func (
	*workflowTransitionStoreContractStub,
) ApplyWorkflowTransition(
	context.Context,
	WorkflowTransitionCommand,
) error {
	return nil
}

func TestNewWorkflowTransitionCommandStoresValidatingTransition(
	t *testing.T,
) {
	params :=
		validWorkflowTransitionCommandParams(t)

	command, err :=
		NewWorkflowTransitionCommand(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowTransitionCommand() returned an unexpected error: %v",
			err,
		)
	}

	if command.CompanyID().String() !=
		"company-1" {
		t.Fatalf(
			"CompanyID() = %q",
			command.CompanyID(),
		)
	}

	if command.WorkflowExecutionID().String() !=
		"execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q",
			command.WorkflowExecutionID(),
		)
	}

	if command.ExpectedStatus() !=
		execution.WorkflowExecutionStatusCreated {
		t.Fatalf(
			"ExpectedStatus() = %q",
			command.ExpectedStatus(),
		)
	}

	if command.ExpectedLockVersion() != 3 {
		t.Fatalf(
			"ExpectedLockVersion() = %d, want 3",
			command.ExpectedLockVersion(),
		)
	}

	if command.ExpectedNextSequenceNumber() !=
		SequenceNumber(7) {
		t.Fatalf(
			"ExpectedNextSequenceNumber() = %d, want 7",
			command.ExpectedNextSequenceNumber(),
		)
	}

	if command.WorkflowExecution().Status() !=
		execution.WorkflowExecutionStatusValidating {
		t.Fatalf(
			"target status = %q",
			command.WorkflowExecution().Status(),
		)
	}

	if actual := len(command.Timeline()); actual != 2 {
		t.Fatalf(
			"timeline count = %d, want 2",
			actual,
		)
	}

	if actual := len(command.Errors()); actual != 0 {
		t.Fatalf(
			"error count = %d, want 0",
			actual,
		)
	}

	if !command.IsValid() {
		t.Fatal(
			"valid WorkflowTransitionCommand was reported as invalid",
		)
	}
}

func TestNewWorkflowTransitionCommandStoresFailedTransition(
	t *testing.T,
) {
	params :=
		validFailedWorkflowTransitionCommandParams(t)

	command, err :=
		NewWorkflowTransitionCommand(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowTransitionCommand() returned an unexpected error: %v",
			err,
		)
	}

	if command.ExpectedStatus() !=
		execution.WorkflowExecutionStatusRunning {
		t.Fatalf(
			"ExpectedStatus() = %q, want RUNNING",
			command.ExpectedStatus(),
		)
	}

	if command.WorkflowExecution().Status() !=
		execution.WorkflowExecutionStatusFailed {
		t.Fatalf(
			"target status = %q, want FAILED",
			command.WorkflowExecution().Status(),
		)
	}

	if actual := len(command.Errors()); actual != 1 {
		t.Fatalf(
			"error count = %d, want 1",
			actual,
		)
	}

	if !command.IsValid() {
		t.Fatal(
			"valid failed transition command was reported as invalid",
		)
	}
}

func TestNewWorkflowTransitionCommandRejectsInvalidCoreState(
	t *testing.T,
) {
	tests := []struct {
		name   string
		field  string
		mutate func(
			*WorkflowTransitionCommandParams,
			*testing.T,
		)
	}{
		{
			name:  "blank company ID",
			field: "companyID",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.CompanyID =
					workflow.CompanyID(" ")
			},
		},
		{
			name:  "workflow execution ID mismatch",
			field: "workflowExecution",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.WorkflowExecutionID =
					execution.WorkflowExecutionID(
						"execution-2",
					)
			},
		},
		{
			name:  "invalid expected status",
			field: "expectedStatus",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedStatus =
					execution.WorkflowExecutionStatus(
						"UNKNOWN",
					)
			},
		},
		{
			name:  "illegal transition",
			field: "expectedStatus",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedStatus =
					execution.WorkflowExecutionStatusRunning
			},
		},
		{
			name:  "negative expected lock version",
			field: "expectedLockVersion",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedLockVersion = -1
			},
		},
		{
			name:  "lock version did not increment",
			field: "workflowExecution",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				params.WorkflowExecution =
					newValidatingTransitionRecord(
						t,
						9,
						3,
						nil,
					)
			},
		},
		{
			name:  "invalid expected sequence",
			field: "expectedNextSequenceNumber",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.ExpectedNextSequenceNumber = 0
			},
		},
		{
			name:  "next sequence did not advance correctly",
			field: "workflowExecution",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				params.WorkflowExecution =
					newValidatingTransitionRecord(
						t,
						10,
						4,
						nil,
					)
			},
		},
		{
			name:  "invalid updated workflow record",
			field: "workflowExecution",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.WorkflowExecution =
					WorkflowExecutionRecord{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validWorkflowTransitionCommandParams(t)

			test.mutate(&params, t)

			_, err :=
				NewWorkflowTransitionCommand(params)
			if err == nil {
				t.Fatal(
					"NewWorkflowTransitionCommand() returned nil error",
				)
			}

			requireWorkflowTransitionValidationField(
				t,
				err,
				test.field,
			)
		})
	}
}

func TestNewWorkflowTransitionCommandRejectsInvalidTimeline(
	t *testing.T,
) {
	tests := []struct {
		name   string
		mutate func(
			*WorkflowTransitionCommandParams,
			*testing.T,
		)
	}{
		{
			name: "empty timeline",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.Timeline = nil
			},
		},
		{
			name: "first entry is a log",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.Timeline[0] =
					params.Timeline[1]
			},
		},
		{
			name: "wrong transition event type",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				record :=
					params.WorkflowExecution

				params.Timeline[0] =
					newWorkflowTransitionEventEntry(
						t,
						record,
						params.ExpectedStatus,
						ExecutionEventTypeWorkflowStarted,
						ExecutionEventID(
							"event-wrong-type",
						),
						record.UpdatedAt(),
					)
			},
		},
		{
			name: "node scoped workflow log",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				params.Timeline[1] =
					newWorkflowTransitionLogEntry(
						t,
						params.WorkflowExecution,
						ExecutionLogID(
							"log-node-scoped",
						),
						execution.NodeExecutionID(
							"node-execution-1",
						),
						params.WorkflowExecution.
							UpdatedAt(),
					)
			},
		},
		{
			name: "duplicate log ID",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.Timeline = append(
					params.Timeline,
					params.Timeline[1],
				)

				params.WorkflowExecution =
					newValidatingTransitionRecord(
						t,
						10,
						4,
						nil,
					)
			},
		},
		{
			name: "transition event timestamp mismatch",
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				record :=
					params.WorkflowExecution

				params.Timeline[0] =
					newWorkflowTransitionEventEntry(
						t,
						record,
						params.ExpectedStatus,
						ExecutionEventTypeWorkflowValidating,
						ExecutionEventID(
							"event-wrong-time",
						),
						record.UpdatedAt().
							Add(time.Second),
					)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params :=
				validWorkflowTransitionCommandParams(t)

			test.mutate(&params, t)

			_, err :=
				NewWorkflowTransitionCommand(params)
			if err == nil {
				t.Fatal(
					"NewWorkflowTransitionCommand() returned nil error",
				)
			}

			requireWorkflowTransitionValidationField(
				t,
				err,
				"timeline",
			)
		})
	}
}

func TestNewWorkflowTransitionCommandRejectsInvalidErrors(
	t *testing.T,
) {
	tests := []struct {
		name   string
		params func(
			*testing.T,
		) WorkflowTransitionCommandParams
		mutate func(
			*WorkflowTransitionCommandParams,
			*testing.T,
		)
	}{
		{
			name:   "failed transition without structured error",
			params: validFailedWorkflowTransitionCommandParams,
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.Errors = nil
			},
		},
		{
			name:   "nonfailure transition with structured error",
			params: validWorkflowTransitionCommandParams,
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				event, _ :=
					params.Timeline[0].Event()

				params.Errors =
					[]ExecutionErrorRecord{
						newWorkflowTransitionError(
							t,
							params.WorkflowExecution,
							event.ID(),
							nil,
						),
					}
			},
		},
		{
			name:   "error without related event",
			params: validFailedWorkflowTransitionCommandParams,
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				params.Errors[0] =
					newWorkflowTransitionError(
						t,
						params.WorkflowExecution,
						"",
						nil,
					)
			},
		},
		{
			name:   "node scoped workflow error",
			params: validFailedWorkflowTransitionCommandParams,
			mutate: func(
				params *WorkflowTransitionCommandParams,
				t *testing.T,
			) {
				event, _ :=
					params.Timeline[0].Event()

				params.Errors[0] =
					newWorkflowTransitionError(
						t,
						params.WorkflowExecution,
						event.ID(),
						func(
							errorParams *ExecutionErrorRecordParams,
						) {
							errorParams.NodeExecutionID =
								execution.NodeExecutionID(
									"node-execution-1",
								)
						},
					)
			},
		},
		{
			name:   "duplicate error ID",
			params: validFailedWorkflowTransitionCommandParams,
			mutate: func(
				params *WorkflowTransitionCommandParams,
				_ *testing.T,
			) {
				params.Errors = append(
					params.Errors,
					params.Errors[0],
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := test.params(t)
			test.mutate(&params, t)

			_, err :=
				NewWorkflowTransitionCommand(params)
			if err == nil {
				t.Fatal(
					"NewWorkflowTransitionCommand() returned nil error",
				)
			}

			requireWorkflowTransitionValidationField(
				t,
				err,
				"errors",
			)
		})
	}
}

func TestWorkflowTransitionCommandReturnsDefensiveSliceCopies(
	t *testing.T,
) {
	command, err :=
		NewWorkflowTransitionCommand(
			validFailedWorkflowTransitionCommandParams(t),
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowTransitionCommand() returned an unexpected error: %v",
			err,
		)
	}

	timeline := command.Timeline()
	timeline[0] = TimelineEntry{}

	if !command.Timeline()[0].IsValid() {
		t.Fatal(
			"Timeline() allowed command mutation",
		)
	}

	executionErrors := command.Errors()
	executionErrors[0] = ExecutionErrorRecord{}

	if !command.Errors()[0].IsValid() {
		t.Fatal(
			"Errors() allowed command mutation",
		)
	}
}

func TestWorkflowTransitionCommandZeroValueIsInvalid(
	t *testing.T,
) {
	var command WorkflowTransitionCommand

	if command.IsValid() {
		t.Fatal(
			"zero-value WorkflowTransitionCommand was reported as valid",
		)
	}
}

func validWorkflowTransitionCommandParams(
	t *testing.T,
) WorkflowTransitionCommandParams {
	t.Helper()

	record :=
		newValidatingTransitionRecord(
			t,
			9,
			4,
			nil,
		)

	return WorkflowTransitionCommandParams{
		CompanyID:                  record.CompanyID(),
		WorkflowExecutionID:        record.ID(),
		ExpectedStatus:             execution.WorkflowExecutionStatusCreated,
		ExpectedLockVersion:        3,
		ExpectedNextSequenceNumber: SequenceNumber(7),
		WorkflowExecution:          record,
		Timeline: newWorkflowTransitionTimeline(
			t,
			record,
			execution.WorkflowExecutionStatusCreated,
			ExecutionEventTypeWorkflowValidating,
			ExecutionEventID(
				"event-workflow-validating",
			),
			ExecutionLogID(
				"log-workflow-validating",
			),
		),
	}
}

func validFailedWorkflowTransitionCommandParams(
	t *testing.T,
) WorkflowTransitionCommandParams {
	t.Helper()

	record :=
		newFailedTransitionRecord(
			t,
			22,
			6,
			nil,
		)

	timeline :=
		newWorkflowTransitionTimeline(
			t,
			record,
			execution.WorkflowExecutionStatusRunning,
			ExecutionEventTypeWorkflowFailed,
			ExecutionEventID(
				"event-workflow-failed",
			),
			ExecutionLogID(
				"log-workflow-failed",
			),
		)

	event, _ := timeline[0].Event()

	return WorkflowTransitionCommandParams{
		CompanyID:                  record.CompanyID(),
		WorkflowExecutionID:        record.ID(),
		ExpectedStatus:             execution.WorkflowExecutionStatusRunning,
		ExpectedLockVersion:        5,
		ExpectedNextSequenceNumber: SequenceNumber(20),
		WorkflowExecution:          record,
		Timeline:                   timeline,
		Errors: []ExecutionErrorRecord{
			newWorkflowTransitionError(
				t,
				record,
				event.ID(),
				nil,
			),
		},
	}
}

func newValidatingTransitionRecord(
	t *testing.T,
	nextSequence SequenceNumber,
	lockVersion int64,
	mutate func(
		*WorkflowExecutionRecordParams,
	),
) WorkflowExecutionRecord {
	t.Helper()

	params :=
		validWorkflowExecutionRecordParams()

	params.Status =
		execution.WorkflowExecutionStatusValidating

	params.ValidatingAt =
		params.CreatedAt.Add(time.Minute)

	params.QueuedAt = time.Time{}
	params.StartedAt = time.Time{}
	params.FinishedAt = time.Time{}

	params.UpdatedAt =
		params.ValidatingAt

	params.TerminalOutputs = nil
	params.FailureSummary = nil
	params.IsStalled = false
	params.NextSequenceNumber = nextSequence
	params.LockVersion = lockVersion

	if mutate != nil {
		mutate(&params)
	}

	record, err :=
		NewWorkflowExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func newFailedTransitionRecord(
	t *testing.T,
	nextSequence SequenceNumber,
	lockVersion int64,
	mutate func(
		*WorkflowExecutionRecordParams,
	),
) WorkflowExecutionRecord {
	t.Helper()

	params :=
		validWorkflowExecutionRecordParams()

	params.Status =
		execution.WorkflowExecutionStatusFailed

	params.ValidatingAt =
		params.CreatedAt.Add(time.Minute)

	params.QueuedAt = time.Time{}

	params.StartedAt =
		params.CreatedAt.Add(
			2 * time.Minute,
		)

	params.FinishedAt =
		params.CreatedAt.Add(
			3 * time.Minute,
		)

	params.UpdatedAt =
		params.FinishedAt

	params.TerminalOutputs = nil

	params.FailureSummary =
		[]byte(
			`{"code":"WORKFLOW_FAILED"}`,
		)

	params.IsStalled = false
	params.NextSequenceNumber = nextSequence
	params.LockVersion = lockVersion

	if mutate != nil {
		mutate(&params)
	}

	record, err :=
		NewWorkflowExecutionRecord(params)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecutionRecord() returned an unexpected error: %v",
			err,
		)
	}

	return record
}

func newWorkflowTransitionTimeline(
	t *testing.T,
	record WorkflowExecutionRecord,
	previousStatus execution.WorkflowExecutionStatus,
	eventType ExecutionEventType,
	eventID ExecutionEventID,
	logID ExecutionLogID,
) []TimelineEntry {
	t.Helper()

	transitionAt :=
		record.UpdatedAt()

	return []TimelineEntry{
		newWorkflowTransitionEventEntry(
			t,
			record,
			previousStatus,
			eventType,
			eventID,
			transitionAt,
		),
		newWorkflowTransitionLogEntry(
			t,
			record,
			logID,
			"",
			transitionAt,
		),
	}
}

func newWorkflowTransitionEventEntry(
	t *testing.T,
	record WorkflowExecutionRecord,
	previousStatus execution.WorkflowExecutionStatus,
	eventType ExecutionEventType,
	eventID ExecutionEventID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	eventDraft, err :=
		NewExecutionEventDraft(
			ExecutionEventDraftParams{
				ID:                  eventID,
				WorkflowExecutionID: record.ID(),
				CompanyID:           record.CompanyID(),
				Type:                eventType,
				PreviousStatus:      previousStatus.String(),
				NewStatus:           record.Status().String(),
				SafeMessage:         "Workflow execution status changed",
				CreatedAt:           createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionEventDraft() returned an unexpected error: %v",
			err,
		)
	}

	entry, err :=
		NewEventTimelineEntry(eventDraft)
	if err != nil {
		t.Fatalf(
			"NewEventTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	return entry
}

func newWorkflowTransitionLogEntry(
	t *testing.T,
	record WorkflowExecutionRecord,
	logID ExecutionLogID,
	nodeExecutionID execution.NodeExecutionID,
	createdAt time.Time,
) TimelineEntry {
	t.Helper()

	logDraft, err :=
		NewExecutionLogDraft(
			ExecutionLogDraftParams{
				ID:                  logID,
				WorkflowExecutionID: record.ID(),
				CompanyID:           record.CompanyID(),
				NodeExecutionID:     nodeExecutionID,
				Level:               ExecutionLogLevelInfo,
				Message:             "Workflow execution status changed",
				CreatedAt:           createdAt,
			},
		)
	if err != nil {
		t.Fatalf(
			"NewExecutionLogDraft() returned an unexpected error: %v",
			err,
		)
	}

	entry, err :=
		NewLogTimelineEntry(logDraft)
	if err != nil {
		t.Fatalf(
			"NewLogTimelineEntry() returned an unexpected error: %v",
			err,
		)
	}

	return entry
}

func newWorkflowTransitionError(
	t *testing.T,
	record WorkflowExecutionRecord,
	relatedEventID ExecutionEventID,
	mutate func(
		*ExecutionErrorRecordParams,
	),
) ExecutionErrorRecord {
	t.Helper()

	params := ExecutionErrorRecordParams{
		ID: ExecutionErrorID(
			"error-workflow-transition",
		),
		WorkflowExecutionID: record.ID(),
		CompanyID:           record.CompanyID(),
		RelatedEventID:      relatedEventID,
		Category:            runtime.FailureCategoryExecution,
		Code:                "WORKFLOW_EXECUTION_FAILED",
		SafeMessage:         "Workflow execution failed",
		TechnicalDetail:     "workflow execution returned a failure",
		Retryable:           false,
		Details: []byte(
			`{"scope":"workflow"}`,
		),
		CreatedAt: record.UpdatedAt(),
	}

	if mutate != nil {
		mutate(&params)
	}

	executionError, err :=
		NewExecutionErrorRecord(params)
	if err != nil {
		t.Fatalf(
			"NewExecutionErrorRecord() returned an unexpected error: %v",
			err,
		)
	}

	return executionError
}

func requireWorkflowTransitionValidationField(
	t *testing.T,
	err error,
	expectedField string,
) {
	t.Helper()

	var validationError *ValidationError

	if !errors.As(
		err,
		&validationError,
	) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field !=
		expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}
}
