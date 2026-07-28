package worker

import (
	context "context"
	sha256 "crypto/sha256"
	hex "encoding/hex"
	errors "errors"
	config "miletos-go/internal/config"
	engine "miletos-go/internal/engine"
	enginepersistence "miletos-go/internal/engine/lifecycle/persistence"
	core "miletos-go/internal/engine/nodes/core"
	plugin "miletos-go/internal/engine/plugin"
	runtime "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	transport "miletos-go/internal/infra/kafka"
	kafkav1 "miletos-go/internal/infra/kafka/protocol/kafkav1"
	postgres "miletos-go/internal/infra/postgres"
	messaging "miletos-go/internal/ports/messaging"
	repository "miletos-go/internal/ports/persistence"
	sharedclock "miletos-go/internal/shared/clock"
	os "os"
	sync "sync"
	atomic "sync/atomic"
	testing "testing"
	time "time"
)

func TestPoolBoundsConcurrency(t *testing.T) {
	pool, err := NewPool(context.Background(), 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Shutdown(context.Background())
	var current, maximum int32
	done := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		if err := pool.Submit(func(context.Context) error {
			n := atomic.AddInt32(&current, 1)
			for {
				old := atomic.LoadInt32(&maximum)
				if n <= old || atomic.CompareAndSwapInt32(&maximum, old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&current, -1)
			done <- struct{}{}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	if maximum > 2 {
		t.Fatalf("maximum concurrency=%d", maximum)
	}
}

func TestPoolRejectsUnsupportedConcurrency(t *testing.T) {
	if _, err := NewPool(context.Background(), 1, 2); err == nil {
		t.Fatal("accepted concurrency below minimum")
	}
	if _, err := NewPool(context.Background(), 6, 6); err == nil {
		t.Fatal("accepted concurrency above maximum")
	}
}

func TestPoolShutdownReturnsWhileIdle(t *testing.T) {
	pool, err := NewPool(context.Background(), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown()=%v", err)
	}
	if err := pool.Submit(func(context.Context) error { return nil }); err == nil {
		t.Fatal("Submit() accepted work after shutdown")
	}
}

func TestPoolShutdownCancelsInFlightWork(t *testing.T) {
	pool, err := NewPool(context.Background(), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	finished := make(chan struct{})
	if err := pool.Submit(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown()=%v", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("in-flight task did not observe shutdown cancellation")
	}
}

func TestWorkerPostgreSQLKafkaFinalizationRollbackReplayIntegration(t *testing.T) {
	broker := os.Getenv("MILETOS_KAFKA_TEST_BROKER")
	if broker == "" || os.Getenv("MILETOS_RUNTIME_POSTGRES_PASSWORD") == "" {
		t.Skip("real PostgreSQL and Kafka are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pgConfig, err := config.LoadPostgreSQL()
	if err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.OpenPool(ctx, pgConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, _ := postgres.NewStore(pool)
	codec, _ := kafkav1.NewCodec(1024*1024, 1024*1024)
	limits, _ := runtime.NewRuntimeLimits(100, 100, 1024*1024)
	descriptors, _ := core.CoreDescriptors()
	plugins, _ := plugin.NewRegistry(descriptors)
	registrations, _ := core.DefaultExecutorRegistrations(limits)
	executors, _ := runtime.NewExecutorRegistry(registrations)
	recorder, _ := enginepersistence.NewRecorder(store)
	dependencies, _ := engine.NewEngineDependencies(plugins, executors, limits, sharedclock.System())
	dependencies, _ = dependencies.WithLifecycleRecorder(recorder)
	runSuffix := time.Now().UTC().Format("150405000000000")
	kafkaConfig := config.KafkaConfig{Enabled: true, Brokers: []string{broker}, EngineClientID: "cp08-continuation-engine-" + runSuffix, WorkerClientID: "cp08-continuation-worker-" + runSuffix, CommandTopic: "miletos.workflow.node.commands.v1", EventTopic: "miletos.workflow.node.events.v1", WorkerGroupID: "cp08-continuation-workers-" + runSuffix, EngineGroupID: "cp08-continuation-results-" + runSuffix, MaxMessageBytes: 1024 * 1024}
	coordinator := engine.AsyncCoordinator{Outbox: store, Codec: codec, Topic: kafkaConfig.CommandTopic}
	runner, _ := engine.NewAsyncRunner(dependencies, store, coordinator)
	executionID := execution.WorkflowExecutionID("cp08-continuation-" + runSuffix)
	definition := continuationWorkflow(t, executionID.String())
	request, _ := engine.NewAsyncExecutionRequest(executionID, definition, "cp08-continuation-correlation", nil)
	runResult, err := runner.Run(ctx, request)
	if err != nil || runResult.ScheduledRoots != 1 {
		t.Fatalf("async run roots=%d error=%v", runResult.ScheduledRoots, err)
	}

	producer, _ := transport.NewProducer(kafkaConfig.Brokers, kafkaConfig.EngineClientID, kafkaConfig.MaxMessageBytes)
	defer producer.Close()
	publisher, _ := transport.NewOutboxPublisher(store, producer, "cp08-continuation-publisher", 10, 50*time.Millisecond, time.Minute)
	identity, _ := repository.NewConsumerIdentity(kafkaConfig.WorkerGroupID)
	registryExecutor := RegistryExecutor{Registry: executors, Plugins: plugins, Limits: limits, Store: store}
	var executions atomic.Int32
	counting := ExecutorFunc(func(ctx context.Context, command messaging.NodeCommand, record repository.NodeExecutionRecord) (runtime.NodeResult, error) {
		executions.Add(1)
		result, err := registryExecutor.Execute(ctx, command, record)
		if err != nil {
			t.Logf("registry executor error: %v", err)
		}
		return result, err
	})
	failing := &rollbackFinalizationTransactor{delegate: store, fail: true}
	firstConsumer, _ := transport.NewConsumer(kafkaConfig.Brokers, kafkaConfig.WorkerClientID, kafkaConfig.WorkerGroupID, kafkaConfig.CommandTopic)
	firstPool, _ := NewPool(ctx, 2, 2)
	firstWorker, _ := NewWorker(codec, store, failing, counting, firstConsumer, firstPool, identity, NewCompletionBuilder(codec, kafkaConfig.EventTopic))
	firstErr := make(chan error, 1)
	go func() { firstErr <- firstWorker.Run(ctx) }()
	time.Sleep(750 * time.Millisecond)
	if count, err := publisher.PublishBatch(ctx); err != nil || count != 1 {
		t.Fatalf("publish command=%d/%v", count, err)
	}
	var firstRunErr error
	select {
	case err := <-firstErr:
		firstRunErr = err
		if err == nil {
			t.Fatal("first worker unexpectedly succeeded")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	firstConsumer.Close()
	_ = firstPool.Shutdown(context.Background())
	nodeExecutionID := execution.NodeExecutionID(executionID.String() + "/node/source")
	checkpoint, err := store.GetDurableWorkerResult(ctx, definition.CompanyID(), executionID, nodeExecutionID, 1)
	if err != nil || !checkpoint.IsValid() || executions.Load() != 1 {
		t.Fatalf("checkpoint=%t error=%v executions=%d worker_error=%v", checkpoint.IsValid(), err, executions.Load(), firstRunErr)
	}
	record, _ := store.GetNodeExecution(ctx, definition.CompanyID(), executionID, nodeExecutionID)
	if record.Status() != execution.NodeExecutionStatusRunning {
		t.Fatalf("after rollback status=%s", record.Status())
	}

	secondConsumer, _ := transport.NewConsumer(kafkaConfig.Brokers, kafkaConfig.WorkerClientID+"-replay", kafkaConfig.WorkerGroupID, kafkaConfig.CommandTopic)
	secondPool, _ := NewPool(ctx, 2, 2)
	secondWorker, _ := NewWorker(codec, store, store, counting, secondConsumer, secondPool, identity, NewCompletionBuilder(codec, kafkaConfig.EventTopic))
	secondErr := make(chan error, 1)
	go func() { secondErr <- secondWorker.Run(ctx) }()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		record, err = store.GetNodeExecution(ctx, definition.CompanyID(), executionID, nodeExecutionID)
		if err == nil && record.Status() == execution.NodeExecutionStatusSucceeded {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	secondConsumer.Close()
	_ = secondPool.Shutdown(context.Background())
	if record.Status() != execution.NodeExecutionStatusSucceeded || executions.Load() != 1 {
		t.Fatalf("replay status=%s executions=%d", record.Status(), executions.Load())
	}
	messageID := continuationCommandID(executionID, nodeExecutionID)
	processed, err := store.HasProcessedInboxMessage(ctx, identity, repository.MessageID(messageID))
	if err != nil || !processed {
		t.Fatalf("inbox=%t/%v", processed, err)
	}
	claimed, err := store.ClaimPublishableOutboxMessages(ctx, mustContinuationClaim(t, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	results := 0
	for _, message := range claimed {
		if message.OperationKind() == repository.OutboxOperationNodeResult {
			results++
		}
	}
	if results != 1 {
		t.Fatalf("NODE_RESULT outbox=%d", results)
	}
	_ = secondErr
}

type rollbackFinalizationTransactor struct {
	delegate repository.AsyncPersistenceTransactor
	fail     bool
}

func (wrapper *rollbackFinalizationTransactor) WithinAsyncPersistenceTransaction(ctx context.Context, work repository.AsyncPersistenceWork) error {
	return wrapper.delegate.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		return work(ctx, &rollbackFinalizationTransaction{AsyncPersistenceTransaction: tx, owner: wrapper})
	})
}

type rollbackFinalizationTransaction struct {
	repository.AsyncPersistenceTransaction
	owner *rollbackFinalizationTransactor
}

func (tx *rollbackFinalizationTransaction) FinalizeAsyncNodeExecution(ctx context.Context, finalization repository.AsyncNodeFinalization) error {
	if err := tx.AsyncPersistenceTransaction.FinalizeAsyncNodeExecution(ctx, finalization); err != nil {
		return err
	}
	if tx.owner.fail {
		tx.owner.fail = false
		return errors.New("forced finalization rollback")
	}
	return nil
}

func continuationWorkflow(t *testing.T, suffix string) workflow.WorkflowDefinition {
	t.Helper()
	source, _ := workflow.NewNodeDefinition("source", core.StaticInputPluginType, core.CorePluginVersion, []byte(`{"value":{"cp08":true}}`), nil)
	terminal, _ := workflow.NewNodeDefinition("terminal", core.TerminalPluginType, core.CorePluginVersion, []byte(`{}`), nil)
	edge, _ := workflow.NewEdgeDefinition("source-terminal", source.ID(), core.OutputPortName, terminal.ID(), core.InputPortName)
	definition, err := workflow.NewWorkflowDefinition(workflow.WorkflowID("workflow-"+suffix), workflow.CompanyID("company-"+suffix), "CP08 continuation", 1, []workflow.NodeDefinition{source, terminal}, []workflow.EdgeDefinition{edge}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}
func continuationCommandID(executionID execution.WorkflowExecutionID, nodeExecutionID execution.NodeExecutionID) string {
	digest := sha256.Sum256([]byte("node-command|" + executionID.String() + "|" + nodeExecutionID.String() + "|1"))
	return "node-command-" + hex.EncodeToString(digest[:])
}
func mustContinuationClaim(t *testing.T, at time.Time) repository.OutboxClaimRequest {
	t.Helper()
	request, err := repository.NewOutboxClaimRequest("continuation-inspector", at, 10)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestWorkerReplaysFinalizationFromCheckpointWithoutExecutingPluginAgain(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	store := newContinuationWorkerStore(t, now)
	transactor := &continuationTransactor{store: store, failFinalizationOnce: true}
	codec, _ := kafkav1.NewCodec(1024*1024, 1024*1024)
	command := continuationCommand(t, now)
	transportCommand, _ := kafkav1.ToNodeCommandV1(command, 1024*1024)
	payload, _ := codec.EncodeNodeCommand(transportCommand)
	delivery := transport.Delivery{Topic: "commands", Partition: 0, Offset: 10, Value: payload}
	pool, _ := NewPool(context.Background(), 2, 2)
	defer pool.Shutdown(context.Background())
	var executions atomic.Int32
	executor := ExecutorFunc(func(context.Context, messaging.NodeCommand, repository.NodeExecutionRecord) (runtime.NodeResult, error) {
		executions.Add(1)
		value, _ := runtime.NewInlinePayload(runtime.ContentTypeApplicationJSON, []byte(`{"ok":true}`), nil, 1024)
		changes, _ := runtime.NewContextChanges(nil, nil)
		return runtime.NewNodeSuccessResult(map[string][]runtime.Payload{"output": {value}}, changes)
	})
	consumer := &continuationConsumer{}
	identity, _ := repository.NewConsumerIdentity("worker-continuation-test")
	worker, err := NewWorker(codec, store, transactor, executor, consumer, pool, identity, NewCompletionBuilder(codec, "events"))
	if err != nil {
		t.Fatal(err)
	}
	firstErr := worker.processDelivery(context.Background(), delivery)
	if firstErr == nil {
		t.Fatal("first delivery unexpectedly finalized")
	}
	if executions.Load() != 1 || !store.hasCheckpoint() || store.status() != execution.NodeExecutionStatusRunning || store.processed {
		t.Fatalf("after rollback error=%v executions=%d checkpoint=%t status=%s processed=%t", firstErr, executions.Load(), store.hasCheckpoint(), store.status(), store.processed)
	}
	if err := worker.processDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("replay error=%v", err)
	}
	if executions.Load() != 1 || store.status() != execution.NodeExecutionStatusSucceeded || !store.processed || store.outboxCount != 1 {
		t.Fatalf("after replay executions=%d status=%s processed=%t outbox=%d", executions.Load(), store.status(), store.processed, store.outboxCount)
	}
}

type continuationWorkerStore struct {
	mu               sync.Mutex
	record           repository.NodeExecutionRecord
	checkpoint       repository.DurableWorkerResult
	checkpointExists bool
	processed        bool
	outboxCount      int
}

func newContinuationWorkerStore(t *testing.T, now time.Time) *continuationWorkerStore {
	t.Helper()
	record, err := repository.NewNodeExecutionRecord(repository.NodeExecutionRecordParams{ID: "execution/node/node", WorkflowExecutionID: "execution", CompanyID: "company", NodeID: "node", PluginType: "core.static-input", PluginVersion: "v1", Status: execution.NodeExecutionStatusQueued, Attempt: 1, CreatedAt: now, ReadyAt: now, QueuedAt: now, UpdatedAt: now, LockVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	return &continuationWorkerStore{record: record}
}

func (store *continuationWorkerStore) WithAsyncNodeExecutionLock(ctx context.Context, _ repository.AsyncNodeExecutionLock, work repository.AsyncNodeExecutionLockWork) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	return work(ctx)
}
func (store *continuationWorkerStore) GetNodeExecution(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, execution.NodeExecutionID) (repository.NodeExecutionRecord, error) {
	return store.record, nil
}
func (store *continuationWorkerStore) CreateDurableWorkerResult(_ context.Context, result repository.DurableWorkerResult) error {
	if store.checkpointExists {
		return repository.NewConflictError("create", "checkpoint", errors.New("duplicate"))
	}
	store.checkpoint = result
	store.checkpointExists = true
	return nil
}
func (store *continuationWorkerStore) GetDurableWorkerResult(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, execution.NodeExecutionID, int16) (repository.DurableWorkerResult, error) {
	if !store.checkpointExists {
		return repository.DurableWorkerResult{}, repository.NewNotFoundError("get", "checkpoint", errors.New("missing"))
	}
	return store.checkpoint, nil
}
func (store *continuationWorkerStore) hasCheckpoint() bool { return store.checkpointExists }
func (store *continuationWorkerStore) status() execution.NodeExecutionStatus {
	return store.record.Status()
}

type continuationTransactor struct {
	store                *continuationWorkerStore
	failFinalizationOnce bool
}

func (transactor *continuationTransactor) WithinAsyncPersistenceTransaction(ctx context.Context, work repository.AsyncPersistenceWork) error {
	return work(ctx, &continuationTransaction{AsyncPersistenceTransaction: nil, owner: transactor})
}

type continuationTransaction struct {
	repository.AsyncPersistenceTransaction
	owner *continuationTransactor
}

func (tx *continuationTransaction) HasProcessedInboxMessage(context.Context, repository.ConsumerIdentity, repository.MessageID) (bool, error) {
	return tx.owner.store.processed, nil
}
func (tx *continuationTransaction) ClaimAsyncNodeExecution(_ context.Context, claim repository.AsyncNodeClaim) (int64, error) {
	now := claim.StartedAt
	record, err := repository.NewNodeExecutionRecord(repository.NodeExecutionRecordParams{ID: "execution/node/node", WorkflowExecutionID: "execution", CompanyID: "company", NodeID: "node", PluginType: "core.static-input", PluginVersion: "v1", Status: execution.NodeExecutionStatusRunning, Attempt: 1, CreatedAt: tx.owner.store.record.CreatedAt(), ReadyAt: mustReady(tx.owner.store.record), QueuedAt: mustQueued(tx.owner.store.record), StartedAt: now, UpdatedAt: now, LockVersion: 1})
	if err != nil {
		return 0, err
	}
	tx.owner.store.record = record
	return 1, nil
}
func (tx *continuationTransaction) FinalizeAsyncNodeExecution(_ context.Context, finalization repository.AsyncNodeFinalization) error {
	if tx.owner.failFinalizationOnce {
		tx.owner.failFinalizationOnce = false
		return errors.New("forced finalization rollback")
	}
	r := tx.owner.store.record
	record, err := repository.NewNodeExecutionRecord(repository.NodeExecutionRecordParams{ID: r.ID(), WorkflowExecutionID: r.WorkflowExecutionID(), CompanyID: r.CompanyID(), NodeID: r.NodeID(), PluginType: r.PluginType(), PluginVersion: r.PluginVersion(), Status: finalization.Completion.Status, Attempt: r.Attempt(), CreatedAt: r.CreatedAt(), ReadyAt: mustReady(r), QueuedAt: mustQueued(r), StartedAt: mustStarted(r), FinishedAt: finalization.Completion.FinishedAt, UpdatedAt: finalization.Completion.FinishedAt, OutputSummary: []byte(`{}`), FailureSummary: []byte(`{}`), LockVersion: r.LockVersion() + 1})
	if err != nil {
		return err
	}
	tx.owner.store.record = record
	tx.owner.store.processed = true
	tx.owner.store.outboxCount++
	return nil
}

func mustStarted(record repository.NodeExecutionRecord) time.Time {
	value, _ := record.StartedAt()
	return value
}

func mustReady(record repository.NodeExecutionRecord) time.Time {
	value, _ := record.ReadyAt()
	return value
}

func mustQueued(record repository.NodeExecutionRecord) time.Time {
	value, _ := record.QueuedAt()
	return value
}

type continuationConsumer struct{}

func (*continuationConsumer) Run(context.Context, transport.DeliveryHandler) error { return nil }
func (*continuationConsumer) Close()                                               {}

func continuationCommand(t *testing.T, now time.Time) messaging.NodeCommand {
	t.Helper()
	metadata, _ := messaging.NewMessageMetadata(messaging.MessageMetadataParams{MessageID: "command-message", CreatedAt: now, CompanyID: "company", WorkflowID: "workflow", WorkflowExecutionID: "execution", NodeID: "node", NodeExecutionID: "execution/node/node", Attempt: 1, CorrelationID: "correlation", CausationID: "causation"})
	node, _ := workflow.NewNodeDefinition("node", "core.static-input", "v1", []byte(`{"value":{"ok":true}}`), nil)
	input, _ := runtime.NewNodeInput(nil)
	command, err := messaging.NewNodeCommand(metadata, node, input, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return command
}
