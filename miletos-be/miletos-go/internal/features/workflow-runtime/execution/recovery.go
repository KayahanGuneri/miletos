package execution

import (
	"context"
	"errors"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
)

var ErrRecoveryUnsupported = errors.New("source execution is not eligible for recovery")

type RecoveryOutcome struct {
	SourceExecutionID   string
	RecoveryExecutionID string
	Status              model.ExecutionStatus
	PreservedNodeCount  int
	ScheduledNodeCount  int
	ResetNodeCount      int
	CreatedAt           time.Time
	Replayed            bool
}

type RecoveryService struct {
	workflows       *workflowfeature.WorkflowRepository
	executions      *repository.ExecutionRepository
	workflowService *workflowfeature.WorkflowService
	scheduler       *Scheduler
}

func NewRecoveryService(
	workflows *workflowfeature.WorkflowRepository,
	executions *repository.ExecutionRepository,
	workflowService *workflowfeature.WorkflowService,
	scheduler *Scheduler,
) *RecoveryService {
	return &RecoveryService{
		workflows: workflows, executions: executions,
		workflowService: workflowService, scheduler: scheduler,
	}
}

func (service *RecoveryService) Recover(
	ctx context.Context,
	companyID string,
	sourceExecutionID string,
	idempotencyKey string,
) (RecoveryOutcome, error) {
	fingerprint := recoveryFingerprint(companyID, sourceExecutionID)
	sourceID, recoveryID, storedFingerprint, preserved, scheduled, reset, createdAt, found, err :=
		service.executions.FindRecovery(ctx, companyID, idempotencyKey)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	if found {
		if sourceID != sourceExecutionID || storedFingerprint != fingerprint {
			return RecoveryOutcome{}, repository.ErrIdempotencyConflict
		}
	} else {
		existingKey, _, _, sourceFound, lookupErr :=
			service.executions.FindRecoveryBySource(ctx, companyID, sourceExecutionID)
		if lookupErr != nil {
			return RecoveryOutcome{}, lookupErr
		}
		if sourceFound && existingKey != idempotencyKey {
			return RecoveryOutcome{}, repository.ErrRecoveryConflict
		}
	}
	source, err := service.executions.FindByID(ctx, companyID, sourceExecutionID)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	if source.Status != model.ExecutionFailed &&
		source.Status != model.ExecutionCancelled &&
		source.Status != model.ExecutionTimedOut {
		return RecoveryOutcome{}, ErrRecoveryUnsupported
	}
	snapshot, err := service.workflows.FindByExecutionID(ctx, companyID, sourceExecutionID)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	states, err := service.executions.ListAllNodes(ctx, companyID, sourceExecutionID)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	failedIDs := make([]string, 0)
	outOfScopeIDs := make([]string, 0)
	outOfScope := make(map[string]bool)
	for _, state := range states {
		if state.Status == model.NodeSucceeded {
			continue
		}
		if nodeSkipReason(state) == string(model.SkipReasonOutOfTriggerScope) {
			outOfScope[state.NodeID] = true
			outOfScopeIDs = append(outOfScopeIDs, state.NodeID)
			continue
		}
		failedIDs = append(failedIDs, state.NodeID)
	}
	if len(failedIDs) == 0 {
		return RecoveryOutcome{}, ErrRecoveryUnsupported
	}
	rerun := workflowfeature.Downstream(snapshot.Workflow, failedIDs)
	preservedIDs := make([]string, 0)
	for _, state := range states {
		if state.Status == model.NodeSucceeded && !rerun[state.NodeID] && !outOfScope[state.NodeID] {
			preservedIDs = append(preservedIDs, state.NodeID)
		}
	}
	preserved = len(preservedIDs)
	reset = len(snapshot.Workflow.Nodes) - preserved - len(outOfScopeIDs)
	expectedScheduled := recoveryReadyCount(snapshot.Workflow, preservedIDs, outOfScope)
	if expectedScheduled == 0 {
		return RecoveryOutcome{}, ErrRecoveryUnsupported
	}
	replayed := found
	var recovery model.Execution
	if found {
		recovery, err = service.executions.FindByID(ctx, companyID, recoveryID)
	} else {
		recovery, err = service.executions.ReserveRecovery(
			ctx, snapshot.Workflow, source.CorrelationID, idempotencyKey, fingerprint,
			sourceExecutionID, preserved, expectedScheduled, reset,
		)
		if errors.Is(err, repository.ErrIdempotencyConflict) {
			concurrentSourceID, concurrentRecoveryID, concurrentFingerprint,
				concurrentPreserved, concurrentScheduled, concurrentReset,
				concurrentCreatedAt, concurrentFound, lookupErr :=
				service.executions.FindRecovery(ctx, companyID, idempotencyKey)
			if lookupErr != nil {
				return RecoveryOutcome{}, lookupErr
			}
			if !concurrentFound || concurrentSourceID != sourceExecutionID ||
				concurrentFingerprint != fingerprint {
				return RecoveryOutcome{}, repository.ErrIdempotencyConflict
			}
			recovery, err = service.executions.FindByID(
				ctx, companyID, concurrentRecoveryID,
			)
			preserved, scheduled, reset = concurrentPreserved, concurrentScheduled, concurrentReset
			createdAt, replayed = concurrentCreatedAt, true
		} else if errors.Is(err, repository.ErrRecoveryConflict) {
			return RecoveryOutcome{}, repository.ErrRecoveryConflict
		}
		recoveryID = recovery.ID
		if !replayed {
			createdAt = recovery.CreatedAt
			scheduled = expectedScheduled
		}
	}
	if err != nil {
		return RecoveryOutcome{}, err
	}
	if err := service.executions.CopySuccessfulNodesForCompany(
		ctx, companyID, sourceExecutionID, recovery.ID, preservedIDs,
	); err != nil {
		return RecoveryOutcome{}, err
	}
	for _, nodeID := range outOfScopeIDs {
		if err := service.executions.MarkNodeSkipped(
			ctx, companyID, recovery.ID, nodeID,
			string(model.SkipReasonOutOfTriggerScope),
		); err != nil {
			return RecoveryOutcome{}, err
		}
	}
	scheduledNow, err := service.scheduler.Activate(ctx, recovery, nil)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	if scheduledNow > 0 {
		scheduled = scheduledNow
	}
	recovery, err = service.executions.FindByID(ctx, companyID, recovery.ID)
	if err != nil {
		return RecoveryOutcome{}, err
	}
	return RecoveryOutcome{
		SourceExecutionID: sourceExecutionID, RecoveryExecutionID: recovery.ID,
		Status: recovery.Status, PreservedNodeCount: preserved,
		ScheduledNodeCount: scheduled, ResetNodeCount: reset,
		CreatedAt: createdAt, Replayed: replayed,
	}, nil
}

func recoveryReadyCount(
	workflow workflowfeature.Workflow,
	preservedNodeIDs []string,
	excluded map[string]bool,
) int {
	preserved := make(map[string]bool, len(preservedNodeIDs))
	for _, nodeID := range preservedNodeIDs {
		preserved[nodeID] = true
	}
	count := 0
	for _, node := range workflow.Nodes {
		if preserved[node.ID] || excluded[node.ID] {
			continue
		}
		ready := true
		for _, predecessorID := range workflowfeature.Predecessors(workflow, node.ID) {
			if !preserved[predecessorID] {
				ready = false
				break
			}
		}
		if ready {
			count++
		}
	}
	return count
}

func nodeSkipReason(state model.NodeExecution) string {
	if state.Status != model.NodeSkipped || state.Failure == nil {
		return ""
	}
	reason, _ := state.Failure["skipReason"].(string)
	return reason
}
