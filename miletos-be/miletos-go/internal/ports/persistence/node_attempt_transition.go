package repository

import (
	"time"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type NodeAttemptMutationKind string

const (
	NodeAttemptMutationStart    NodeAttemptMutationKind = "START"
	NodeAttemptMutationComplete NodeAttemptMutationKind = "COMPLETE"
)

func (kind NodeAttemptMutationKind) IsValid() bool {
	return kind == NodeAttemptMutationStart ||
		kind == NodeAttemptMutationComplete
}

type NodeAttemptCompletionParams struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeExecutionID     execution.NodeExecutionID
	ExpectedAttempt     execution.AttemptNumber
	Status              execution.NodeExecutionStatus
	FinishedAt          time.Time
	OutputSummary       []byte
	FailureSummary      []byte
	RetryDecision       execution.RetryDecision
	NextAttemptAt       time.Time
}

type NodeAttemptCompletion struct {
	params NodeAttemptCompletionParams

	outputSummary     JSONObject
	hasOutputSummary  bool
	failureSummary    JSONObject
	hasFailureSummary bool
	retryDecision     execution.RetryDecision
	hasRetryDecision  bool
}

func NewNodeAttemptCompletion(
	params NodeAttemptCompletionParams,
) (NodeAttemptCompletion, error) {
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return NodeAttemptCompletion{},
			newValidationError("companyID", err.Error())
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(
		params.WorkflowExecutionID.String(),
	)
	if err != nil {
		return NodeAttemptCompletion{},
			newValidationError("workflowExecutionID", err.Error())
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(
		params.NodeExecutionID.String(),
	)
	if err != nil {
		return NodeAttemptCompletion{},
			newValidationError("nodeExecutionID", err.Error())
	}
	expectedAttempt, err := execution.NewAttemptNumber(
		params.ExpectedAttempt.Int16(),
	)
	if err != nil {
		return NodeAttemptCompletion{},
			newValidationError("expectedAttempt", err.Error())
	}
	if !isTerminalNodeExecutionAttemptStatus(params.Status) {
		return NodeAttemptCompletion{}, newValidationError(
			"status", "must contain a supported terminal attempt status")
	}
	finishedAt, err := normalizeRequiredRecordTime(
		"finishedAt", params.FinishedAt,
	)
	if err != nil {
		return NodeAttemptCompletion{}, err
	}
	outputSummary, hasOutputSummary, err := normalizeOptionalJSONObject(
		"outputSummary", params.OutputSummary,
	)
	if err != nil {
		return NodeAttemptCompletion{}, err
	}
	failureSummary, hasFailureSummary, err := normalizeOptionalJSONObject(
		"failureSummary", params.FailureSummary,
	)
	if err != nil {
		return NodeAttemptCompletion{}, err
	}
	if params.Status != execution.NodeExecutionStatusSucceeded &&
		hasOutputSummary {
		return NodeAttemptCompletion{}, newValidationError(
			"outputSummary", "must be absent unless status is SUCCEEDED")
	}
	switch params.Status {
	case execution.NodeExecutionStatusSucceeded:
		if hasFailureSummary {
			return NodeAttemptCompletion{}, newValidationError(
				"failureSummary", "must be absent for SUCCEEDED status")
		}
	case execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusTimedOut:
		if !hasFailureSummary {
			return NodeAttemptCompletion{}, newValidationError(
				"failureSummary", "must be provided for the attempt status")
		}
	}
	retryDecision, hasRetryDecision, err := normalizeOptionalRetryDecision(
		params.RetryDecision,
	)
	if err != nil {
		return NodeAttemptCompletion{}, err
	}
	if err := validateNodeExecutionAttemptRetryDecision(
		params.Status, retryDecision, hasRetryDecision,
	); err != nil {
		return NodeAttemptCompletion{}, err
	}
	nextAttemptAt, err := normalizeOptionalRecordTime(
		"nextAttemptAt", params.NextAttemptAt, finishedAt,
	)
	if err != nil {
		return NodeAttemptCompletion{}, err
	}
	if hasRetryDecision {
		if retryDecision.CurrentAttempt() != expectedAttempt {
			return NodeAttemptCompletion{}, newValidationError(
				"retryDecision", "current attempt must match expectedAttempt")
		}
		if retryDecision.Kind() == execution.RetryDecisionRetry {
			if nextAttemptAt.IsZero() ||
				!nextAttemptAt.After(finishedAt) {
				return NodeAttemptCompletion{}, newValidationError(
					"nextAttemptAt", "must be after finishedAt for a retry decision")
			}
		} else if !nextAttemptAt.IsZero() {
			return NodeAttemptCompletion{}, newValidationError(
				"nextAttemptAt", "must be absent when retry is not scheduled")
		}
	} else if !nextAttemptAt.IsZero() {
		return NodeAttemptCompletion{}, newValidationError(
			"nextAttemptAt", "must be absent without a retry decision")
	}
	normalizedParams := NodeAttemptCompletionParams{
		CompanyID:           companyID,
		WorkflowExecutionID: workflowExecutionID,
		NodeExecutionID:     nodeExecutionID,
		ExpectedAttempt:     expectedAttempt,
		Status:              params.Status,
		FinishedAt:          finishedAt,
		OutputSummary:       optionalJSONObjectBytes(outputSummary, hasOutputSummary),
		FailureSummary:      optionalJSONObjectBytes(failureSummary, hasFailureSummary),
		RetryDecision:       retryDecision,
		NextAttemptAt:       nextAttemptAt,
	}
	return NodeAttemptCompletion{
		params:            normalizedParams,
		outputSummary:     outputSummary,
		hasOutputSummary:  hasOutputSummary,
		failureSummary:    failureSummary,
		hasFailureSummary: hasFailureSummary,
		retryDecision:     retryDecision,
		hasRetryDecision:  hasRetryDecision,
	}, nil
}

func isTerminalNodeExecutionAttemptStatus(
	status execution.NodeExecutionStatus,
) bool {
	switch status {
	case execution.NodeExecutionStatusSucceeded,
		execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusCancelled,
		execution.NodeExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}

func (completion NodeAttemptCompletion) CompanyID() workflow.CompanyID {
	return completion.params.CompanyID
}

func (completion NodeAttemptCompletion) WorkflowExecutionID() execution.WorkflowExecutionID {
	return completion.params.WorkflowExecutionID
}

func (completion NodeAttemptCompletion) NodeExecutionID() execution.NodeExecutionID {
	return completion.params.NodeExecutionID
}

func (completion NodeAttemptCompletion) ExpectedAttempt() execution.AttemptNumber {
	return completion.params.ExpectedAttempt
}

func (completion NodeAttemptCompletion) Status() execution.NodeExecutionStatus {
	return completion.params.Status
}

func (completion NodeAttemptCompletion) FinishedAt() time.Time {
	return completion.params.FinishedAt
}

func (completion NodeAttemptCompletion) OutputSummary() (JSONObject, bool) {
	if !completion.hasOutputSummary {
		return JSONObject{}, false
	}
	return completion.outputSummary, true
}

func (completion NodeAttemptCompletion) FailureSummary() (JSONObject, bool) {
	if !completion.hasFailureSummary {
		return JSONObject{}, false
	}
	return completion.failureSummary, true
}

func (completion NodeAttemptCompletion) RetryDecision() (execution.RetryDecision, bool) {
	if !completion.hasRetryDecision {
		return execution.RetryDecision{}, false
	}
	return completion.retryDecision, true
}

func (completion NodeAttemptCompletion) NextAttemptAt() (time.Time, bool) {
	return optionalRecordTime(completion.params.NextAttemptAt)
}

func (completion NodeAttemptCompletion) IsValid() bool {
	_, err := NewNodeAttemptCompletion(completion.params)
	return err == nil
}

type NodeAttemptMutation struct {
	kind       NodeAttemptMutationKind
	start      NodeExecutionAttemptRecord
	completion NodeAttemptCompletion
}

func NewNodeAttemptStartMutation(
	record NodeExecutionAttemptRecord,
) (NodeAttemptMutation, error) {
	if !record.IsValid() ||
		record.Status() != execution.NodeExecutionStatusRunning {
		return NodeAttemptMutation{}, newValidationError(
			"attemptRecord", "must be a valid RUNNING attempt record")
	}
	return NodeAttemptMutation{
		kind:  NodeAttemptMutationStart,
		start: record,
	}, nil
}

func NewNodeAttemptCompleteMutation(
	completion NodeAttemptCompletion,
) (NodeAttemptMutation, error) {
	if !completion.IsValid() {
		return NodeAttemptMutation{}, newValidationError(
			"attemptCompletion", "must be valid")
	}
	return NodeAttemptMutation{
		kind:       NodeAttemptMutationComplete,
		completion: completion,
	}, nil
}

func (mutation NodeAttemptMutation) Kind() NodeAttemptMutationKind {
	return mutation.kind
}

func (mutation NodeAttemptMutation) Start() (NodeExecutionAttemptRecord, bool) {
	if mutation.kind != NodeAttemptMutationStart {
		return NodeExecutionAttemptRecord{}, false
	}
	return mutation.start, true
}

func (mutation NodeAttemptMutation) Completion() (NodeAttemptCompletion, bool) {
	if mutation.kind != NodeAttemptMutationComplete {
		return NodeAttemptCompletion{}, false
	}
	return mutation.completion, true
}

func (mutation NodeAttemptMutation) IsValid() bool {
	switch mutation.kind {
	case NodeAttemptMutationStart:
		return mutation.start.IsValid() &&
			mutation.start.Status() == execution.NodeExecutionStatusRunning
	case NodeAttemptMutationComplete:
		return mutation.completion.IsValid()
	default:
		return false
	}
}

func validateNodeTransitionAttemptMutation(
	companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID,
	expectedStatus execution.NodeExecutionStatus,
	expectedAttempt int16,
	nodeExecution NodeExecutionRecord,
	transitionAt time.Time,
	mutation NodeAttemptMutation,
	hasMutation bool,
) (NodeAttemptMutation, error) {
	if !hasMutation {
		if mutation.Kind() != "" {
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "must be empty when absent")
		}
		return NodeAttemptMutation{}, nil
	}
	if !mutation.IsValid() {
		return NodeAttemptMutation{}, newValidationError(
			"attemptMutation", "must be valid when provided")
	}
	switch mutation.Kind() {
	case NodeAttemptMutationStart:
		record, _ := mutation.Start()
		if nodeExecution.Status() != execution.NodeExecutionStatusRunning {
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "START requires a RUNNING target node")
		}
		switch expectedStatus {
		case execution.NodeExecutionStatusReady,
			execution.NodeExecutionStatusQueued,
			execution.NodeExecutionStatusRetryPending:
		default:
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "START is not allowed for the node transition")
		}
		if err := validateNodeAttemptMutationIdentity(
			companyID, workflowExecutionID, nodeExecutionID,
			record.CompanyID(), record.WorkflowExecutionID(), record.NodeExecutionID(),
		); err != nil {
			return NodeAttemptMutation{}, err
		}
		if record.Attempt().Int16() != nodeExecution.Attempt() {
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "START attempt must match the target node attempt")
		}
		if !record.CreatedAt().Equal(transitionAt) ||
			!record.StartedAt().Equal(transitionAt) ||
			!record.UpdatedAt().Equal(transitionAt) {
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "START timestamps must equal the node transition time")
		}
	case NodeAttemptMutationComplete:
		completion, _ := mutation.Completion()
		if expectedStatus != execution.NodeExecutionStatusRunning {
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "COMPLETE requires a RUNNING source node")
		}
		if err := validateNodeAttemptMutationIdentity(
			companyID, workflowExecutionID, nodeExecutionID,
			completion.CompanyID(), completion.WorkflowExecutionID(), completion.NodeExecutionID(),
		); err != nil {
			return NodeAttemptMutation{}, err
		}
		if completion.ExpectedAttempt().Int16() != expectedAttempt {
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "COMPLETE attempt must match expectedNodeAttempt")
		}
		if !completion.FinishedAt().Equal(transitionAt) {
			return NodeAttemptMutation{}, newValidationError(
				"attemptMutation", "COMPLETE time must equal the node transition time")
		}
		if err := validateNodeAttemptCompletionTarget(
			nodeExecution, completion,
		); err != nil {
			return NodeAttemptMutation{}, err
		}
	default:
		return NodeAttemptMutation{}, newValidationError(
			"attemptMutation", "contains an unsupported operation")
	}
	return mutation, nil
}

func validateNodeAttemptMutationIdentity(
	expectedCompanyID workflow.CompanyID,
	expectedWorkflowExecutionID execution.WorkflowExecutionID,
	expectedNodeExecutionID execution.NodeExecutionID,
	actualCompanyID workflow.CompanyID,
	actualWorkflowExecutionID execution.WorkflowExecutionID,
	actualNodeExecutionID execution.NodeExecutionID,
) error {
	if actualCompanyID != expectedCompanyID ||
		actualWorkflowExecutionID != expectedWorkflowExecutionID ||
		actualNodeExecutionID != expectedNodeExecutionID {
		return newValidationError(
			"attemptMutation", "identity must match the node transition command")
	}
	return nil
}

func validateNodeAttemptCompletionTarget(
	nodeExecution NodeExecutionRecord,
	completion NodeAttemptCompletion,
) error {
	switch nodeExecution.Status() {
	case execution.NodeExecutionStatusRetryPending:
		if completion.Status() != execution.NodeExecutionStatusFailed &&
			completion.Status() != execution.NodeExecutionStatusTimedOut {
			return newValidationError(
				"attemptMutation", "retry scheduling must complete a FAILED or TIMED_OUT attempt")
		}
		decision, exists := completion.RetryDecision()
		if !exists || decision.Kind() != execution.RetryDecisionRetry {
			return newValidationError(
				"attemptMutation", "retry scheduling requires a RETRY decision")
		}
		nextAttempt, exists := decision.NextAttempt()
		if !exists ||
			nextAttempt.Int16() != nodeExecution.Attempt()+1 {
			return newValidationError(
				"attemptMutation", "retry decision next attempt must match the node retry plan")
		}
		nodeNextAttemptAt, exists := nodeExecution.NextAttemptAt()
		completionNextAttemptAt, completionHasNext := completion.NextAttemptAt()
		if !exists || !completionHasNext ||
			!completionNextAttemptAt.Equal(nodeNextAttemptAt) {
			return newValidationError(
				"attemptMutation", "retry schedule must match the target node")
		}
	case execution.NodeExecutionStatusSucceeded:
		if completion.Status() != execution.NodeExecutionStatusSucceeded {
			return newValidationError(
				"attemptMutation", "attempt status must match SUCCEEDED node status")
		}
	case execution.NodeExecutionStatusFailed:
		if completion.Status() != execution.NodeExecutionStatusFailed {
			return newValidationError(
				"attemptMutation", "attempt status must match FAILED node status")
		}
	case execution.NodeExecutionStatusCancelled:
		if completion.Status() != execution.NodeExecutionStatusCancelled {
			return newValidationError(
				"attemptMutation", "attempt status must match CANCELLED node status")
		}
	case execution.NodeExecutionStatusTimedOut:
		if completion.Status() != execution.NodeExecutionStatusTimedOut {
			return newValidationError(
				"attemptMutation", "attempt status must match TIMED_OUT node status")
		}
	default:
		return newValidationError(
			"attemptMutation", "COMPLETE is not allowed for the node transition")
	}
	if !optionalJSONObjectEqual(
		nodeExecution.OutputSummary, completion.OutputSummary,
	) || !optionalJSONObjectEqual(
		nodeExecution.FailureSummary, completion.FailureSummary,
	) {
		return newValidationError(
			"attemptMutation", "summaries must match the target node record")
	}
	return nil
}

func optionalJSONObjectEqual(
	left func() (JSONObject, bool),
	right func() (JSONObject, bool),
) bool {
	leftValue, leftExists := left()
	rightValue, rightExists := right()
	if leftExists != rightExists {
		return false
	}
	return !leftExists || leftValue.String() == rightValue.String()
}
