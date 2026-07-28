package execution

import (
	"time"

	"miletos-go/internal/features/workflow"
)

type WorkflowExecution struct {
	id               WorkflowExecutionID
	companyID        workflow.CompanyID
	workflowID       workflow.WorkflowID
	workflowRevision uint64
	mode             ExecutionMode
	status           WorkflowExecutionStatus
	createdAt        time.Time
	startedAt        time.Time
	finishedAt       time.Time
}

func NewWorkflowExecution(id WorkflowExecutionID, companyID workflow.CompanyID,
	workflowID workflow.WorkflowID, workflowRevision uint64, mode ExecutionMode,
	createdAt time.Time) (WorkflowExecution, error) {
	normalizedID, err := NewWorkflowExecutionID(
		id.String())
	if err != nil {
		return WorkflowExecution{}, err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return WorkflowExecution{}, err
	}
	normalizedWorkflowID, err := workflow.NewWorkflowID(workflowID.String())
	if err != nil {
		return WorkflowExecution{}, err
	}
	if workflowRevision == 0 {
		return WorkflowExecution{}, newValidationError("workflowRevision", "must be greater than zero")
	}
	if !mode.IsValid() {
		return WorkflowExecution{}, &InvalidModeError{Value: mode.String()}
	}
	if err := validateCreatedAt(createdAt); err != nil {
		return WorkflowExecution{}, err
	}
	return WorkflowExecution{id: normalizedID,
		companyID: normalizedCompanyID, workflowID: normalizedWorkflowID, workflowRevision: workflowRevision,
		mode: mode, status: WorkflowExecutionStatusCreated, createdAt: createdAt,
	}, nil
}
func (execution WorkflowExecution) ID() WorkflowExecutionID { return execution.id }
func (execution WorkflowExecution) CompanyID() workflow.CompanyID {
	return execution.companyID
}
func (execution WorkflowExecution) WorkflowID() workflow.WorkflowID {
	return execution.workflowID
}
func (execution WorkflowExecution) WorkflowRevision() uint64 { return execution.workflowRevision }
func (execution WorkflowExecution) Mode() ExecutionMode {
	return execution.mode
}
func (execution WorkflowExecution) Status() WorkflowExecutionStatus {
	return execution.status
}
func (execution WorkflowExecution) CreatedAt() time.Time { return execution.createdAt }
func (execution WorkflowExecution) StartedAt() (time.Time, bool) {
	if execution.startedAt.IsZero() {
		return time.Time{}, false
	}
	return execution.startedAt, true
}
func (execution WorkflowExecution) FinishedAt() (time.Time, bool) {
	if execution.finishedAt.IsZero() {
		return time.Time{}, false
	}
	return execution.finishedAt, true
}
func (execution WorkflowExecution) IsTerminal() bool {
	return execution.status.IsTerminal()
}
func (execution *WorkflowExecution) StartValidation(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusValidating, at)
}
func (execution *WorkflowExecution) Reject(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusRejected, at)
}
func (execution *WorkflowExecution) Queue(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusQueued, at)
}
func (execution *WorkflowExecution) Start(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusRunning, at)
}
func (execution *WorkflowExecution) Succeed(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusSucceeded, at)
}
func (execution *WorkflowExecution) Fail(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusFailed, at)
}
func (execution *WorkflowExecution) Cancel(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusCancelled, at)
}
func (execution *WorkflowExecution) Timeout(at time.Time) error {
	return execution.transitionTo(WorkflowExecutionStatusTimedOut, at)
}
func (execution *WorkflowExecution) transitionTo(target WorkflowExecutionStatus, at time.Time,
) error {
	if execution == nil {
		return newValidationError(
			"workflowExecution", "must not be nil")
	}
	if !isWorkflowTransitionAllowed(
		execution.status, target) {
		return &TransitionError{Entity: "workflow execution", CurrentStatus: execution.status.String(),
			TargetStatus: target.String()}
	}
	if err := validateTransitionTimestamp(at,
		execution.createdAt, execution.startedAt, target.IsTerminal(),
	); err != nil {
		return err
	}
	nextStartedAt := execution.startedAt
	nextFinishedAt := execution.finishedAt
	if target == WorkflowExecutionStatusRunning && nextStartedAt.IsZero() {
		nextStartedAt = at
	}
	if target.IsTerminal() {
		nextFinishedAt = at
	}
	execution.status = target
	execution.startedAt = nextStartedAt
	execution.finishedAt = nextFinishedAt
	return nil
}
func isWorkflowTransitionAllowed(
	current WorkflowExecutionStatus, target WorkflowExecutionStatus) bool {
	switch current {
	case WorkflowExecutionStatusCreated:
		return target == WorkflowExecutionStatusValidating ||
			target == WorkflowExecutionStatusCancelled
	case WorkflowExecutionStatusValidating:
		return target == WorkflowExecutionStatusRejected || target == WorkflowExecutionStatusQueued || target == WorkflowExecutionStatusRunning ||
			target == WorkflowExecutionStatusCancelled || target == WorkflowExecutionStatusTimedOut
	case WorkflowExecutionStatusQueued:
		return target == WorkflowExecutionStatusRunning || target == WorkflowExecutionStatusFailed ||
			target == WorkflowExecutionStatusCancelled || target == WorkflowExecutionStatusTimedOut
	case WorkflowExecutionStatusRunning:
		return target == WorkflowExecutionStatusSucceeded || target == WorkflowExecutionStatusFailed ||
			target == WorkflowExecutionStatusCancelled || target == WorkflowExecutionStatusTimedOut
	default:
		return false
	}
}

type WorkflowExecutionStatus string

const (
	WorkflowExecutionStatusCreated    WorkflowExecutionStatus = "CREATED"
	WorkflowExecutionStatusValidating WorkflowExecutionStatus = "VALIDATING"
	WorkflowExecutionStatusRejected   WorkflowExecutionStatus = "REJECTED"
	WorkflowExecutionStatusQueued     WorkflowExecutionStatus = "QUEUED"
	WorkflowExecutionStatusRunning    WorkflowExecutionStatus = "RUNNING"
	WorkflowExecutionStatusSucceeded  WorkflowExecutionStatus = "SUCCEEDED"
	WorkflowExecutionStatusFailed     WorkflowExecutionStatus = "FAILED"
	WorkflowExecutionStatusCancelled  WorkflowExecutionStatus = "CANCELLED"
	WorkflowExecutionStatusTimedOut   WorkflowExecutionStatus = "TIMED_OUT"
)

func (status WorkflowExecutionStatus) String() string {
	return string(status)
}
func (status WorkflowExecutionStatus) IsValid() bool {
	switch status {
	case WorkflowExecutionStatusCreated, WorkflowExecutionStatusValidating,
		WorkflowExecutionStatusRejected, WorkflowExecutionStatusQueued, WorkflowExecutionStatusRunning,
		WorkflowExecutionStatusSucceeded, WorkflowExecutionStatusFailed, WorkflowExecutionStatusCancelled,
		WorkflowExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func (
	status WorkflowExecutionStatus) IsTerminal() bool {
	switch status {
	case WorkflowExecutionStatusRejected, WorkflowExecutionStatusSucceeded, WorkflowExecutionStatusFailed,
		WorkflowExecutionStatusCancelled, WorkflowExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func (status WorkflowExecutionStatus) CanTransitionTo(
	target WorkflowExecutionStatus) bool {
	if !status.IsValid() ||
		!target.IsValid() {
		return false
	}
	return isWorkflowTransitionAllowed(status,
		target)
}
