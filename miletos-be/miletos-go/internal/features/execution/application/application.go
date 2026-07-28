package executionfeature

import (
	"context"
	"fmt"
	"miletos-go/internal/engine"
	"miletos-go/internal/features/execution"
	"time"
)

type ValidationDetail struct {
	Code          string
	Field         string
	Reason        string
	NodeID        string
	EdgeID        string
	PluginType    string
	PluginVersion string
	Expected      string
	Actual        string
	CyclePath     []string
}

func buildValidationDetails(report engine.PreflightValidationReport,
) []ValidationDetail {
	if report.IsValid() {
		return nil
	}
	details :=
		make([]ValidationDetail, 0,
			report.Len())
	for _, issue := range report.StructuralIssues() {
		cyclePath :=
			make([]string, 0,
				len(issue.Path))
		for _, nodeID := range issue.Path {
			cyclePath = append(
				cyclePath, nodeID.String())
		}
		details = append(
			details, ValidationDetail{Code: string(
				issue.Code),
				Reason: issue.Message, NodeID: issue.
					NodeID.String(),
				EdgeID:    issue.EdgeID.String(),
				CyclePath: cyclePath},
		)
	}
	for _, issue := range report.PluginIssues() {
		detail :=
			ValidationDetail{Code: string(issue.Code), Field: issue.Field,
				Reason: issue.Message,
				NodeID: issue.NodeID.String(),
				EdgeID: issue.EdgeID.
					String()}
		if issue.PluginIdentity.IsValid() {
			detail.PluginType = issue.
				PluginIdentity.Type().String()
			detail.PluginVersion = issue.
				PluginIdentity.Version().String()
		}
		expected :=
			formatEdgeCountExpectation(issue.Minimum, issue.Maximum,
				issue.HasMaximum)
		if expected != "" {
			detail.Expected = expected
			detail.Actual = fmt.Sprintf(
				"%d", issue.Actual)
		}
		details = append(
			details, detail)
	}
	for _, issue := range report.EngineIssues() {
		detail := ValidationDetail{
			Code:   issue.Code.String(),
			Field:  issue.Field,
			Reason: safeEngineValidationReasonValue(issue.Code),
			NodeID: issue.NodeID.
				String(), EdgeID: issue.
				EdgeID.String()}
		if issue.PluginIdentity.
			IsValid() {
			detail.PluginType =
				issue.PluginIdentity.Type().
					String()
			detail.PluginVersion =
				issue.PluginIdentity.Version().
					String()
		}
		details = append(details, detail)
	}
	return details
}

func formatEdgeCountExpectation(minimum uint, maximum uint,
	hasMaximum bool) string {
	if hasMaximum {
		if minimum == maximum {
			return fmt.Sprintf("%d",
				minimum)
		}
		return fmt.Sprintf("%d..%d",
			minimum, maximum)
	}
	if minimum > 0 {
		return fmt.Sprintf(">=%d", minimum)
	}
	return ""
}

func safeEngineValidationReasonValue(code engine.PreflightValidationIssueCode) string {
	switch code {
	case engine.IssueCodeTopologicalOrderUnavailable:
		return "workflow topological order is unavailable"
	case engine.IssueCodeRuntimeEdgeInvalid:
		return "runtime edge configuration is invalid"
	case engine.
		IssueCodeExecutorNotFound:
		return "required node executor is unavailable"
	default:
		return "execution plan validation failed"
	}
}

func edgeCountExpectation(
	minimum uint, maximum uint, hasMaximum bool,
) string {
	return formatEdgeCountExpectation(minimum,
		maximum, hasMaximum)
}

func safeEngineValidationReason(
	code engine.PreflightValidationIssueCode) string {
	return safeEngineValidationReasonValue(
		code)
}

type ExecutionOutcome struct {
	ExecutionID       string
	WorkflowID        string
	WorkflowRevision  uint64
	Mode              string
	Status            string
	CorrelationID     string
	CreatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	ScheduledRoots    int
	Rejected          bool
	ValidationDetails []ValidationDetail
}

type ExecutionApplication interface {
	Execute(
		context.Context, engine.ExecutionRequest) (
		ExecutionOutcome, error)
	AsyncEnabled() bool
}

type EngineExecutionApplication struct {
	service engine.ExecutionService
}

func NewEngineExecutionApplication(
	service engine.ExecutionService) (EngineExecutionApplication,
	error) {
	if !service.IsValid() {
		return EngineExecutionApplication{}, fmt.Errorf("execution service must be valid")
	}
	return EngineExecutionApplication{service: service},
		nil
}

func (application EngineExecutionApplication) AsyncEnabled() bool {
	return application.service.AsyncEnabled()
}

func (application EngineExecutionApplication) Execute(
	ctx context.Context, request engine.ExecutionRequest) (
	ExecutionOutcome, error) {
	if ctx == nil {
		return ExecutionOutcome{}, fmt.Errorf(
			"execution context must not be nil")
	}
	if !request.IsValid() {
		return ExecutionOutcome{},
			fmt.Errorf("execution request must be valid")
	}
	result, err :=
		application.service.Run(ctx, request)
	switch result.Mode() {
	case execution.ExecutionModeSync:
		syncResult, exists := result.SyncResult()
		if exists && syncResult.IsValid() {
			return newExecutionOutcome(syncResult.
				WorkflowExecution(), request.
				CorrelationID(), syncResult.
				ValidationReport(), 0,
			), nil
		}
	case execution.ExecutionModeAsync:
		asyncResult, exists :=
			result.AsyncResult()
		if exists &&
			asyncResult.WorkflowExecution.ID().
				String() != "" {
			outcome :=
				newExecutionOutcome(asyncResult.WorkflowExecution,
					request.CorrelationID(),
					asyncResult.ValidationReport,
					asyncResult.ScheduledRoots,
				)
			if err != nil {
				return outcome, err
			}
			return outcome, nil
		}
	}
	if err != nil {
		return ExecutionOutcome{}, err
	}
	return ExecutionOutcome{},
		fmt.Errorf("execution service returned no valid execution result")
}

func newExecutionOutcome(
	workflowExecution execution.WorkflowExecution, correlationID string, validationReport engine.PreflightValidationReport,
	scheduledRoots int) ExecutionOutcome {
	var startedAt *time.Time
	var finishedAt *time.Time
	if value, exists :=
		workflowExecution.StartedAt(); exists {
		copied := value.UTC()
		startedAt = &copied
	}
	if value, exists := workflowExecution.
		FinishedAt(); exists {
		copied :=
			value.UTC()
		finishedAt =
			&copied
	}
	return ExecutionOutcome{ExecutionID: workflowExecution.ID().
		String(), WorkflowID: workflowExecution.
		WorkflowID().String(),
		WorkflowRevision: workflowExecution.WorkflowRevision(),
		Mode:             workflowExecution.Mode().String(),
		Status: workflowExecution.Status().
			String(), CorrelationID: correlationID,
		CreatedAt: workflowExecution.CreatedAt().
			UTC(), StartedAt: startedAt,
		FinishedAt:     finishedAt,
		ScheduledRoots: scheduledRoots, Rejected: workflowExecution.
				Status() == execution.WorkflowExecutionStatusRejected,
		ValidationDetails: buildValidationDetails(validationReport)}
}
