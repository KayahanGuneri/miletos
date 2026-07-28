package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/messaging"
	repository "miletos-go/internal/ports/persistence"
)

type AsyncCoordinator struct {
	Outbox repository.OutboxStore
	Codec  messaging.Codec
	Topic  string
}

func (coordinator AsyncCoordinator) ScheduleNodeCommand(ctx context.Context, command messaging.NodeCommand) error {
	return coordinator.ScheduleNodeCommandWithStore(ctx, coordinator.Outbox, command)
}

// ScheduleNodeCommandWithStore allows the async scheduler to bind command
// creation to the caller-owned PostgreSQL transaction. Network publication is
// deliberately left to the outbox publisher.
func (coordinator AsyncCoordinator) ScheduleNodeCommandWithStore(
	ctx context.Context, store repository.OutboxStore, command messaging.NodeCommand,
) error {
	if ctx == nil || store == nil || !command.IsValid() || coordinator.Topic == "" {
		return fmt.Errorf("invalid async coordinator dependencies")
	}
	message, err := coordinator.BuildNodeCommandOutbox(command)
	if err != nil {
		return err
	}
	return store.CreateOutboxMessage(ctx, message)
}
func (coordinator AsyncCoordinator) BuildNodeCommandOutbox(command messaging.NodeCommand) (repository.OutboxMessage, error) {
	return coordinator.BuildNodeCommandOutboxAvailableAt(
		command, command.Metadata().CreatedAt(),
	)
}
func (coordinator AsyncCoordinator) BuildNodeCommandOutboxAvailableAt(
	command messaging.NodeCommand,
	availableAt time.Time,
) (repository.OutboxMessage, error) {
	if !command.IsValid() || coordinator.Topic == "" {
		return repository.OutboxMessage{}, fmt.Errorf("invalid async coordinator dependencies")
	}
	if availableAt.IsZero() ||
		availableAt.Before(command.Metadata().CreatedAt()) {
		return repository.OutboxMessage{},
			fmt.Errorf("node command availability must not be before creation")
	}
	encoded, err := coordinator.Codec.EncodeCommand(command)
	if err != nil {
		return repository.OutboxMessage{}, err
	}
	metadata := command.Metadata()
	return repository.NewOutboxMessage(repository.OutboxMessageParams{MessageID: repository.MessageID(metadata.MessageID()), CompanyID: metadata.CompanyID(),
		WorkflowExecutionID: metadata.WorkflowExecutionID(), NodeExecutionID: metadata.NodeExecutionID(), NodeID: metadata.NodeID(), Attempt: metadata.Attempt(),
		OperationKind: repository.OutboxOperationNodeCommand, MessageType: encoded.Descriptor.Type, MessageVersion: encoded.Descriptor.Version, Destination: coordinator.Topic,
		MessageKey: metadata.WorkflowExecutionID().String(), EncodedPayload: encoded.Payload, PublicationState: repository.OutboxPublicationPending, CreatedAt: metadata.CreatedAt(),
		AvailableAt: availableAt})
}
func (coordinator AsyncCoordinator) ScheduleNodeCommandInTransaction(
	ctx context.Context, transaction repository.AsyncPersistenceTransaction, command messaging.NodeCommand,
	expectedLockVersion int64, queuedAt time.Time) error {
	if ctx == nil || transaction == nil || queuedAt.IsZero() || expectedLockVersion < 0 {
		return fmt.Errorf("invalid transactional node scheduling dependencies")
	}
	message, err := coordinator.BuildNodeCommandOutbox(command)
	if err != nil {
		return err
	}
	metadata := command.Metadata()
	return transaction.ScheduleAsyncNodeExecution(ctx, repository.AsyncNodeSchedule{
		CompanyID: metadata.CompanyID(), WorkflowExecutionID: metadata.WorkflowExecutionID(), NodeExecutionID: metadata.NodeExecutionID(),
		NodeID: metadata.NodeID(), Attempt: metadata.Attempt(), ExpectedLockVersion: expectedLockVersion,
		QueuedAt: queuedAt, OutboxMessage: message})
}

type AsyncRunResult struct {
	WorkflowExecution execution.WorkflowExecution
	ValidationReport  PreflightValidationReport
	ScheduledRoots    int
}
type AsyncRunner struct {
	dependencies EngineDependencies
	transactor   repository.AsyncPersistenceTransactor
	coordinator  AsyncCoordinator
}

func NewAsyncRunner(dependencies EngineDependencies,
	transactor repository.AsyncPersistenceTransactor, coordinator AsyncCoordinator) (AsyncRunner, error) {
	if !dependencies.IsValid() || transactor == nil || coordinator.Topic == "" {
		return AsyncRunner{}, newValidationError("asyncRunner", "dependencies must be valid")
	}
	return AsyncRunner{dependencies: dependencies, transactor: transactor, coordinator: coordinator}, nil
}
func (runner AsyncRunner) Run(ctx context.Context, request ExecutionRequest) (AsyncRunResult, error) {
	if ctx == nil || !request.IsValid() {
		return AsyncRunResult{}, newValidationError("request", "must be valid")
	}
	if request.Mode() != execution.ExecutionModeAsync {
		return AsyncRunResult{}, newValidationError("request.mode", "async runner accepts only ASYNC requests")
	}
	preparation, err := prepareExecution(ctx, request, runner.dependencies)
	if err != nil {
		return AsyncRunResult{}, fmt.Errorf("prepare asynchronous workflow execution: %w", err)
	}
	result := AsyncRunResult{WorkflowExecution: preparation.WorkflowExecution(), ValidationReport: preparation.ValidationReport()}
	if preparation.IsRejected() {
		return result, nil
	}
	if !preparation.IsPrepared() || preparation.prepared == nil {
		return AsyncRunResult{}, fmt.Errorf("asynchronous workflow execution was not prepared")
	}
	prepared := preparation.prepared
	roots := prepared.plan.graph.Roots()
	if len(roots) == 0 {
		return AsyncRunResult{}, fmt.Errorf("validated asynchronous workflow has no runnable roots")
	}
	queuedAt, err := executionTime(runner.dependencies.Clock(), "asyncRoots.queuedAt")
	if err != nil {
		return AsyncRunResult{}, err
	}
	err = runner.transactor.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		variables := request.InitialVariables()
		keys := make([]string, 0, len(variables))
		for key := range variables {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			variable, err := repository.NewAsyncContextVariable(repository.AsyncContextVariableParams{
				CompanyID: request.Definition().CompanyID(), WorkflowExecutionID: request.WorkflowExecutionID(), Key: key,
				Value: variables[key], CreatedAt: queuedAt, UpdatedAt: queuedAt,
				Version: 1})
			if err != nil {
				return err
			}
			write, err := repository.NewAsyncContextWrite(variable, 0)
			if err != nil {
				return err
			}
			if err := tx.CompareAndSwapAsyncContextVariable(ctx, write); err != nil {
				return err
			}
		}
		for _, nodeID := range roots {
			command, err := buildInitialNodeCommand(request, prepared, nodeID, queuedAt)
			if err != nil {
				return err
			}
			if err := runner.coordinator.ScheduleNodeCommandInTransaction(ctx, tx, command, 0, queuedAt); err != nil {
				return fmt.Errorf("schedule root node %s: %w", nodeID, err)
			}
		}
		return nil
	})
	if err != nil {
		return AsyncRunResult{}, err
	}
	result.ScheduledRoots = len(roots)
	return result, nil
}
func buildInitialNodeCommand(request ExecutionRequest,
	prepared *preparedExecution, nodeID workflow.NodeID, createdAt time.Time,
) (messaging.NodeCommand, error) {
	if prepared == nil {
		return messaging.NodeCommand{}, fmt.Errorf("prepared execution must not be nil")
	}
	node, exists := prepared.plan.graph.Node(nodeID)
	if !exists {
		return messaging.NodeCommand{}, fmt.Errorf("root node %s is unavailable", nodeID)
	}
	nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]
	if !exists || nodeExecution == nil {
		return messaging.NodeCommand{}, fmt.Errorf("root node execution %s is unavailable", nodeID)
	}
	input, err := runtime.NewNodeInput(nil)
	if err != nil {
		return messaging.NodeCommand{}, err
	}
	metadata, err := messaging.NewMessageMetadata(messaging.MessageMetadataParams{MessageID: asyncNodeCommandMessageID(request.WorkflowExecutionID(), nodeExecution.ID(), 1),
		CreatedAt: createdAt, CompanyID: request.Definition().CompanyID(), WorkflowID: request.Definition().ID(),
		WorkflowExecutionID: request.WorkflowExecutionID(), NodeID: nodeID, NodeExecutionID: nodeExecution.ID(),
		Attempt: 1, CorrelationID: request.CorrelationID(), CausationID: request.WorkflowExecutionID().String(),
	})
	if err != nil {
		return messaging.NodeCommand{}, err
	}
	return messaging.NewNodeCommand(metadata, node, input, request.InitialVariables(), nil)
}
func asyncNodeCommandMessageID(workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID, attempt int16) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("node-command|%s|%s|%d", workflowExecutionID, nodeExecutionID, attempt)))
	return "node-command-" + hex.EncodeToString(digest[:])
}
