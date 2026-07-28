package repository

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

const MaximumInterruptedAttemptAuditBatchLimit = 1000

type InterruptedAttemptAuditRequest struct {
	batchLimit int
}

func NewInterruptedAttemptAuditRequest(batchLimit int) (InterruptedAttemptAuditRequest, error) {
	if batchLimit <= 0 || batchLimit > MaximumInterruptedAttemptAuditBatchLimit {
		return InterruptedAttemptAuditRequest{},
			newValidationError("batchLimit", "must be between 1 and 1000")
	}
	return InterruptedAttemptAuditRequest{batchLimit: batchLimit}, nil
}

func (request InterruptedAttemptAuditRequest) BatchLimit() int {
	return request.batchLimit
}

func (request InterruptedAttemptAuditRequest) IsValid() bool {
	_, err := NewInterruptedAttemptAuditRequest(request.batchLimit)
	return err == nil
}

type InterruptedAttemptCandidateParams struct {
	CompanyID               workflow.CompanyID
	WorkflowExecutionID     execution.WorkflowExecutionID
	NodeExecutionID         execution.NodeExecutionID
	NodeID                  workflow.NodeID
	Attempt                 execution.AttemptNumber
	ExpectedNodeLockVersion int64
	AttemptStartedAt        time.Time
}

type InterruptedAttemptCandidate struct {
	params InterruptedAttemptCandidateParams
}

func NewInterruptedAttemptCandidate(
	params InterruptedAttemptCandidateParams,
) (InterruptedAttemptCandidate, error) {
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return InterruptedAttemptCandidate{}, newValidationError("companyID", err.Error())
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(
		params.WorkflowExecutionID.String())
	if err != nil {
		return InterruptedAttemptCandidate{},
			newValidationError("workflowExecutionID", err.Error())
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(params.NodeExecutionID.String())
	if err != nil {
		return InterruptedAttemptCandidate{},
			newValidationError("nodeExecutionID", err.Error())
	}
	nodeID, err := workflow.NewNodeID(params.NodeID.String())
	if err != nil {
		return InterruptedAttemptCandidate{}, newValidationError("nodeID", err.Error())
	}
	attempt, err := execution.NewAttemptNumber(params.Attempt.Int16())
	if err != nil {
		return InterruptedAttemptCandidate{}, newValidationError("attempt", err.Error())
	}
	if params.ExpectedNodeLockVersion < 0 {
		return InterruptedAttemptCandidate{},
			newValidationError("expectedNodeLockVersion", "must not be negative")
	}
	attemptStartedAt, err := normalizeRequiredRecordTime(
		"attemptStartedAt", params.AttemptStartedAt)
	if err != nil {
		return InterruptedAttemptCandidate{}, err
	}
	return InterruptedAttemptCandidate{params: InterruptedAttemptCandidateParams{
		CompanyID: companyID, WorkflowExecutionID: workflowExecutionID,
		NodeExecutionID: nodeExecutionID, NodeID: nodeID, Attempt: attempt,
		ExpectedNodeLockVersion: params.ExpectedNodeLockVersion,
		AttemptStartedAt:        attemptStartedAt,
	}}, nil
}

func (candidate InterruptedAttemptCandidate) CompanyID() workflow.CompanyID {
	return candidate.params.CompanyID
}
func (candidate InterruptedAttemptCandidate) WorkflowExecutionID() execution.WorkflowExecutionID {
	return candidate.params.WorkflowExecutionID
}
func (candidate InterruptedAttemptCandidate) NodeExecutionID() execution.NodeExecutionID {
	return candidate.params.NodeExecutionID
}
func (candidate InterruptedAttemptCandidate) NodeID() workflow.NodeID {
	return candidate.params.NodeID
}
func (candidate InterruptedAttemptCandidate) Attempt() execution.AttemptNumber {
	return candidate.params.Attempt
}
func (candidate InterruptedAttemptCandidate) ExpectedNodeLockVersion() int64 {
	return candidate.params.ExpectedNodeLockVersion
}
func (candidate InterruptedAttemptCandidate) AttemptStartedAt() time.Time {
	return candidate.params.AttemptStartedAt
}
func (candidate InterruptedAttemptCandidate) IsValid() bool {
	_, err := NewInterruptedAttemptCandidate(candidate.params)
	return err == nil
}

type InterruptedAttemptFinalizationParams struct {
	Candidate         InterruptedAttemptCandidate
	DescendantNodeIDs []workflow.NodeID
	Failure           runtime.RuntimeFailure
	RetryDecision     execution.RetryDecision
	InterruptedAt     time.Time
}

type InterruptedAttemptFinalization struct {
	params         InterruptedAttemptFinalizationParams
	failureSummary JSONObject
}

func NewInterruptedAttemptFinalization(
	params InterruptedAttemptFinalizationParams,
) (InterruptedAttemptFinalization, error) {
	if !params.Candidate.IsValid() {
		return InterruptedAttemptFinalization{},
			newValidationError("candidate", "must be valid")
	}
	if !params.Failure.IsValid() ||
		params.Failure.Category() != runtime.FailureCategoryTimeout ||
		params.Failure.Retryable() {
		return InterruptedAttemptFinalization{}, newValidationError(
			"failure", "must be a valid non-retryable timeout failure")
	}
	if !params.RetryDecision.IsValid() ||
		params.RetryDecision.Kind() != execution.RetryDecisionDoNotRetry ||
		params.RetryDecision.Reason() != execution.RetryReasonCategoryNotRetryable ||
		params.RetryDecision.CurrentAttempt() != params.Candidate.Attempt() {
		return InterruptedAttemptFinalization{}, newValidationError(
			"retryDecision",
			"must reject automatic retry for the interrupted attempt",
		)
	}
	interruptedAt, err := normalizeRequiredRecordTime("interruptedAt", params.InterruptedAt)
	if err != nil {
		return InterruptedAttemptFinalization{}, err
	}
	if interruptedAt.Before(params.Candidate.AttemptStartedAt()) {
		return InterruptedAttemptFinalization{}, newValidationError(
			"interruptedAt", "must not be before the attempt start")
	}
	descendants := append([]workflow.NodeID(nil), params.DescendantNodeIDs...)
	sort.Slice(descendants, func(left int, right int) bool {
		return descendants[left].String() < descendants[right].String()
	})
	seen := make(map[workflow.NodeID]struct{}, len(descendants))
	for index, descendant := range descendants {
		normalized, err := workflow.NewNodeID(descendant.String())
		if err != nil {
			return InterruptedAttemptFinalization{},
				newValidationError("descendantNodeIDs", err.Error())
		}
		if normalized == params.Candidate.NodeID() {
			return InterruptedAttemptFinalization{}, newValidationError(
				"descendantNodeIDs", "must not contain the interrupted node")
		}
		if _, exists := seen[normalized]; exists {
			return InterruptedAttemptFinalization{}, newValidationError(
				"descendantNodeIDs", "must not contain duplicates")
		}
		seen[normalized] = struct{}{}
		descendants[index] = normalized
	}
	encodedSummary, err := json.Marshal(struct {
		Category  string `json:"category"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	}{
		Category: params.Failure.Category().String(),
		Code:     params.Failure.Code(), Message: params.Failure.Message(),
		Retryable: params.Failure.Retryable(),
	})
	if err != nil {
		return InterruptedAttemptFinalization{}, err
	}
	failureSummary, err := newJSONObject("failureSummary", encodedSummary)
	if err != nil {
		return InterruptedAttemptFinalization{}, err
	}
	return InterruptedAttemptFinalization{
		params: InterruptedAttemptFinalizationParams{
			Candidate: params.Candidate, DescendantNodeIDs: descendants,
			Failure: params.Failure, RetryDecision: params.RetryDecision,
			InterruptedAt: interruptedAt,
		},
		failureSummary: failureSummary,
	}, nil
}

func (finalization InterruptedAttemptFinalization) Candidate() InterruptedAttemptCandidate {
	return finalization.params.Candidate
}
func (finalization InterruptedAttemptFinalization) DescendantNodeIDs() []workflow.NodeID {
	return append([]workflow.NodeID(nil), finalization.params.DescendantNodeIDs...)
}
func (finalization InterruptedAttemptFinalization) Failure() runtime.RuntimeFailure {
	return finalization.params.Failure
}
func (finalization InterruptedAttemptFinalization) RetryDecision() execution.RetryDecision {
	return finalization.params.RetryDecision
}
func (finalization InterruptedAttemptFinalization) FailureSummary() JSONObject {
	return finalization.failureSummary
}
func (finalization InterruptedAttemptFinalization) InterruptedAt() time.Time {
	return finalization.params.InterruptedAt
}
func (finalization InterruptedAttemptFinalization) IsValid() bool {
	_, err := NewInterruptedAttemptFinalization(finalization.params)
	return err == nil
}

type InterruptedAttemptOwnershipWork func(context.Context) error

type InterruptedAttemptStore interface {
	ListInterruptedAttemptCandidates(
		context.Context,
		InterruptedAttemptAuditRequest,
	) ([]InterruptedAttemptCandidate, error)
	TryWithInterruptedAttemptOwnership(
		context.Context,
		InterruptedAttemptCandidate,
		InterruptedAttemptOwnershipWork,
	) (bool, error)
	FinalizeInterruptedAttempt(
		context.Context,
		InterruptedAttemptFinalization,
	) error
}
