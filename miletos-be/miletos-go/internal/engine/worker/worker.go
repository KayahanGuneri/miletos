package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/messaging"
	repository "miletos-go/internal/ports/persistence"
)

func NewCompletionBuilder(codec messaging.Codec, destination string) CompletionBuilder {
	return func(
		command messaging.NodeCommand,
		result runtime.NodeResult,
		technicalDetail string,
		attemptStartedAt time.Time,
		finished time.Time,
	) (repository.DurableWorkerResult, repository.OutboxMessage, error) {
		if attemptStartedAt.IsZero() {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{},
				fmt.Errorf("attempt start time must not be zero")
		}
		if finished.IsZero() {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{},
				fmt.Errorf("attempt finish time must not be zero")
		}
		if finished.Before(attemptStartedAt) {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{},
				fmt.Errorf("attempt finish time must not be before attempt start time")
		}
		commandMetadata := command.Metadata()
		digest := sha256.Sum256([]byte(commandMetadata.MessageID()))
		messageID := "worker-result-" + hex.EncodeToString(digest[:])
		resultMetadata, err := messaging.NewMessageMetadata(messaging.MessageMetadataParams{MessageID: messageID, CreatedAt: finished, CompanyID: commandMetadata.CompanyID(),
			WorkflowID: commandMetadata.WorkflowID(), WorkflowExecutionID: commandMetadata.WorkflowExecutionID(), NodeID: commandMetadata.NodeID(),
			NodeExecutionID: commandMetadata.NodeExecutionID(), Attempt: commandMetadata.Attempt(), CorrelationID: commandMetadata.CorrelationID(), CausationID: commandMetadata.MessageID()})
		if err != nil {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{}, err
		}
		event, err := messaging.NewNodeResultEvent(
			resultMetadata, result, attemptStartedAt, finished,
		)
		if err != nil {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{}, err
		}
		encoded, err := codec.EncodeResult(event)
		if err != nil {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{}, err
		}
		status := repository.DurableWorkerResultSucceeded
		if result.IsFailure() {
			status = repository.DurableWorkerResultFailed
		}
		durable, err := repository.NewDurableWorkerResult(repository.DurableWorkerResultParams{CompanyID: command.Metadata().CompanyID(),
			WorkflowExecutionID: command.Metadata().WorkflowExecutionID(), NodeExecutionID: command.Metadata().NodeExecutionID(), NodeID: command.Metadata().NodeID(),
			Attempt: command.Metadata().Attempt(), Status: status, Result: result, TechnicalDetail: technicalDetail,
			StartedAt: attemptStartedAt, FinishedAt: finished, CreatedAt: finished})
		if err != nil {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{}, err
		}
		outbox, err := repository.NewOutboxMessage(repository.OutboxMessageParams{MessageID: repository.MessageID(messageID), CompanyID: command.Metadata().CompanyID(),
			WorkflowExecutionID: command.Metadata().WorkflowExecutionID(), NodeExecutionID: command.Metadata().NodeExecutionID(), NodeID: command.Metadata().NodeID(),
			Attempt: command.Metadata().Attempt(), OperationKind: repository.OutboxOperationNodeResult, MessageType: encoded.Descriptor.Type, MessageVersion: encoded.Descriptor.Version,
			Destination: destination, MessageKey: command.Metadata().WorkflowExecutionID().String(), EncodedPayload: encoded.Payload, PublicationState: repository.OutboxPublicationPending,
			CreatedAt: finished, AvailableAt: finished})
		if err != nil {
			return repository.DurableWorkerResult{},
				repository.OutboxMessage{}, err
		}
		return durable, outbox, nil
	}
}

const (
	MinConcurrency = 2
	MaxConcurrency = 5
)

type Task func(context.Context) error
type Pool struct {
	ctx     context.Context
	cancel  context.CancelFunc
	tasks   chan Task
	workers int
	wg      sync.WaitGroup
	mu      sync.Mutex
	closed  bool
}

func NewPool(parent context.Context, concurrency, queueSize int) (*Pool, error) {
	if parent == nil {
		return nil, fmt.Errorf("worker context must not be nil")
	}
	if concurrency < MinConcurrency || concurrency > MaxConcurrency {
		return nil, fmt.Errorf("worker concurrency must be between %d and %d", MinConcurrency, MaxConcurrency)
	}
	if queueSize < concurrency {
		return nil, fmt.Errorf("worker queue size must be at least concurrency")
	}
	ctx, cancel := context.WithCancel(parent)
	pool := &Pool{ctx: ctx, cancel: cancel, tasks: make(chan Task, queueSize), workers: concurrency}
	for i := 0; i < concurrency; i++ {
		pool.wg.Add(1)
		go pool.run()
	}
	return pool, nil
}
func (pool *Pool) Submit(task Task) error {
	if pool == nil || task == nil {
		return fmt.Errorf("worker pool and task must be valid")
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.closed {
		return fmt.Errorf("worker pool is closed")
	}
	select {
	case pool.tasks <- task:
		return nil
	case <-pool.ctx.Done():
		return pool.ctx.Err()
	}
}
func (pool *Pool) run() {
	defer pool.wg.Done()
	for {
		select {
		case <-pool.ctx.Done():
			return
		case task := <-pool.tasks:
			if task != nil {
				_ = task(pool.ctx)
			}
		}
	}
}
func (pool *Pool) Shutdown(ctx context.Context) error {
	if pool == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("shutdown context must not be nil")
	}
	pool.mu.Lock()
	pool.closed = true
	close(pool.tasks)
	pool.mu.Unlock()
	pool.cancel()
	done := make(chan struct{})
	go func() { pool.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type Executor interface {
	Execute(
		context.Context,
		messaging.NodeCommand,
		repository.NodeExecutionRecord,
	) (AttemptExecutionOutcome, error)
}
type ExecutorFunc func(
	context.Context,
	messaging.NodeCommand,
	repository.NodeExecutionRecord,
) (runtime.NodeResult, error)

func (function ExecutorFunc) Execute(
	ctx context.Context,
	command messaging.NodeCommand,
	record repository.NodeExecutionRecord,
) (AttemptExecutionOutcome, error) {
	if function == nil {
		return AttemptExecutionOutcome{},
			fmt.Errorf("worker executor must not be nil")
	}
	result, err := function(ctx, command, record)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	return NewAttemptExecutionOutcome(result, "")
}

type CompletionBuilder func(
	messaging.NodeCommand,
	runtime.NodeResult,
	string,
	time.Time,
	time.Time,
) (repository.DurableWorkerResult, repository.OutboxMessage, error)
type WorkerStore interface {
	GetNodeExecution(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, execution.NodeExecutionID) (repository.NodeExecutionRecord, error)

	repository.DurableWorkerResultStore
	repository.AsyncNodeExecutionLocker
}
type Worker struct {
	codec       messaging.Codec
	store       WorkerStore
	transactor  repository.AsyncPersistenceTransactor
	executor    Executor
	consumer    messaging.ConsumerRunner
	pool        *Pool
	consumerID  repository.ConsumerIdentity
	messageSize int
	build       CompletionBuilder
}

func NewWorker(codec messaging.Codec, store WorkerStore, transactor repository.AsyncPersistenceTransactor, executor Executor, consumer messaging.ConsumerRunner, pool *Pool,
	consumerID repository.ConsumerIdentity, build CompletionBuilder) (*Worker, error) {
	if store == nil || transactor == nil || executor == nil || consumer == nil || pool == nil || consumerID.String() == "" || build == nil {
		return nil, fmt.Errorf("invalid worker dependencies")
	}
	return &Worker{codec: codec, store: store, transactor: transactor, executor: executor, consumer: consumer, pool: pool, consumerID: consumerID, build: build}, nil
}
func (worker *Worker) processDelivery(ctx context.Context, delivery messaging.Delivery) error {
	if ctx == nil {
		return fmt.Errorf("worker context must not be nil")
	}
	command, err := worker.codec.DecodeCommand(delivery.Value)
	if err != nil {
		return fmt.Errorf("decode worker command: %w", err)
	}
	metadata := command.Metadata()
	lock := repository.AsyncNodeExecutionLock{CompanyID: metadata.CompanyID(), WorkflowExecutionID: metadata.WorkflowExecutionID(), NodeExecutionID: metadata.NodeExecutionID(),
		Attempt: metadata.Attempt()}
	return worker.store.WithAsyncNodeExecutionLock(ctx, lock, func(ctx context.Context) error {
		return worker.processLockedDelivery(ctx, delivery, command)
	})
}
func (worker *Worker) processLockedDelivery(ctx context.Context, delivery messaging.Delivery, command messaging.NodeCommand) error {
	metadata := command.Metadata()
	processed, err := worker.hasProcessedCommand(ctx, metadata)
	if err != nil || processed {
		return err
	}
	nodeRecord, err := worker.store.GetNodeExecution(ctx, metadata.CompanyID(), metadata.WorkflowExecutionID(), metadata.NodeExecutionID())
	if err != nil {
		return err
	}
	if nodeRecord.CompanyID() != metadata.CompanyID() || nodeRecord.WorkflowExecutionID() != metadata.WorkflowExecutionID() ||
		nodeRecord.ID() != metadata.NodeExecutionID() || nodeRecord.NodeID() != metadata.NodeID() || nodeRecord.PluginType() != command.NodeDefinition().PluginType() ||
		nodeRecord.PluginVersion() != command.NodeDefinition().PluginVersion() {
		return fmt.Errorf("worker command does not match persisted node execution")
	}
	ignore, claimRequired, err := classifyAsyncCommandAttempt(
		nodeRecord, metadata.Attempt(),
	)
	if err != nil {
		return err
	}
	if ignore {
		ignoredAt := time.Now().UTC()
		inbox, err := worker.commandInbox(
			delivery, command, ignoredAt,
			repository.InboxProcessingIgnored,
		)
		if err != nil {
			return err
		}
		return worker.transactor.WithinAsyncPersistenceTransaction(
			ctx,
			func(
				ctx context.Context,
				tx repository.AsyncPersistenceTransaction,
			) error {
				return tx.RecordInboxMessage(ctx, inbox)
			},
		)
	}
	checkpoint, checkpointExists, err := worker.loadCheckpoint(ctx, metadata)
	if err != nil {
		return err
	}
	var attemptStartedAt time.Time
	if claimRequired {
		if checkpointExists {
			return fmt.Errorf(
				"claimable node unexpectedly has a durable worker result",
			)
		}
		claimedVersion := int64(0)
		claimAt := time.Now().UTC()
		err = worker.transactor.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
			claimedVersion, err = tx.ClaimAsyncNodeExecution(
				ctx,
				repository.AsyncNodeClaim{
					CompanyID:           metadata.CompanyID(),
					WorkflowExecutionID: metadata.WorkflowExecutionID(),
					NodeExecutionID:     metadata.NodeExecutionID(),
					ExpectedNodeStatus:  nodeRecord.Status(),
					ExpectedNodeAttempt: nodeRecord.Attempt(),
					CommandAttempt:      metadata.Attempt(),
					ExpectedLockVersion: nodeRecord.LockVersion(),
					StartedAt:           claimAt,
				},
			)
			return err
		})
		if err != nil {
			return err
		}
		nodeRecord, err = worker.store.GetNodeExecution(ctx, metadata.CompanyID(), metadata.WorkflowExecutionID(), metadata.NodeExecutionID())
		if err != nil ||
			nodeRecord.Status() != execution.NodeExecutionStatusRunning ||
			nodeRecord.Attempt() != metadata.Attempt() ||
			nodeRecord.LockVersion() != claimedVersion {
			return fmt.Errorf("claimed node execution was not durably reconstructed: %w", err)
		}
		attemptStartedAt = claimAt
	} else if checkpointExists {
		attemptStartedAt = checkpoint.StartedAt()
	} else {
		if nodeRecord.Status() != execution.NodeExecutionStatusRunning {
			return fmt.Errorf(
				"checkpoint-free recovery requires a RUNNING node execution",
			)
		}
		aggregateStartedAt, exists := nodeRecord.StartedAt()
		if !exists {
			return fmt.Errorf(
				"RUNNING node execution must contain aggregate start time",
			)
		}
		attemptStartedAt = nodeRecord.UpdatedAt()
		if attemptStartedAt.Before(aggregateStartedAt) {
			return fmt.Errorf(
				"current attempt start time must not be before aggregate node start time",
			)
		}
	}
	var (
		publicationResult repository.DurableWorkerResult
		resultOutbox      repository.OutboxMessage
	)
	if !checkpointExists {
		outcome, executionErr := worker.executor.Execute(
			ctx, command, nodeRecord,
		)
		if executionErr != nil {
			return executionErr
		}
		finished := time.Now().UTC()
		technicalDetail, _ := outcome.TechnicalDetail()
		publicationResult, resultOutbox, err = worker.build(
			command, outcome.Result(), technicalDetail,
			attemptStartedAt, finished,
		)
		if err != nil {
			return err
		}
		if err := worker.store.CreateDurableWorkerResult(
			ctx, publicationResult,
		); err != nil {
			return fmt.Errorf("checkpoint durable worker result: %w", err)
		}
		checkpoint = publicationResult
	} else {
		result, exists := checkpoint.Result()
		if !exists {
			return fmt.Errorf("durable worker result cannot reconstruct runtime result")
		}
		technicalDetail, _ := checkpoint.TechnicalDetail()
		publicationResult, resultOutbox, err = worker.build(
			command, result, technicalDetail,
			attemptStartedAt, checkpoint.FinishedAt(),
		)
		if err != nil {
			return err
		}
	}
	inbox, err := worker.commandInbox(
		delivery, command, checkpoint.FinishedAt(),
		repository.InboxProcessingApplied,
	)
	if err != nil {
		return err
	}
	return worker.transactor.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		return tx.PublishAsyncWorkerResult(
			ctx,
			repository.AsyncWorkerResultPublication{
				CompanyID:               metadata.CompanyID(),
				WorkflowExecutionID:     metadata.WorkflowExecutionID(),
				NodeExecutionID:         metadata.NodeExecutionID(),
				ExpectedNodeLockVersion: nodeRecord.LockVersion(),
				DurableWorkerResult:     publicationResult,
				NodeResultOutboxMessage: resultOutbox,
				CommandInboxMessage:     inbox,
			},
		)
	})
}
func (worker *Worker) hasProcessedCommand(ctx context.Context, metadata messaging.MessageMetadata) (bool, error) {
	processed := false
	err := worker.transactor.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		var err error
		processed, err = tx.HasProcessedInboxMessage(ctx, worker.consumerID, repository.MessageID(metadata.MessageID()))
		return err
	})
	return processed, err
}
func (worker *Worker) loadCheckpoint(ctx context.Context, metadata messaging.MessageMetadata) (repository.DurableWorkerResult, bool, error) {
	result, err := worker.store.GetDurableWorkerResult(ctx, metadata.CompanyID(), metadata.WorkflowExecutionID(), metadata.NodeExecutionID(), metadata.Attempt())
	if repository.IsNotFound(err) {
		return repository.DurableWorkerResult{}, false, nil
	}
	return result, err == nil, err
}
func classifyAsyncCommandAttempt(
	record repository.NodeExecutionRecord,
	commandAttempt int16,
) (bool, bool, error) {
	legalAttempt := record.Attempt()
	claimRequired := false
	switch record.Status() {
	case execution.NodeExecutionStatusQueued:
		claimRequired = true
	case execution.NodeExecutionStatusRunning:
	case execution.NodeExecutionStatusRetryPending:
		currentAttempt, err := execution.NewAttemptNumber(record.Attempt())
		if err != nil {
			return false, false, err
		}
		nextAttempt, err := currentAttempt.Next()
		if err != nil {
			return false, false,
				fmt.Errorf("retry command attempt cannot advance: %w", err)
		}
		legalAttempt = nextAttempt.Int16()
		claimRequired = true
	default:
		if record.Status().IsTerminal() {
			if commandAttempt <= legalAttempt {
				return true, false, nil
			}
			return false, false,
				fmt.Errorf("worker command attempt is ahead of terminal node execution")
		}
		return false, false, fmt.Errorf(
			"node execution state %s cannot process a command",
			record.Status(),
		)
	}
	if commandAttempt < legalAttempt {
		return true, false, nil
	}
	if commandAttempt > legalAttempt {
		return false, false,
			fmt.Errorf("worker command attempt is ahead of persisted node execution")
	}
	return false, claimRequired, nil
}

func (worker *Worker) commandInbox(
	delivery messaging.Delivery,
	command messaging.NodeCommand,
	at time.Time,
	processingResult repository.InboxProcessingResult,
) (repository.InboxMessage, error) {
	position, err := repository.NewInboxSourcePosition(delivery.Topic, delivery.Partition, delivery.Offset)
	if err != nil {
		return repository.InboxMessage{}, err
	}
	metadata := command.Metadata()
	return repository.NewInboxMessage(repository.InboxMessageParams{ConsumerIdentity: worker.consumerID, MessageID: repository.MessageID(metadata.MessageID()),
		CompanyID: metadata.CompanyID(), WorkflowExecutionID: metadata.WorkflowExecutionID(), NodeExecutionID: metadata.NodeExecutionID(),
		MessageType: worker.codec.CommandDescriptor().Type, MessageVersion: worker.codec.CommandDescriptor().Version, ProcessingResult: processingResult,
		SourcePosition: &position, ReceivedAt: at, ProcessedAt: at})
}
func (worker *Worker) HandleDelivery(ctx context.Context, delivery messaging.Delivery) error {
	done := make(chan error, 1)
	if err := worker.pool.Submit(func(taskCtx context.Context) error {
		err := worker.processDelivery(taskCtx, delivery)
		done <- err
		return nil
	}); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (worker *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("worker context must not be nil")
	}
	return worker.consumer.Run(ctx, worker.HandleDelivery)
}
func (worker *Worker) Shutdown(ctx context.Context) error {
	if worker == nil {
		return nil
	}
	if err := worker.pool.Shutdown(ctx); err != nil {
		return err
	}
	worker.consumer.Close()
	return nil
}
