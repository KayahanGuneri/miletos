package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"miletos-go/internal/engine/graph"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
	sharedclock "miletos-go/internal/shared/clock"
)

const (
	interruptedAttemptFailureCode    = "INTERRUPTED_ATTEMPT"
	interruptedAttemptFailureMessage = "Node attempt was interrupted after worker ownership was lost"
)

var errInterruptedAttemptCandidateUnsafe = errors.New(
	"interrupted attempt candidate is unsafe")

func newInterruptedAttemptCandidateUnsafeError(cause error) error {
	if cause == nil {
		return errInterruptedAttemptCandidateUnsafe
	}
	return fmt.Errorf("%w: %v", errInterruptedAttemptCandidateUnsafe, cause)
}

func isInterruptedAttemptCandidateLocal(err error) bool {
	return repository.IsStaleWrite(err) ||
		repository.IsNotFound(err) ||
		errors.Is(err, errInterruptedAttemptCandidateUnsafe)
}

type InterruptedAttemptAuditPersistence interface {
	repository.InterruptedAttemptStore
	repository.WorkflowExecutionReader
	repository.WorkflowSnapshotRepository
}

type InterruptedAttemptAuditResult struct {
	Candidates int
	Active     int
	Finalized  int
	Stale      int
}

type InterruptedAttemptAuditor struct {
	persistence InterruptedAttemptAuditPersistence
	clock       sharedclock.Clock
	interval    time.Duration
	batchLimit  int
}

func NewInterruptedAttemptAuditor(
	persistence InterruptedAttemptAuditPersistence,
	clock sharedclock.Clock,
	interval time.Duration,
	batchLimit int,
) (*InterruptedAttemptAuditor, error) {
	if persistence == nil || clock == nil {
		return nil, fmt.Errorf("interrupted attempt auditor dependencies must be valid")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("interrupted attempt audit interval must be greater than zero")
	}
	request, err := repository.NewInterruptedAttemptAuditRequest(batchLimit)
	if err != nil {
		return nil, fmt.Errorf("interrupted attempt audit batch limit: %w", err)
	}
	return &InterruptedAttemptAuditor{
		persistence: persistence, clock: clock,
		interval: interval, batchLimit: request.BatchLimit(),
	}, nil
}

func (auditor *InterruptedAttemptAuditor) Run(ctx context.Context) error {
	if auditor == nil || auditor.persistence == nil || auditor.clock == nil ||
		auditor.interval <= 0 || auditor.batchLimit <= 0 {
		return fmt.Errorf("interrupted attempt auditor must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("interrupted attempt audit context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ticker := time.NewTicker(auditor.interval)
	defer ticker.Stop()
	for {
		if _, err := auditor.AuditBatch(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (auditor *InterruptedAttemptAuditor) AuditBatch(
	ctx context.Context,
) (InterruptedAttemptAuditResult, error) {
	if auditor == nil || auditor.persistence == nil || auditor.clock == nil {
		return InterruptedAttemptAuditResult{},
			fmt.Errorf("interrupted attempt auditor must be valid")
	}
	if ctx == nil {
		return InterruptedAttemptAuditResult{},
			fmt.Errorf("interrupted attempt audit context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return InterruptedAttemptAuditResult{}, err
	}
	request, err := repository.NewInterruptedAttemptAuditRequest(auditor.batchLimit)
	if err != nil {
		return InterruptedAttemptAuditResult{}, err
	}
	candidates, err := auditor.persistence.ListInterruptedAttemptCandidates(ctx, request)
	if err != nil {
		return InterruptedAttemptAuditResult{},
			fmt.Errorf("list interrupted attempt candidates: %w", err)
	}
	result := InterruptedAttemptAuditResult{Candidates: len(candidates)}
	for _, candidate := range candidates {
		if !candidate.IsValid() {
			result.Stale++
			continue
		}
		acquired, err := auditor.persistence.TryWithInterruptedAttemptOwnership(
			ctx, candidate, func(lockContext context.Context) error {
				return auditor.finalizeCandidate(lockContext, candidate)
			})
		if err != nil {
			if isInterruptedAttemptCandidateLocal(err) {
				result.Stale++
				continue
			}
			return result, fmt.Errorf(
				"audit interrupted attempt %s attempt %d: %w",
				candidate.NodeExecutionID(), candidate.Attempt().Int16(), err)
		}
		if acquired {
			result.Finalized++
		} else {
			result.Active++
		}
	}
	return result, nil
}

func (auditor *InterruptedAttemptAuditor) finalizeCandidate(
	ctx context.Context,
	candidate repository.InterruptedAttemptCandidate,
) error {
	workflowRecord, err := auditor.persistence.GetWorkflowExecution(
		ctx, candidate.CompanyID(), candidate.WorkflowExecutionID())
	if err != nil {
		return err
	}
	if workflowRecord.Mode() != execution.ExecutionModeAsync ||
		workflowRecord.Status() != execution.WorkflowExecutionStatusRunning {
		return repository.NewStaleWriteError(
			"audit", "interrupted workflow execution",
			errors.New("workflow is no longer a running async execution"))
	}
	snapshot, err := auditor.persistence.GetByID(
		ctx, candidate.CompanyID(), workflowRecord.SnapshotID())
	if err != nil {
		return err
	}
	definition, err := workflow.DecodePersistedDefinition(snapshot.DefinitionJSON().Bytes())
	if err != nil {
		return newInterruptedAttemptCandidateUnsafeError(
			fmt.Errorf("decode persisted workflow definition: %w", err))
	}
	if definition.CompanyID() != candidate.CompanyID() ||
		definition.ID() != workflowRecord.WorkflowID() ||
		definition.Revision() != workflowRecord.WorkflowRevision() {
		return newInterruptedAttemptCandidateUnsafeError(
			errors.New("persisted workflow definition identity is inconsistent"))
	}
	builtGraph, report := graph.BuildValidated(definition)
	if !report.IsValid() {
		return newInterruptedAttemptCandidateUnsafeError(
			errors.New("persisted workflow graph is invalid"))
	}
	node, exists := builtGraph.Node(candidate.NodeID())
	if !exists || node.ID() != candidate.NodeID() {
		return newInterruptedAttemptCandidateUnsafeError(
			errors.New("interrupted node does not belong to the persisted graph"))
	}
	descendants := interruptedNodeDescendants(builtGraph, candidate.NodeID())
	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryTimeout,
		interruptedAttemptFailureCode,
		interruptedAttemptFailureMessage,
		false,
		map[string]string{"ownership": "lost"},
	)
	if err != nil {
		return err
	}
	retryDecision, err := execution.NewRetryDecision(
		execution.RetryDecisionDoNotRetry,
		execution.RetryReasonCategoryNotRetryable,
		candidate.Attempt(),
		0,
		0,
	)
	if err != nil {
		return err
	}
	interruptedAt := auditor.clock.Now().UTC()
	if interruptedAt.Before(candidate.AttemptStartedAt()) {
		interruptedAt = candidate.AttemptStartedAt()
	}
	finalization, err := repository.NewInterruptedAttemptFinalization(
		repository.InterruptedAttemptFinalizationParams{
			Candidate: candidate, DescendantNodeIDs: descendants,
			Failure: failure, RetryDecision: retryDecision,
			InterruptedAt: interruptedAt,
		})
	if err != nil {
		return err
	}
	return auditor.persistence.FinalizeInterruptedAttempt(ctx, finalization)
}

func interruptedNodeDescendants(
	builtGraph graph.Graph,
	sourceNodeID workflow.NodeID,
) []workflow.NodeID {
	queue := []workflow.NodeID{sourceNodeID}
	seen := map[workflow.NodeID]struct{}{sourceNodeID: {}}
	descendants := make([]workflow.NodeID, 0)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range builtGraph.OutgoingEdges(current) {
			target := edge.TargetNodeID()
			if _, exists := seen[target]; exists {
				continue
			}
			seen[target] = struct{}{}
			descendants = append(descendants, target)
			queue = append(queue, target)
		}
	}
	sort.Slice(descendants, func(left int, right int) bool {
		return descendants[left].String() < descendants[right].String()
	})
	return descendants
}
