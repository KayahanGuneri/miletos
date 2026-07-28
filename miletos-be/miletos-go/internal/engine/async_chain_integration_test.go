package engine_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"testing"
	"time"

	"miletos-go/internal/config"
	"miletos-go/internal/engine"
	enginepersistence "miletos-go/internal/engine/lifecycle/persistence"
	"miletos-go/internal/engine/nodes/core"
	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/engine/worker"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	transport "miletos-go/internal/infra/kafka"
	"miletos-go/internal/infra/kafka/protocol/kafkav1"
	"miletos-go/internal/infra/postgres"
	"miletos-go/internal/ports/messaging"
	"miletos-go/internal/ports/persistence"
	sharedclock "miletos-go/internal/shared/clock"
)

func TestAsyncEngineWorkerChainPostgreSQLKafkaIntegration(t *testing.T) {
	runAsyncEngineWorkerGraph(t, chainDefinition, 1, execution.WorkflowExecutionStatusSucceeded, map[workflow.NodeID]execution.NodeExecutionStatus{"source": execution.NodeExecutionStatusSucceeded, "pass": execution.NodeExecutionStatusSucceeded, "terminal": execution.NodeExecutionStatusSucceeded}, true, "", false)
}

func TestAsyncEngineWorkerMultipleRootsFanOutFanInIntegration(t *testing.T) {
	runAsyncEngineWorkerGraph(t, fanGraphDefinition, 2, execution.WorkflowExecutionStatusFailed, map[workflow.NodeID]execution.NodeExecutionStatus{"source-a": execution.NodeExecutionStatusSucceeded, "source-b": execution.NodeExecutionStatusSucceeded, "pass-a": execution.NodeExecutionStatusSucceeded, "terminal": execution.NodeExecutionStatusFailed}, false, "", false)
}

func TestAsyncEngineWorkerFailureSkipsDescendantsPostgreSQLKafkaIntegration(t *testing.T) {
	runAsyncEngineWorkerGraph(t, chainDefinition, 1, execution.WorkflowExecutionStatusFailed, map[workflow.NodeID]execution.NodeExecutionStatus{"source": execution.NodeExecutionStatusSucceeded, "pass": execution.NodeExecutionStatusFailed, "terminal": execution.NodeExecutionStatusSkipped}, false, "pass", false)
}

func TestAsyncEngineResultConsumerInterruptionResumePostgreSQLKafkaIntegration(t *testing.T) {
	runAsyncEngineWorkerGraph(t, chainDefinition, 1, execution.WorkflowExecutionStatusSucceeded, map[workflow.NodeID]execution.NodeExecutionStatus{"source": execution.NodeExecutionStatusSucceeded, "pass": execution.NodeExecutionStatusSucceeded, "terminal": execution.NodeExecutionStatusSucceeded}, false, "", true)
}

func runAsyncEngineWorkerGraph(t *testing.T, buildDefinition func(*testing.T, string) workflow.WorkflowDefinition, expectedRoots int, expectedWorkflow execution.WorkflowExecutionStatus, expectedNodes map[workflow.NodeID]execution.NodeExecutionStatus, verifyDuplicate bool, failingNode workflow.NodeID, pauseFirstResult bool) {
	t.Helper()
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
	suffix := time.Now().UTC().Format("150405000000000")
	kafkaConfig := config.KafkaConfig{Enabled: true, Brokers: []string{broker}, EngineClientID: "chain-engine-" + suffix, WorkerClientID: "chain-worker-" + suffix, CommandTopic: "miletos.workflow.node.commands.v1", EventTopic: "miletos.workflow.node.events.v1", WorkerGroupID: "chain-workers-closure-v3", EngineGroupID: "chain-results-closure-v3", MaxMessageBytes: 1024 * 1024}
	coordinator := engine.AsyncCoordinator{Outbox: store, Codec: codec, Topic: kafkaConfig.CommandTopic}
	asyncRunner, _ := engine.NewAsyncRunner(dependencies, store, coordinator)
	syncRunner, _ := engine.NewSyncRunner(dependencies)
	service, _ := engine.NewExecutionService(syncRunner, asyncRunner)

	workerConsumer, _ := transport.NewConsumer(kafkaConfig.Brokers, kafkaConfig.WorkerClientID, kafkaConfig.WorkerGroupID, kafkaConfig.CommandTopic)
	workerPool, _ := worker.NewPool(ctx, 2, 4)
	workerIdentity, _ := repository.NewConsumerIdentity(kafkaConfig.WorkerGroupID)
	registryExecutor := worker.RegistryExecutor{Registry: executors, Plugins: plugins, Limits: limits, Store: store}
	var workerExecutor worker.Executor = registryExecutor
	if failingNode != "" {
		workerExecutor = worker.ExecutorFunc(func(ctx context.Context, command messaging.NodeCommand, record repository.NodeExecutionRecord) (runtime.NodeResult, error) {
			if command.Metadata().NodeID() != failingNode {
				return registryExecutor.Execute(ctx, command, record)
			}
			failure, failureErr := runtime.NewRuntimeFailure(runtime.FailureCategoryExecution, "CP08_DETERMINISTIC_FAILURE", "deterministic integration failure", false, nil)
			if failureErr != nil {
				return runtime.NodeResult{}, failureErr
			}
			return runtime.NewNodeFailureResult(failure)
		})
	}
	workerRuntime, _ := worker.NewWorker(codec, store, store, workerExecutor, workerConsumer, workerPool, workerIdentity, worker.NewCompletionBuilder(codec, kafkaConfig.EventTopic))
	engineIdentity, _ := repository.NewConsumerIdentity(kafkaConfig.EngineGroupID)
	processor, _ := engine.NewAsyncResultProcessor(codec, store, store, plugins, coordinator, engineIdentity)
	resultKafkaConsumer, _ := transport.NewConsumer(kafkaConfig.Brokers, kafkaConfig.EngineClientID, kafkaConfig.EngineGroupID, kafkaConfig.EventTopic)
	resultConsumer, _ := engine.NewAsyncResultConsumer(resultKafkaConsumer, processor)
	producer, _ := transport.NewProducer(kafkaConfig.Brokers, kafkaConfig.EngineClientID, kafkaConfig.MaxMessageBytes)
	publisher, _ := transport.NewOutboxPublisher(store, producer, "chain-publisher-"+suffix, 50, 25*time.Millisecond, time.Minute)
	defer producer.Close()
	executionID := execution.WorkflowExecutionID("chain-execution-" + suffix)
	definition := buildDefinition(t, suffix)
	request, err := engine.NewAsyncExecutionRequest(executionID, definition, "chain-correlation-"+suffix, nil)
	if err != nil {
		t.Fatal(err)
	}
	errorsCh := make(chan error, 3)
	go func() { errorsCh <- workerRuntime.Run(ctx) }()
	resultReceived := make(chan struct{})
	resultRelease := make(chan struct{})
	if pauseFirstResult {
		var firstResult sync.Once
		go func() {
			errorsCh <- resultKafkaConsumer.Run(ctx, func(handlerCtx context.Context, delivery transport.Delivery) error {
				transportResult, decodeErr := codec.DecodeNodeResultEvent(delivery.Value)
				if decodeErr != nil || transportResult.Envelope.WorkflowExecutionID != executionID.String() {
					return processor.HandleDelivery(handlerCtx, delivery)
				}
				var waitErr error
				firstResult.Do(func() {
					close(resultReceived)
					select {
					case <-resultRelease:
					case <-handlerCtx.Done():
						waitErr = handlerCtx.Err()
					}
				})
				if waitErr != nil {
					return waitErr
				}
				return processor.HandleDelivery(handlerCtx, delivery)
			})
		}()
	} else {
		go func() { errorsCh <- resultConsumer.Run(ctx) }()
	}
	go func() { errorsCh <- publisher.Run(ctx) }()
	time.Sleep(750 * time.Millisecond)

	serviceResult, err := service.Run(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	asyncResult, ok := serviceResult.AsyncResult()
	if !ok || asyncResult.ScheduledRoots != expectedRoots {
		t.Fatalf("async result roots=%d exists=%t", asyncResult.ScheduledRoots, ok)
	}
	if pauseFirstResult {
		select {
		case <-resultReceived:
		case <-time.After(20 * time.Second):
			t.Fatal("engine result consumer did not receive the first durable result")
		}
		interruptedRecord, readErr := store.GetWorkflowExecution(ctx, definition.CompanyID(), executionID)
		if readErr != nil || interruptedRecord.Status() != execution.WorkflowExecutionStatusRunning {
			t.Fatalf("interrupted workflow status=%s error=%v", interruptedRecord.Status(), readErr)
		}
		sourceExecutionID := execution.NodeExecutionID(executionID.String() + "/node/source")
		if _, readErr = store.GetDurableWorkerResult(ctx, definition.CompanyID(), executionID, sourceExecutionID, 1); readErr != nil {
			t.Fatalf("durable result before engine resume: %v", readErr)
		}
		close(resultRelease)
	}
	deadline := time.Now().Add(45 * time.Second)
	var workflowRecord repository.WorkflowExecutionRecord
	for time.Now().Before(deadline) {
		workflowRecord, err = store.GetWorkflowExecution(ctx, definition.CompanyID(), executionID)
		if err == nil && workflowRecord.Status().IsTerminal() {
			break
		}
		select {
		case runErr := <-errorsCh:
			if runErr != nil && ctx.Err() == nil {
				t.Fatalf("runtime stopped: %v", runErr)
			}
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if workflowRecord.Status() != expectedWorkflow {
		t.Fatalf("workflow status=%s error=%v", workflowRecord.Status(), err)
	}
	for nodeID, expectedStatus := range expectedNodes {
		nodeExecutionID := execution.NodeExecutionID(executionID.String() + "/node/" + nodeID.String())
		record, readErr := store.GetNodeExecution(ctx, definition.CompanyID(), executionID, nodeExecutionID)
		if readErr != nil || record.Status() != expectedStatus {
			t.Fatalf("node %s status=%s error=%v", nodeID, record.Status(), readErr)
		}
		if expectedStatus != execution.NodeExecutionStatusSkipped {
			if _, readErr = store.GetDurableWorkerResult(ctx, definition.CompanyID(), executionID, nodeExecutionID, 1); readErr != nil {
				t.Fatalf("node %s durable result: %v", nodeID, readErr)
			}
		}
	}
	if verifyDuplicate {
		verifyDuplicateSourceResult(t, ctx, store, producer, codec, kafkaConfig, engineIdentity, definition, executionID)
	}
	cancel()
	resultKafkaConsumer.Close()
	workerConsumer.Close()
	_ = workerPool.Shutdown(context.Background())
}

func verifyDuplicateSourceResult(t *testing.T, ctx context.Context, store *postgres.Store, producer *transport.Producer, codec kafkav1.Codec, kafkaConfig config.KafkaConfig, engineIdentity repository.ConsumerIdentity, definition workflow.WorkflowDefinition, executionID execution.WorkflowExecutionID) {
	t.Helper()
	sourceExecutionID := execution.NodeExecutionID(executionID.String() + "/node/source")
	durable, err := store.GetDurableWorkerResult(ctx, definition.CompanyID(), executionID, sourceExecutionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := durable.Result()
	commandDigest := sha256.Sum256([]byte("node-command|" + executionID.String() + "|" + sourceExecutionID.String() + "|1"))
	commandID := "node-command-" + hex.EncodeToString(commandDigest[:])
	resultDigest := sha256.Sum256([]byte(commandID))
	resultID := "worker-result-" + hex.EncodeToString(resultDigest[:])
	metadata, err := messaging.NewMessageMetadata(messaging.MessageMetadataParams{MessageID: resultID, CreatedAt: durable.FinishedAt(), CompanyID: definition.CompanyID(), WorkflowID: definition.ID(), WorkflowExecutionID: executionID, NodeID: "source", NodeExecutionID: sourceExecutionID, Attempt: 1, CorrelationID: "chain-correlation-" + executionID.String()[len("chain-execution-"):], CausationID: commandID})
	if err != nil {
		t.Fatal(err)
	}
	event, err := messaging.NewNodeResultEvent(metadata, result, durable.StartedAt(), durable.FinishedAt())
	if err != nil {
		t.Fatal(err)
	}
	transportEvent, _ := kafkav1.ToNodeResultEventV1(event, 1024*1024)
	payload, _ := codec.EncodeNodeResultEvent(transportEvent)
	if err := producer.Publish(ctx, kafkaConfig.EventTopic, executionID.String(), payload, map[string]string{"message_type": kafkav1.MessageTypeNodeResultEventV1}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	inputs, err := store.ListAsyncNodeInputs(ctx, definition.CompanyID(), executionID, execution.NodeExecutionID(executionID.String()+"/node/pass"))
	if err != nil || len(inputs) != 1 {
		t.Fatalf("duplicate result inputs=%d error=%v", len(inputs), err)
	}
	processed, err := store.HasProcessedInboxMessage(ctx, engineIdentity, repository.MessageID(resultID))
	if err != nil || !processed {
		t.Fatalf("duplicate result inbox=%t error=%v", processed, err)
	}
}

func chainDefinition(t *testing.T, suffix string) workflow.WorkflowDefinition {
	t.Helper()
	source, _ := workflow.NewNodeDefinition("source", core.StaticInputPluginType, core.CorePluginVersion, []byte(`{"value":{"chain":true}}`), nil)
	pass, _ := workflow.NewNodeDefinition("pass", core.PassThroughPluginType, core.CorePluginVersion, []byte(`{}`), nil)
	terminal, _ := workflow.NewNodeDefinition("terminal", core.TerminalPluginType, core.CorePluginVersion, []byte(`{}`), nil)
	edge1, _ := workflow.NewEdgeDefinition("source-pass", source.ID(), core.OutputPortName, pass.ID(), core.InputPortName)
	edge2, _ := workflow.NewEdgeDefinition("pass-terminal", pass.ID(), core.OutputPortName, terminal.ID(), core.InputPortName)
	definition, err := workflow.NewWorkflowDefinition(workflow.WorkflowID("chain-workflow-"+suffix), workflow.CompanyID("chain-company-"+suffix), "CP08 async chain", 1, []workflow.NodeDefinition{source, pass, terminal}, []workflow.EdgeDefinition{edge1, edge2}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func fanGraphDefinition(t *testing.T, suffix string) workflow.WorkflowDefinition {
	t.Helper()
	sourceA, _ := workflow.NewNodeDefinition("source-a", core.StaticInputPluginType, core.CorePluginVersion, []byte(`{"value":{"source":"a"}}`), nil)
	sourceB, _ := workflow.NewNodeDefinition("source-b", core.StaticInputPluginType, core.CorePluginVersion, []byte(`{"value":{"source":"b"}}`), nil)
	passA, _ := workflow.NewNodeDefinition("pass-a", core.PassThroughPluginType, core.CorePluginVersion, []byte(`{}`), nil)
	terminal, _ := workflow.NewNodeDefinition("terminal", core.TerminalPluginType, core.CorePluginVersion, []byte(`{}`), nil)
	edges := []workflow.EdgeDefinition{
		mustEngineEdge(t, "a-pass", sourceA.ID(), passA.ID()),
		mustEngineEdge(t, "a-terminal", sourceA.ID(), terminal.ID()),
		mustEngineEdge(t, "b-terminal", sourceB.ID(), terminal.ID()),
	}
	definition, err := workflow.NewWorkflowDefinition(workflow.WorkflowID("fan-workflow-"+suffix), workflow.CompanyID("fan-company-"+suffix), "CP08 multiple roots fan graph", 1, []workflow.NodeDefinition{sourceA, sourceB, passA, terminal}, edges, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func mustEngineEdge(t *testing.T, id string, source, target workflow.NodeID) workflow.EdgeDefinition {
	t.Helper()
	edge, err := workflow.NewEdgeDefinition(workflow.EdgeID(id), source, core.OutputPortName, target, core.InputPortName)
	if err != nil {
		t.Fatal(err)
	}
	return edge
}
