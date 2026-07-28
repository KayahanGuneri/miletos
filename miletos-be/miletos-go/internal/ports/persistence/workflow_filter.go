package repository

import (
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type WorkflowExecutionFilter struct {
	workflowID    workflow.WorkflowID
	hasWorkflowID bool
	status        execution.WorkflowExecutionStatus
	hasStatus     bool
}

func NewWorkflowExecutionFilter(
	workflowID workflow.WorkflowID, status execution.WorkflowExecutionStatus) (WorkflowExecutionFilter, error) {
	var normalizedWorkflowID workflow.WorkflowID
	hasWorkflowID := workflowID.String() != ""
	if hasWorkflowID {
		value, err := workflow.NewWorkflowID(workflowID.String())
		if err != nil {
			return WorkflowExecutionFilter{},
				newValidationError("workflowID", err.Error())
		}
		normalizedWorkflowID = value
	}
	var normalizedStatus execution.WorkflowExecutionStatus
	hasStatus := status.String() != ""
	if hasStatus {
		if !status.IsValid() {
			return WorkflowExecutionFilter{},
				newValidationError("status", "must contain a supported workflow execution status")
		}
		normalizedStatus = status
	}
	return WorkflowExecutionFilter{workflowID: normalizedWorkflowID, hasWorkflowID: hasWorkflowID,
		status: normalizedStatus, hasStatus: hasStatus}, nil
}
func (filter WorkflowExecutionFilter) WorkflowID() (
	workflow.WorkflowID, bool) {
	if !filter.hasWorkflowID {
		return "", false
	}
	return filter.workflowID, true
}
func (filter WorkflowExecutionFilter) Status() (execution.WorkflowExecutionStatus,
	bool) {
	if !filter.hasStatus {
		return "", false
	}
	return filter.status, true
}
func (filter WorkflowExecutionFilter) IsEmpty() bool {
	return !filter.hasWorkflowID && !filter.hasStatus
}
func (filter WorkflowExecutionFilter) IsValid() bool {
	_, err := NewWorkflowExecutionFilter(filter.workflowID, filter.status)
	return err == nil
}
