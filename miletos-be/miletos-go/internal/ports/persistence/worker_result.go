package repository

import (
	"fmt"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strconv"
	"time"
)

const MaximumDurableWorkerTechnicalDetailCharacters = maximumExecutionErrorTechnicalDetailCharacters

type DurableWorkerResultStatus string

const (
	DurableWorkerResultSucceeded DurableWorkerResultStatus = "SUCCEEDED"
	DurableWorkerResultFailed    DurableWorkerResultStatus = "FAILED"
	DurableWorkerResultCancelled DurableWorkerResultStatus = "CANCELLED"
	DurableWorkerResultTimedOut  DurableWorkerResultStatus = "TIMED_OUT"
)

func (status DurableWorkerResultStatus) IsValid() bool {
	switch status {
	case DurableWorkerResultSucceeded, DurableWorkerResultFailed, DurableWorkerResultCancelled, DurableWorkerResultTimedOut:
		return true
	default:
		return false
	}
}

type DurableWorkerResultParams struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeExecutionID     execution.NodeExecutionID
	NodeID              workflow.NodeID
	Attempt             int16
	Status              DurableWorkerResultStatus
	Result              runtime.NodeResult
	Failure             runtime.RuntimeFailure
	TechnicalDetail     string
	StartedAt           time.Time
	FinishedAt          time.Time
	CreatedAt           time.Time
}
type DurableWorkerResult struct {
	params     DurableWorkerResultParams
	hasResult  bool
	hasFailure bool
	identity   string
}

func NewDurableWorkerResult(params DurableWorkerResultParams) (DurableWorkerResult, error) {
	companyID, workflowExecutionID, err := normalizeAsyncExecutionScope(params.CompanyID, params.WorkflowExecutionID)
	if err != nil {
		return DurableWorkerResult{}, err
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(params.NodeExecutionID.String())
	if err != nil {
		return DurableWorkerResult{}, newValidationError("nodeExecutionID", err.Error())
	}
	nodeID, err := workflow.NewNodeID(params.NodeID.String())
	if err != nil {
		return DurableWorkerResult{}, newValidationError("nodeID", err.Error())
	}
	if params.Attempt <= 0 {
		return DurableWorkerResult{}, newValidationError("attempt", "must be greater than zero")
	}
	if !params.Status.IsValid() {
		return DurableWorkerResult{}, newValidationError("status", "must contain a supported durable worker result status")
	}
	result, hasResult, failure, hasFailure, err := normalizeDurableResultParts(params)
	if err != nil {
		return DurableWorkerResult{}, err
	}
	technicalDetail, err := normalizeOptionalBoundedString(
		"technicalDetail", params.TechnicalDetail,
		MaximumDurableWorkerTechnicalDetailCharacters,
	)
	if err != nil {
		return DurableWorkerResult{}, err
	}
	startedAt, err := normalizeRequiredRecordTime("startedAt", params.StartedAt)
	if err != nil {
		return DurableWorkerResult{}, err
	}
	finishedAt, err := normalizeRequiredRecordTime("finishedAt", params.FinishedAt)
	if err != nil {
		return DurableWorkerResult{}, err
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return DurableWorkerResult{}, err
	}
	if finishedAt.Before(startedAt) {
		return DurableWorkerResult{}, newValidationError("finishedAt", "must not be before startedAt")
	}
	if createdAt.Before(finishedAt) {
		return DurableWorkerResult{}, newValidationError("createdAt", "must not be before finishedAt")
	}
	identity := deterministicAsyncIdentity("worker-result", companyID.String(), workflowExecutionID.String(), nodeExecutionID.String(), strconv.Itoa(int(params.Attempt)))
	return DurableWorkerResult{params: DurableWorkerResultParams{CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, NodeExecutionID: nodeExecutionID, NodeID: nodeID, Attempt: params.Attempt, Status: params.Status, Result: result, Failure: failure, TechnicalDetail: technicalDetail, StartedAt: startedAt, FinishedAt: finishedAt, CreatedAt: createdAt}, hasResult: hasResult, hasFailure: hasFailure, identity: identity}, nil
}
func normalizeDurableResultParts(params DurableWorkerResultParams) (runtime.NodeResult, bool, runtime.RuntimeFailure, bool, error) {
	switch params.Status {
	case DurableWorkerResultSucceeded, DurableWorkerResultFailed:
		result, err := cloneBoundedNodeResult(params.Result)
		if err != nil {
			return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("result", err.Error())
		}
		if params.Status == DurableWorkerResultSucceeded && !result.IsSuccess() {
			return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("status", "must match successful runtime result")
		}
		if params.Status == DurableWorkerResultFailed && !result.IsFailure() {
			return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("status", "must match failed runtime result")
		}
		if params.Failure.IsValid() {
			return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("failure", "must be empty when runtime result is present")
		}
		return result, true, runtime.RuntimeFailure{}, false, nil
	case DurableWorkerResultCancelled, DurableWorkerResultTimedOut:
		if params.Result.IsValid() {
			return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("result", "must be empty for interrupted result")
		}
		failure, err := cloneRuntimeFailure(params.Failure)
		if err != nil {
			return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("failure", err.Error())
		}
		expectedCategory := runtime.FailureCategoryCanceled
		if params.Status == DurableWorkerResultTimedOut {
			expectedCategory = runtime.FailureCategoryTimeout
		}
		if failure.Category() != expectedCategory {
			return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("failure", "category must match interrupted result status")
		}
		return runtime.NodeResult{}, false, failure, true, nil
	}
	return runtime.NodeResult{}, false, runtime.RuntimeFailure{}, false, newValidationError("status", "must contain a supported durable worker result status")
}
func cloneBoundedNodeResult(result runtime.NodeResult) (runtime.NodeResult, error) {
	if !result.IsValid() {
		return runtime.NodeResult{}, fmt.Errorf("must be valid")
	}
	if result.IsFailure() {
		failure, exists := result.Failure()
		if !exists {
			return runtime.NodeResult{}, fmt.Errorf("failed result must contain failure")
		}
		return runtime.NewNodeFailureResult(failure)
	}
	changes, err := cloneBoundedContextChanges(result.ContextChanges())
	if err != nil {
		return runtime.NodeResult{}, err
	}
	if terminal, exists := result.TerminalOutput(); exists {
		cloned, err := cloneBoundedAsyncPayload(terminal)
		if err != nil {
			return runtime.NodeResult{}, err
		}
		return runtime.NewTerminalNodeSuccessResult(cloned, changes)
	}
	outputs := result.OutputsSnapshot()
	for port, payloads := range outputs {
		cloned := make([]runtime.Payload, len(payloads))
		for index, payload := range payloads {
			cloned[index], err = cloneBoundedAsyncPayload(payload)
			if err != nil {
				return runtime.NodeResult{}, fmt.Errorf("output %q payload %d: %w", port, index, err)
			}
		}
		outputs[port] = cloned
	}
	return runtime.NewNodeSuccessResult(outputs, changes)
}
func cloneBoundedContextChanges(changes runtime.ContextChanges) (runtime.ContextChanges, error) {
	if !changes.IsValid() {
		return runtime.ContextChanges{}, fmt.Errorf("context changes must be valid")
	}
	for key, value := range changes.SetValues() {
		if len(value.Bytes()) > MaximumAsyncContextValueBytes {
			return runtime.ContextChanges{}, fmt.Errorf("context value %q exceeds size limit", key)
		}
	}
	return runtime.NewContextChanges(changes.SetValues(), changes.DeleteKeys())
}
func cloneRuntimeFailure(failure runtime.RuntimeFailure) (runtime.RuntimeFailure, error) {
	if !failure.IsValid() {
		return runtime.RuntimeFailure{}, fmt.Errorf("must be valid")
	}
	return runtime.NewRuntimeFailure(failure.Category(), failure.Code(), failure.Message(), failure.Retryable(), failure.Details())
}
func (result DurableWorkerResult) CompanyID() workflow.CompanyID {
	return result.params.CompanyID
}
func (result DurableWorkerResult) WorkflowExecutionID() execution.WorkflowExecutionID {
	return result.params.WorkflowExecutionID
}
func (result DurableWorkerResult) NodeExecutionID() execution.NodeExecutionID {
	return result.params.NodeExecutionID
}
func (result DurableWorkerResult) NodeID() workflow.NodeID {
	return result.params.NodeID
}
func (result DurableWorkerResult) Attempt() int16 {
	return result.params.Attempt
}
func (result DurableWorkerResult) Status() DurableWorkerResultStatus {
	return result.params.Status
}
func (result DurableWorkerResult) Result() (runtime.NodeResult, bool) {
	if !result.hasResult {
		return runtime.NodeResult{}, false
	}
	cloned, _ := cloneBoundedNodeResult(result.params.Result)
	return cloned, true
}
func (result DurableWorkerResult) Failure() (runtime.RuntimeFailure, bool) {
	if !result.hasFailure {
		return runtime.RuntimeFailure{}, false
	}
	cloned, _ := cloneRuntimeFailure(result.params.Failure)
	return cloned, true
}
func (result DurableWorkerResult) TechnicalDetail() (string, bool) {
	if result.params.TechnicalDetail == "" {
		return "", false
	}
	return result.params.TechnicalDetail, true
}
func (result DurableWorkerResult) StartedAt() time.Time {
	return result.params.StartedAt
}
func (result DurableWorkerResult) FinishedAt() time.Time {
	return result.params.FinishedAt
}
func (result DurableWorkerResult) CreatedAt() time.Time {
	return result.params.CreatedAt
}
func (result DurableWorkerResult) IdentityKey() string {
	return result.identity
}
func (result DurableWorkerResult) IsValid() bool {
	normalized, err := NewDurableWorkerResult(result.params)
	return err == nil && normalized.identity == result.identity
}
