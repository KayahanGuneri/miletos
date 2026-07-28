package repository

import (
	"context"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"time"
)

type OutboxStore interface {
	CreateOutboxMessage(ctx context.Context, message OutboxMessage,
	) error
}

// OutboxPublicationStore owns the durable publication lease lifecycle. Claim
// must atomically move a deterministic, bounded PENDING selection to
// PUBLISHING, increment lock versions and return those new versions. Concurrent
// claims must not overlap. Mark must only publish a PUBLISHING message whose
// owner and version both match, then clear the lease and increment its version.
// A missing message uses the repository not-found error; an owner, state or
// version mismatch uses the repository stale-write error and must not expose a
// database error. Persistence constraint violations use the repository
// conflict error.
// Release is infrastructure recovery: it only requeues a deterministic,
// bounded selection with claimed_at before the requested cutoff and never
// changes PUBLISHED messages.
type OutboxPublicationStore interface {
	ClaimPublishableOutboxMessages(ctx context.Context,
		request OutboxClaimRequest) ([]OutboxMessage, error)
	MarkOutboxMessagePublished(ctx context.Context, command MarkOutboxPublishedCommand,
	) error
	ReleaseStaleOutboxClaims(
		ctx context.Context, request ReleaseStaleOutboxClaimsRequest) (int64, error)
}

// InboxStore records a successfully processed delivery. RecordInboxMessage is
// intended to run in the same transaction as its business-state changes.
type InboxStore interface {
	HasProcessedInboxMessage(
		ctx context.Context, consumerIdentity ConsumerIdentity, messageID MessageID,
	) (bool, error)
	RecordInboxMessage(
		ctx context.Context, message InboxMessage) error
}

type AsyncInputStore interface {
	CreateAsyncNodeInput(ctx context.Context, input AsyncNodeInput,
	) error
	ListAsyncNodeInputs(
		ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
		targetNodeExecutionID execution.NodeExecutionID) ([]AsyncNodeInput, error)
}

type DurableWorkerResultStore interface {
	CreateDurableWorkerResult(
		ctx context.Context, result DurableWorkerResult) error
	GetDurableWorkerResult(ctx context.Context,
		companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, nodeExecutionID execution.NodeExecutionID,
		attempt int16) (DurableWorkerResult, error)
}

// AsyncContextStore uses compare-and-swap semantics. ExpectedVersion zero is
// insert-only; a positive expected version is update/delete-only.
type AsyncContextStore interface {
	ListAsyncContextVariables(ctx context.Context,
		companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID) ([]AsyncContextVariable, error)
	GetAsyncContextVariable(ctx context.Context,
		companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, key string,
	) (AsyncContextVariable, error)
	CompareAndSwapAsyncContextVariable(
		ctx context.Context, write AsyncContextWrite) error
	DeleteAsyncContextVariable(ctx context.Context,
		deletion AsyncContextDelete) error
}

// AsyncPersistenceTransaction is the capability set available inside one
// database transaction. It contains no driver or transport-specific types.
type AsyncPersistenceTransaction interface {
	OutboxStore
	OutboxPublicationStore
	InboxStore
	AsyncInputStore
	DurableWorkerResultStore
	AsyncContextStore
	ScheduleAsyncNodeExecution(ctx context.Context, schedule AsyncNodeSchedule) error
	ClaimAsyncNodeExecution(ctx context.Context, claim AsyncNodeClaim) (int64, error)
	PublishAsyncWorkerResult(
		ctx context.Context, publication AsyncWorkerResultPublication,
	) error
	ApplyAsyncNodeResult(
		ctx context.Context, application AsyncNodeResultApplication,
	) error
	LockAsyncWorkflowCoordination(ctx context.Context,
		companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID) error
	LockAsyncNodeCoordination(ctx context.Context,
		companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, nodeID workflow.NodeID,
	) error
	GetAsyncWorkflowExecution(
		ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	) (WorkflowExecutionRecord, error)
	GetAsyncNodeExecution(
		ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
		nodeExecutionID execution.NodeExecutionID) (NodeExecutionRecord, error)
	ListAsyncNodeExecutions(ctx context.Context, companyID workflow.CompanyID,
		workflowExecutionID execution.WorkflowExecutionID) ([]NodeExecutionRecord, error)
	SkipAsyncNodeExecution(ctx context.Context, companyID workflow.CompanyID,
		workflowExecutionID execution.WorkflowExecutionID, nodeExecutionID execution.NodeExecutionID, expectedLockVersion int64,
		finishedAt time.Time) error
	CompleteAsyncWorkflow(ctx context.Context, completion AsyncWorkflowCompletion,
	) error
}

type AsyncPersistenceWork func(ctx context.Context, transaction AsyncPersistenceTransaction,
) error

// AsyncPersistenceTransactor owns begin/commit/rollback. A returned work error
// must roll back every inbox, business-state, payload, context and outbox write.
type AsyncPersistenceTransactor interface {
	WithinAsyncPersistenceTransaction(
		ctx context.Context, work AsyncPersistenceWork) error
}

type AsyncNodeExecutionLock struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeExecutionID     execution.NodeExecutionID
	Attempt             int16
}

func (lock AsyncNodeExecutionLock) IsValid() bool {
	return lock.CompanyID.String() != "" && lock.WorkflowExecutionID.String() != "" && lock.NodeExecutionID.String() != "" && lock.Attempt > 0
}

type AsyncNodeExecutionLockWork func(context.Context) error

type AsyncNodeExecutionLocker interface {
	WithAsyncNodeExecutionLock(context.Context, AsyncNodeExecutionLock, AsyncNodeExecutionLockWork) error
}

type AsyncNodeClaim struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeExecutionID     execution.NodeExecutionID
	ExpectedNodeStatus  execution.NodeExecutionStatus
	ExpectedNodeAttempt int16
	CommandAttempt      int16
	ExpectedLockVersion int64
	StartedAt           time.Time
}

func (claim AsyncNodeClaim) IsValid() bool {
	if claim.CompanyID.String() == "" ||
		claim.WorkflowExecutionID.String() == "" ||
		claim.NodeExecutionID.String() == "" ||
		claim.ExpectedNodeAttempt <= 0 ||
		claim.CommandAttempt <= 0 ||
		claim.ExpectedLockVersion < 0 ||
		claim.StartedAt.IsZero() {
		return false
	}
	switch claim.ExpectedNodeStatus {
	case execution.NodeExecutionStatusQueued:
		return claim.CommandAttempt == claim.ExpectedNodeAttempt
	case execution.NodeExecutionStatusRetryPending:
		currentAttempt, err := execution.NewAttemptNumber(
			claim.ExpectedNodeAttempt,
		)
		if err != nil {
			return false
		}
		nextAttempt, err := currentAttempt.Next()
		return err == nil &&
			claim.CommandAttempt == nextAttempt.Int16()
	default:
		return false
	}
}

type AsyncWorkerResultPublication struct {
	CompanyID               workflow.CompanyID
	WorkflowExecutionID     execution.WorkflowExecutionID
	NodeExecutionID         execution.NodeExecutionID
	ExpectedNodeLockVersion int64
	DurableWorkerResult     DurableWorkerResult
	NodeResultOutboxMessage OutboxMessage
	CommandInboxMessage     InboxMessage
}

func (publication AsyncWorkerResultPublication) IsValid() bool {
	if publication.CompanyID.String() == "" ||
		publication.WorkflowExecutionID.String() == "" ||
		publication.NodeExecutionID.String() == "" ||
		publication.ExpectedNodeLockVersion < 0 ||
		!publication.DurableWorkerResult.IsValid() ||
		!publication.NodeResultOutboxMessage.IsValid() ||
		!publication.CommandInboxMessage.IsValid() {
		return false
	}
	result := publication.DurableWorkerResult
	if result.CompanyID() != publication.CompanyID ||
		result.WorkflowExecutionID() != publication.WorkflowExecutionID ||
		result.NodeExecutionID() != publication.NodeExecutionID {
		return false
	}
	outbox := publication.NodeResultOutboxMessage
	outboxNodeExecutionID, hasOutboxNodeExecutionID :=
		outbox.NodeExecutionID()
	outboxNodeID, hasOutboxNodeID := outbox.NodeID()
	outboxAttempt, hasOutboxAttempt := outbox.Attempt()
	if !hasOutboxNodeExecutionID || !hasOutboxNodeID ||
		!hasOutboxAttempt ||
		outbox.OperationKind() != OutboxOperationNodeResult ||
		outbox.PublicationState() != OutboxPublicationPending ||
		outbox.CompanyID() != publication.CompanyID ||
		outbox.WorkflowExecutionID() != publication.WorkflowExecutionID ||
		outboxNodeExecutionID != publication.NodeExecutionID ||
		outboxNodeID != result.NodeID() ||
		outboxAttempt != result.Attempt() ||
		!outbox.CreatedAt().Equal(result.FinishedAt()) ||
		!outbox.AvailableAt().Equal(outbox.CreatedAt()) {
		return false
	}
	inbox := publication.CommandInboxMessage
	inboxNodeExecutionID, hasInboxNodeExecutionID :=
		inbox.NodeExecutionID()
	return hasInboxNodeExecutionID &&
		inbox.ProcessingResult() == InboxProcessingApplied &&
		inbox.CompanyID() == publication.CompanyID &&
		inbox.WorkflowExecutionID() == publication.WorkflowExecutionID &&
		inboxNodeExecutionID == publication.NodeExecutionID
}

type AsyncNodeResultApplication struct {
	CompanyID               workflow.CompanyID
	WorkflowExecutionID     execution.WorkflowExecutionID
	NodeExecutionID         execution.NodeExecutionID
	ExpectedNodeLockVersion int64
	ExpectedNodeAttempt     int16
	TargetNodeStatus        execution.NodeExecutionStatus
	AppliedAt               time.Time
	DurableWorkerResult     DurableWorkerResult
	AttemptMutation         NodeAttemptMutation
	RetryCommandOutbox      OutboxMessage
	ResultInboxMessage      InboxMessage
}

func (application AsyncNodeResultApplication) IsValid() bool {
	if application.CompanyID.String() == "" ||
		application.WorkflowExecutionID.String() == "" ||
		application.NodeExecutionID.String() == "" ||
		application.ExpectedNodeLockVersion < 0 ||
		application.ExpectedNodeAttempt <= 0 ||
		application.AppliedAt.IsZero() ||
		!application.DurableWorkerResult.IsValid() ||
		!application.AttemptMutation.IsValid() ||
		!application.ResultInboxMessage.IsValid() {
		return false
	}
	result := application.DurableWorkerResult
	if result.CompanyID() != application.CompanyID ||
		result.WorkflowExecutionID() != application.WorkflowExecutionID ||
		result.NodeExecutionID() != application.NodeExecutionID ||
		result.Attempt() != application.ExpectedNodeAttempt ||
		application.AppliedAt.Before(result.FinishedAt()) {
		return false
	}
	completion, exists := application.AttemptMutation.Completion()
	if !exists ||
		completion.CompanyID() != application.CompanyID ||
		completion.WorkflowExecutionID() != application.WorkflowExecutionID ||
		completion.NodeExecutionID() != application.NodeExecutionID ||
		completion.ExpectedAttempt().Int16() != application.ExpectedNodeAttempt ||
		!completion.FinishedAt().Equal(result.FinishedAt()) {
		return false
	}
	if !asyncResultTargetMatchesAttempt(
		application.TargetNodeStatus, completion.Status(),
	) || !asyncResultStatusMatchesAttempt(result.Status(), completion.Status()) {
		return false
	}
	decision, hasDecision := completion.RetryDecision()
	nextAttemptAt, hasNextAttemptAt := completion.NextAttemptAt()
	if application.TargetNodeStatus == execution.NodeExecutionStatusRetryPending {
		if !hasDecision ||
			decision.Kind() != execution.RetryDecisionRetry ||
			!hasNextAttemptAt ||
			!application.RetryCommandOutbox.IsValid() {
			return false
		}
		nextAttempt, exists := decision.NextAttempt()
		outboxNodeExecutionID, hasOutboxNodeExecutionID :=
			application.RetryCommandOutbox.NodeExecutionID()
		outboxNodeID, hasOutboxNodeID :=
			application.RetryCommandOutbox.NodeID()
		outboxAttempt, hasOutboxAttempt :=
			application.RetryCommandOutbox.Attempt()
		if !exists || !hasOutboxNodeExecutionID ||
			!hasOutboxNodeID || !hasOutboxAttempt ||
			application.RetryCommandOutbox.OperationKind() != OutboxOperationNodeCommand ||
			application.RetryCommandOutbox.PublicationState() != OutboxPublicationPending ||
			application.RetryCommandOutbox.CompanyID() != application.CompanyID ||
			application.RetryCommandOutbox.WorkflowExecutionID() != application.WorkflowExecutionID ||
			outboxNodeExecutionID != application.NodeExecutionID ||
			outboxNodeID != result.NodeID() ||
			outboxAttempt != nextAttempt.Int16() ||
			!application.RetryCommandOutbox.CreatedAt().Equal(application.AppliedAt) ||
			!application.RetryCommandOutbox.AvailableAt().Equal(nextAttemptAt) {
			return false
		}
	} else {
		if hasNextAttemptAt ||
			(hasDecision &&
				decision.Kind() == execution.RetryDecisionRetry) ||
			application.RetryCommandOutbox.IsValid() {
			return false
		}
	}
	inboxNodeExecutionID, hasInboxNodeExecutionID :=
		application.ResultInboxMessage.NodeExecutionID()
	return hasInboxNodeExecutionID &&
		application.ResultInboxMessage.ProcessingResult() == InboxProcessingApplied &&
		application.ResultInboxMessage.CompanyID() == application.CompanyID &&
		application.ResultInboxMessage.WorkflowExecutionID() == application.WorkflowExecutionID &&
		inboxNodeExecutionID == application.NodeExecutionID
}

func asyncResultTargetMatchesAttempt(
	target execution.NodeExecutionStatus,
	attemptStatus execution.NodeExecutionStatus,
) bool {
	switch target {
	case execution.NodeExecutionStatusRetryPending:
		return attemptStatus == execution.NodeExecutionStatusFailed ||
			attemptStatus == execution.NodeExecutionStatusTimedOut
	case execution.NodeExecutionStatusSucceeded,
		execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusCancelled,
		execution.NodeExecutionStatusTimedOut:
		return target == attemptStatus
	default:
		return false
	}
}

func asyncResultStatusMatchesAttempt(
	status DurableWorkerResultStatus,
	attemptStatus execution.NodeExecutionStatus,
) bool {
	switch status {
	case DurableWorkerResultSucceeded:
		return attemptStatus == execution.NodeExecutionStatusSucceeded
	case DurableWorkerResultFailed:
		return attemptStatus == execution.NodeExecutionStatusFailed ||
			attemptStatus == execution.NodeExecutionStatusTimedOut
	case DurableWorkerResultCancelled:
		return attemptStatus == execution.NodeExecutionStatusCancelled
	case DurableWorkerResultTimedOut:
		return attemptStatus == execution.NodeExecutionStatusTimedOut
	default:
		return false
	}
}
