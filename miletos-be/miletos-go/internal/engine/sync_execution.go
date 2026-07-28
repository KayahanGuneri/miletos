package engine

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/features/execution"
)

type SyncRunResult struct {
	workflowExecution execution.WorkflowExecution
	validationReport  PreflightValidationReport
	executionResult   ExecutionResult
	hasExecution      bool
	initialized       bool
}

func (result SyncRunResult) WorkflowExecution() execution.WorkflowExecution {
	return result.workflowExecution
}

func (result SyncRunResult) Status() execution.WorkflowExecutionStatus {
	return result.workflowExecution.Status()
}

func (result SyncRunResult) ValidationReport() PreflightValidationReport {
	return result.validationReport
}

func (result SyncRunResult) HasExecutionResult() bool {
	return result.initialized && result.hasExecution
}

func (result SyncRunResult) ExecutionResult() (ExecutionResult, bool) {
	if !result.HasExecutionResult() {
		return ExecutionResult{}, false
	}
	return result.executionResult, true
}

func (result SyncRunResult) IsRejected() bool {
	return result.initialized &&
		result.workflowExecution.Status() == execution.WorkflowExecutionStatusRejected
}

func (result SyncRunResult) IsSucceeded() bool {
	return result.initialized &&
		result.workflowExecution.Status() == execution.WorkflowExecutionStatusSucceeded
}

func (result SyncRunResult) IsFailed() bool {
	return result.initialized &&
		result.workflowExecution.Status() == execution.WorkflowExecutionStatusFailed
}

func (result SyncRunResult) IsCancelled() bool {
	return result.initialized &&
		result.workflowExecution.Status() == execution.WorkflowExecutionStatusCancelled
}

func (result SyncRunResult) IsTimedOut() bool {
	return result.initialized &&
		result.workflowExecution.Status() == execution.WorkflowExecutionStatusTimedOut
}

func (result SyncRunResult) IsTerminal() bool {
	return result.initialized && result.workflowExecution.IsTerminal()
}

func (result SyncRunResult) IsValid() bool {
	if !result.initialized || !result.workflowExecution.IsTerminal() {
		return false
	}
	if result.IsRejected() {
		return !result.validationReport.IsValid() && !result.hasExecution
	}
	if !result.validationReport.IsValid() ||
		!result.hasExecution ||
		!result.executionResult.IsTerminal() {
		return false
	}
	executedWorkflow := result.executionResult.WorkflowExecution()
	return executedWorkflow.ID() == result.workflowExecution.ID() &&
		executedWorkflow.Status() == result.workflowExecution.Status()
}

func newRejectedSyncRunResult(
	workflowExecution execution.WorkflowExecution,
	validationReport PreflightValidationReport,
) (SyncRunResult, error) {
	if workflowExecution.Status() != execution.WorkflowExecutionStatusRejected {
		return SyncRunResult{},
			newValidationError("workflowExecution.status", "must be REJECTED")
	}
	if validationReport.IsValid() {
		return SyncRunResult{},
			newValidationError(
				"validationReport",
				"must contain at least one validation issue",
			)
	}
	result := SyncRunResult{
		workflowExecution: workflowExecution,
		validationReport:  validationReport,
		initialized:       true,
	}
	if !result.IsValid() {
		return SyncRunResult{},
			fmt.Errorf("constructed rejected sync-run result is invalid")
	}
	return result, nil
}

func newExecutedSyncRunResult(
	validationReport PreflightValidationReport,
	executionResult ExecutionResult,
) (SyncRunResult, error) {
	if !validationReport.IsValid() {
		return SyncRunResult{},
			newValidationError(
				"validationReport",
				"must be valid for an executed workflow",
			)
	}
	if !executionResult.IsTerminal() {
		return SyncRunResult{},
			newValidationError("executionResult", "must be terminal")
	}
	workflowExecution := executionResult.WorkflowExecution()
	if workflowExecution.Status() == execution.WorkflowExecutionStatusRejected {
		return SyncRunResult{},
			newValidationError("executionResult.status", "must not be REJECTED")
	}
	result := SyncRunResult{
		workflowExecution: workflowExecution,
		validationReport:  validationReport,
		executionResult:   executionResult,
		hasExecution:      true,
		initialized:       true,
	}
	if !result.IsValid() {
		return SyncRunResult{},
			fmt.Errorf("constructed executed sync-run result is invalid")
	}
	return result, nil
}

type SyncRunner struct {
	dependencies EngineDependencies
	initialized  bool
}

func NewSyncRunner(dependencies EngineDependencies) (SyncRunner, error) {
	if !dependencies.IsValid() {
		return SyncRunner{},
			newValidationError("dependencies", "must be valid")
	}
	return SyncRunner{dependencies: dependencies, initialized: true}, nil
}

func (runner SyncRunner) IsValid() bool {
	return runner.initialized && runner.dependencies.IsValid()
}

func (runner SyncRunner) Run(
	parent context.Context,
	request ExecutionRequest,
) (SyncRunResult, error) {
	if !runner.IsValid() {
		return SyncRunResult{},
			newValidationError("syncRunner", "must be valid")
	}
	if parent == nil {
		return SyncRunResult{},
			newValidationError("context", "must not be nil")
	}
	if !request.IsValid() {
		return SyncRunResult{},
			newValidationError("request", "must be valid")
	}
	if request.Mode() != execution.ExecutionModeSync {
		return SyncRunResult{},
			newValidationError(
				"request.mode",
				"sync runner accepts only SYNC requests",
			)
	}
	preparation, err := prepareExecution(parent, request, runner.dependencies)
	if err != nil {
		return SyncRunResult{},
			fmt.Errorf("prepare synchronous workflow execution: %w", err)
	}
	if preparation.IsRejected() {
		result, err := newRejectedSyncRunResult(
			preparation.WorkflowExecution(),
			preparation.ValidationReport(),
		)
		if err != nil {
			return SyncRunResult{},
				fmt.Errorf("build rejected synchronous run result: %w", err)
		}
		return result, nil
	}
	if !preparation.IsPrepared() || preparation.prepared == nil {
		return SyncRunResult{},
			fmt.Errorf("workflow execution was neither rejected nor prepared")
	}
	executionResult, executionErr := runPreparedExecution(preparation.prepared)
	if !executionResult.IsTerminal() {
		if executionErr != nil {
			return SyncRunResult{},
				fmt.Errorf("run synchronous workflow execution: %w", executionErr)
		}
		return SyncRunResult{},
			fmt.Errorf("synchronous scheduler returned a non-terminal execution result")
	}
	result, resultErr := newExecutedSyncRunResult(
		preparation.ValidationReport(),
		executionResult,
	)
	if resultErr != nil {
		if executionErr != nil {
			return SyncRunResult{},
				errors.Join(
					fmt.Errorf("run synchronous workflow execution: %w", executionErr),
					fmt.Errorf("build synchronous run result: %w", resultErr),
				)
		}
		return SyncRunResult{},
			fmt.Errorf("build synchronous run result: %w", resultErr)
	}
	if executionErr != nil {
		return result,
			fmt.Errorf("run synchronous workflow execution: %w", executionErr)
	}
	return result, nil
}
