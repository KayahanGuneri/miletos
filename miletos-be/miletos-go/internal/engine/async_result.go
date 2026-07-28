package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"miletos-go/internal/engine/graph"
	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/messaging"
	repository "miletos-go/internal/ports/persistence"
)

type AsyncResultConsumer struct {
	consumer  messaging.ConsumerRunner
	processor *AsyncResultProcessor
}

func NewAsyncResultConsumer(consumer messaging.ConsumerRunner, processor *AsyncResultProcessor) (*AsyncResultConsumer, error) {
	if consumer == nil || processor == nil {
		return nil, fmt.Errorf("async result consumer dependencies must be valid")
	}
	return &AsyncResultConsumer{consumer: consumer, processor: processor}, nil
}
func (consumer *AsyncResultConsumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.consumer == nil || consumer.processor == nil {
		return fmt.Errorf("async result consumer is not initialized")
	}
	return consumer.consumer.Run(ctx, consumer.processor.HandleDelivery)
}
func (consumer *AsyncResultConsumer) Close() {
	if consumer != nil && consumer.consumer != nil {
		consumer.consumer.Close()
	}
}

type AsyncResultReadStore interface {
	repository.WorkflowExecutionReader
	repository.WorkflowSnapshotRepository
}
type AsyncResultProcessor struct {
	codec       messaging.Codec
	store       AsyncResultReadStore
	transactor  repository.AsyncPersistenceTransactor
	plugins     plugin.Registry
	coordinator AsyncCoordinator
	consumerID  repository.ConsumerIdentity
	now         func() time.Time
}

func NewAsyncResultProcessor(
	codec messaging.Codec, store AsyncResultReadStore, transactor repository.AsyncPersistenceTransactor,
	plugins plugin.Registry, coordinator AsyncCoordinator, consumerID repository.ConsumerIdentity,
) (*AsyncResultProcessor, error) {
	if store == nil || transactor == nil || plugins.IsEmpty() || coordinator.Topic == "" || consumerID.String() == "" {
		return nil, fmt.Errorf("async result processor dependencies must be valid")
	}
	return &AsyncResultProcessor{
		codec: codec, store: store, transactor: transactor, plugins: plugins, coordinator: coordinator, consumerID: consumerID, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (processor *AsyncResultProcessor) HandleDelivery(ctx context.Context, delivery messaging.Delivery) error {
	event, err := processor.codec.DecodeResult(delivery.Value)
	if err != nil {
		return fmt.Errorf("decode async node result: %w", err)
	}
	metadata := event.Metadata()
	workflowRecord, err := processor.store.GetWorkflowExecution(ctx, metadata.CompanyID(), metadata.WorkflowExecutionID())
	if err != nil {
		return err
	}
	if workflowRecord.Mode() != execution.ExecutionModeAsync || workflowRecord.WorkflowID() != metadata.WorkflowID() {
		return fmt.Errorf("result event does not match asynchronous workflow execution")
	}
	snapshot, err := processor.store.GetByID(ctx, metadata.CompanyID(), workflowRecord.SnapshotID())
	if err != nil {
		return err
	}
	definition, err := workflow.DecodePersistedDefinition(snapshot.DefinitionJSON().Bytes())
	if err != nil {
		return err
	}
	builtGraph, report := graph.BuildValidated(definition)
	if !report.IsValid() {
		return fmt.Errorf("persisted workflow graph is invalid")
	}
	if node, found := builtGraph.Node(metadata.NodeID()); !found || node.PluginType().String() == "" {
		return fmt.Errorf("result node does not belong to persisted graph")
	}
	processedAt := processor.now()
	sourcePosition, err := repository.NewInboxSourcePosition(delivery.Topic, delivery.Partition, delivery.Offset)
	if err != nil {
		return err
	}
	return processor.transactor.WithinAsyncPersistenceTransaction(ctx, func(ctx context.Context, tx repository.AsyncPersistenceTransaction) error {
		if err := tx.LockAsyncWorkflowCoordination(ctx, metadata.CompanyID(), metadata.WorkflowExecutionID()); err != nil {
			return err
		}
		duplicate, err := tx.HasProcessedInboxMessage(ctx, processor.consumerID, repository.MessageID(metadata.MessageID()))
		if err != nil || duplicate {
			return err
		}
		currentWorkflow, err := tx.GetAsyncWorkflowExecution(ctx, metadata.CompanyID(), metadata.WorkflowExecutionID())
		if err != nil {
			return err
		}
		nodeRecord, err := tx.GetAsyncNodeExecution(
			ctx, metadata.CompanyID(),
			metadata.WorkflowExecutionID(),
			metadata.NodeExecutionID(),
		)
		if err != nil {
			return err
		}
		if nodeRecord.NodeID() != metadata.NodeID() {
			return fmt.Errorf(
				"result event does not match persisted node execution",
			)
		}
		ignore, err := classifyAsyncResultAttempt(
			nodeRecord, metadata.Attempt(),
		)
		if err != nil {
			return err
		}
		if ignore || currentWorkflow.Status().IsTerminal() {
			return processor.recordResultInbox(ctx, tx, event, &sourcePosition, processedAt, repository.InboxProcessingIgnored)
		}
		durable, err := tx.GetDurableWorkerResult(ctx, metadata.CompanyID(), metadata.WorkflowExecutionID(), metadata.NodeExecutionID(), metadata.Attempt())
		if err != nil {
			return err
		}
		if err := validateDurableResultEvent(event, durable); err != nil {
			return err
		}
		inbox, err := processor.buildResultInbox(
			event, &sourcePosition, processedAt,
			repository.InboxProcessingApplied,
		)
		if err != nil {
			return err
		}
		application, retryScheduled, err :=
			processor.buildAsyncResultApplication(
				builtGraph, currentWorkflow, nodeRecord,
				durable, inbox, metadata.MessageID(),
				processedAt,
			)
		if err != nil {
			return err
		}
		if err := tx.ApplyAsyncNodeResult(
			ctx, application,
		); err != nil {
			return err
		}
		if retryScheduled {
			return nil
		}
		if result, exists := durable.Result(); exists && result.IsSuccess() {
			if err := processor.applyContextChanges(ctx, tx, durable, result.ContextChanges(), processedAt); err != nil {
				return err
			}
			if err := processor.routeAndSchedule(ctx, tx, builtGraph, currentWorkflow, durable, result, metadata.MessageID(), processedAt); err != nil {
				return err
			}
		} else {
			if err := processor.skipDescendants(ctx, tx, builtGraph, durable, processedAt); err != nil {
				return err
			}
		}
		if err := processor.completeWorkflowIfTerminal(ctx, tx, currentWorkflow, processedAt); err != nil {
			return err
		}
		return nil
	})
}

func classifyAsyncResultAttempt(
	record repository.NodeExecutionRecord,
	resultAttempt int16,
) (bool, error) {
	if resultAttempt < record.Attempt() {
		return true, nil
	}
	if resultAttempt > record.Attempt() {
		return false,
			fmt.Errorf("result attempt is ahead of persisted node execution")
	}
	switch record.Status() {
	case execution.NodeExecutionStatusRunning:
		return false, nil
	case execution.NodeExecutionStatusRetryPending:
		return true, nil
	default:
		if record.Status().IsTerminal() {
			return true, nil
		}
		return false, fmt.Errorf(
			"node execution state %s cannot apply a result",
			record.Status(),
		)
	}
}

func (processor *AsyncResultProcessor) buildAsyncResultApplication(
	builtGraph graph.Graph,
	workflowRecord repository.WorkflowExecutionRecord,
	nodeRecord repository.NodeExecutionRecord,
	durable repository.DurableWorkerResult,
	inbox repository.InboxMessage,
	causationID string,
	appliedAt time.Time,
) (repository.AsyncNodeResultApplication, bool, error) {
	attempt, err := execution.NewAttemptNumber(nodeRecord.Attempt())
	if err != nil {
		return repository.AsyncNodeResultApplication{}, false, err
	}
	targetStatus, attemptStatus, failure, hasFailure, err :=
		asyncDurableResultStatus(durable)
	if err != nil {
		return repository.AsyncNodeResultApplication{}, false, err
	}
	var (
		decision       execution.RetryDecision
		nextAttemptAt  time.Time
		retryOutbox    repository.OutboxMessage
		retryScheduled bool
	)
	if hasFailure &&
		attemptStatus != execution.NodeExecutionStatusCancelled {
		if policy, exists := nodeRecord.RetryPolicy(); exists {
			decision, err = DecideRetry(
				policy, attempt, failure, appliedAt, nil,
			)
			if err != nil {
				return repository.AsyncNodeResultApplication{}, false, err
			}
			if decision.Kind() == execution.RetryDecisionRetry {
				backoff, _ := decision.Backoff()
				nextAttemptAt = appliedAt.Add(backoff)
				targetStatus = execution.NodeExecutionStatusRetryPending
				retryOutbox, err = processor.buildRetryCommandOutbox(
					builtGraph, workflowRecord, nodeRecord,
					decision, causationID, appliedAt,
					nextAttemptAt,
				)
				if err != nil {
					return repository.AsyncNodeResultApplication{}, false, err
				}
				retryScheduled = true
			}
		}
	}
	var failureSummary []byte
	if hasFailure &&
		attemptStatus != execution.NodeExecutionStatusCancelled {
		failureSummary, err = marshalAsyncFailureSummary(failure)
		if err != nil {
			return repository.AsyncNodeResultApplication{}, false, err
		}
	}
	completion, err := repository.NewNodeAttemptCompletion(
		repository.NodeAttemptCompletionParams{
			CompanyID:           durable.CompanyID(),
			WorkflowExecutionID: durable.WorkflowExecutionID(),
			NodeExecutionID:     durable.NodeExecutionID(),
			ExpectedAttempt:     attempt,
			Status:              attemptStatus,
			FinishedAt:          durable.FinishedAt(),
			FailureSummary:      failureSummary,
			RetryDecision:       decision,
			NextAttemptAt:       nextAttemptAt,
		},
	)
	if err != nil {
		return repository.AsyncNodeResultApplication{}, false, err
	}
	mutation, err := repository.NewNodeAttemptCompleteMutation(
		completion,
	)
	if err != nil {
		return repository.AsyncNodeResultApplication{}, false, err
	}
	application := repository.AsyncNodeResultApplication{
		CompanyID:               durable.CompanyID(),
		WorkflowExecutionID:     durable.WorkflowExecutionID(),
		NodeExecutionID:         durable.NodeExecutionID(),
		ExpectedNodeLockVersion: nodeRecord.LockVersion(),
		ExpectedNodeAttempt:     nodeRecord.Attempt(),
		TargetNodeStatus:        targetStatus,
		AppliedAt:               appliedAt,
		DurableWorkerResult:     durable,
		AttemptMutation:         mutation,
		RetryCommandOutbox:      retryOutbox,
		ResultInboxMessage:      inbox,
	}
	if !application.IsValid() {
		return repository.AsyncNodeResultApplication{}, false,
			fmt.Errorf("async node result application must be valid")
	}
	return application, retryScheduled, nil
}

func asyncDurableResultStatus(
	durable repository.DurableWorkerResult,
) (
	execution.NodeExecutionStatus,
	execution.NodeExecutionStatus,
	runtime.RuntimeFailure,
	bool,
	error,
) {
	switch durable.Status() {
	case repository.DurableWorkerResultSucceeded:
		return execution.NodeExecutionStatusSucceeded,
			execution.NodeExecutionStatusSucceeded,
			runtime.RuntimeFailure{}, false, nil
	case repository.DurableWorkerResultCancelled:
		failure, exists := durable.Failure()
		if !exists {
			return "", "", runtime.RuntimeFailure{}, false,
				fmt.Errorf("cancelled durable result must contain a failure")
		}
		return execution.NodeExecutionStatusCancelled,
			execution.NodeExecutionStatusCancelled,
			failure, true, nil
	case repository.DurableWorkerResultTimedOut:
		failure, exists := durable.Failure()
		if !exists {
			return "", "", runtime.RuntimeFailure{}, false,
				fmt.Errorf("timed-out durable result must contain a failure")
		}
		return execution.NodeExecutionStatusTimedOut,
			execution.NodeExecutionStatusTimedOut,
			failure, true, nil
	case repository.DurableWorkerResultFailed:
		result, exists := durable.Result()
		if !exists {
			return "", "", runtime.RuntimeFailure{}, false,
				fmt.Errorf("failed durable result must contain a runtime result")
		}
		failure, exists := result.Failure()
		if !exists {
			return "", "", runtime.RuntimeFailure{}, false,
				fmt.Errorf("failed durable result must contain a failure")
		}
		status := nodeTerminalStatusForFailure(failure)
		return status, status, failure, true, nil
	default:
		return "", "", runtime.RuntimeFailure{}, false,
			fmt.Errorf("durable worker result status is unsupported")
	}
}

func (processor *AsyncResultProcessor) buildRetryCommandOutbox(
	builtGraph graph.Graph,
	workflowRecord repository.WorkflowExecutionRecord,
	nodeRecord repository.NodeExecutionRecord,
	decision execution.RetryDecision,
	causationID string,
	createdAt time.Time,
	availableAt time.Time,
) (repository.OutboxMessage, error) {
	nextAttempt, exists := decision.NextAttempt()
	if !exists {
		return repository.OutboxMessage{},
			fmt.Errorf("retry decision must contain a next attempt")
	}
	nodeDefinition, exists := builtGraph.Node(nodeRecord.NodeID())
	if !exists {
		return repository.OutboxMessage{},
			fmt.Errorf("retry node does not belong to persisted graph")
	}
	input, err := runtime.NewNodeInput(nil)
	if err != nil {
		return repository.OutboxMessage{}, err
	}
	metadata, err := messaging.NewMessageMetadata(
		messaging.MessageMetadataParams{
			MessageID: asyncNodeCommandMessageID(
				nodeRecord.WorkflowExecutionID(),
				nodeRecord.ID(), nextAttempt.Int16(),
			),
			CreatedAt:           createdAt,
			CompanyID:           nodeRecord.CompanyID(),
			WorkflowID:          workflowRecord.WorkflowID(),
			WorkflowExecutionID: nodeRecord.WorkflowExecutionID(),
			NodeID:              nodeRecord.NodeID(),
			NodeExecutionID:     nodeRecord.ID(),
			Attempt:             nextAttempt.Int16(),
			CorrelationID:       mustWorkflowCorrelation(workflowRecord),
			CausationID:         causationID,
		},
	)
	if err != nil {
		return repository.OutboxMessage{}, err
	}
	command, err := messaging.NewNodeCommand(
		metadata, nodeDefinition, input, nil, nil,
	)
	if err != nil {
		return repository.OutboxMessage{}, err
	}
	return processor.coordinator.BuildNodeCommandOutboxAvailableAt(
		command, availableAt,
	)
}

func marshalAsyncFailureSummary(
	failure runtime.RuntimeFailure,
) ([]byte, error) {
	if !failure.IsValid() {
		return nil, fmt.Errorf("runtime failure must be valid")
	}
	return json.Marshal(struct {
		Category  string `json:"category"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	}{
		Category:  failure.Category().String(),
		Code:      failure.Code(),
		Message:   failure.Message(),
		Retryable: failure.Retryable(),
	})
}
func validateDurableResultEvent(event messaging.NodeResultEvent, durable repository.DurableWorkerResult) error {
	metadata := event.Metadata()
	if durable.CompanyID() != metadata.CompanyID() ||
		durable.WorkflowExecutionID() != metadata.WorkflowExecutionID() || durable.NodeExecutionID() != metadata.NodeExecutionID() || durable.NodeID() != metadata.NodeID() ||
		durable.Attempt() != metadata.Attempt() || string(durable.Status()) != event.Status().String() {
		return fmt.Errorf("transport result does not match authoritative durable result")
	}
	return nil
}
func (processor *AsyncResultProcessor) applyContextChanges(ctx context.Context, tx repository.AsyncPersistenceTransaction, durable repository.DurableWorkerResult,
	changes runtime.ContextChanges, at time.Time) error {
	for _, key := range changes.SetKeys() {
		value, exists, err := changes.SetValue(key)
		if err != nil || !exists {
			return fmt.Errorf("context set value %q is unavailable", key)
		}
		current, err := tx.GetAsyncContextVariable(ctx, durable.CompanyID(), durable.WorkflowExecutionID(), key)
		expectedVersion := int64(0)
		createdAt := at
		if err == nil {
			expectedVersion = current.Version()
			createdAt = current.CreatedAt()
		} else if !repository.IsNotFound(err) {
			return err
		}
		variable, err := repository.NewAsyncContextVariable(repository.AsyncContextVariableParams{CompanyID: durable.CompanyID(), WorkflowExecutionID: durable.WorkflowExecutionID(),
			Key: key, Value: value, CreatedAt: createdAt, UpdatedAt: at, Version: expectedVersion + 1})
		if err != nil {
			return err
		}
		write, err := repository.NewAsyncContextWrite(variable, expectedVersion)
		if err != nil {
			return err
		}
		if err := tx.CompareAndSwapAsyncContextVariable(ctx, write); err != nil {
			return err
		}
	}
	for _, key := range changes.DeleteKeys() {
		current, err := tx.GetAsyncContextVariable(ctx, durable.CompanyID(), durable.WorkflowExecutionID(), key)
		if repository.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		deletion, err := repository.NewAsyncContextDelete(durable.CompanyID(), durable.WorkflowExecutionID(), key, current.Version(), at)
		if err != nil {
			return err
		}
		if err := tx.DeleteAsyncContextVariable(ctx, deletion); err != nil {
			return err
		}
	}
	return nil
}
func (processor *AsyncResultProcessor) routeAndSchedule(
	ctx context.Context, tx repository.AsyncPersistenceTransaction, builtGraph graph.Graph,
	workflowRecord repository.WorkflowExecutionRecord, durable repository.DurableWorkerResult, result runtime.NodeResult,
	causationID string, at time.Time) error {
	outgoing := builtGraph.OutgoingEdges(durable.NodeID())
	targets := make(map[workflow.NodeID]struct{})
	for _, edge := range outgoing {
		targets[edge.TargetNodeID()] = struct{}{}
	}
	targetIDs := make([]workflow.NodeID, 0, len(targets))
	for id := range targets {
		targetIDs = append(targetIDs, id)
	}
	sort.Slice(targetIDs, func(left, right int) bool { return targetIDs[left].String() < targetIDs[right].String() })
	for _, targetID := range targetIDs {
		if err := tx.LockAsyncNodeCoordination(ctx, durable.CompanyID(), durable.WorkflowExecutionID(), targetID); err != nil {
			return err
		}
	}
	for _, edge := range outgoing {
		payloads, exists, err := result.OutputPayloads(edge.SourceOutputPort())
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if len(payloads) != 1 {
			return fmt.Errorf("CP-08 durable edge input requires exactly one payload per routed edge")
		}
		targetExecutionID := execution.NodeExecutionID(durable.WorkflowExecutionID().String() + "/node/" + edge.TargetNodeID().String())
		input, err := repository.NewAsyncNodeInput(repository.AsyncNodeInputParams{CompanyID: durable.CompanyID(), WorkflowExecutionID: durable.WorkflowExecutionID(),
			TargetNodeExecutionID: targetExecutionID, SourceNodeExecutionID: durable.NodeExecutionID(), TargetNodeID: edge.TargetNodeID(), SourceNodeID: edge.SourceNodeID(),
			EdgeID: edge.ID(), SourceOutputPort: edge.SourceOutputPort(), TargetInputPort: edge.TargetInputPort(),
			SourceAttempt: durable.Attempt(), Payload: payloads[0], CreatedAt: at})
		if err != nil {
			return err
		}
		if err := tx.CreateAsyncNodeInput(ctx, input); err != nil && !repository.IsConflict(err) {
			return err
		}
	}
	for _, targetID := range targetIDs {
		targetExecutionID := execution.NodeExecutionID(durable.WorkflowExecutionID().String() + "/node/" + targetID.String())
		nodeRecord, err := tx.GetAsyncNodeExecution(ctx, durable.CompanyID(), durable.WorkflowExecutionID(), targetExecutionID)
		if err != nil {
			return err
		}
		if nodeRecord.Status() != execution.NodeExecutionStatusPending {
			continue
		}
		ready, nodeInput, err := asyncTargetReady(ctx, tx, builtGraph, nodeRecord)
		if err != nil || !ready {
			if err != nil {
				return err
			}
			continue
		}
		variables, err := tx.ListAsyncContextVariables(ctx, durable.CompanyID(), durable.WorkflowExecutionID())
		if err != nil {
			return err
		}
		values := make(map[string]runtime.RuntimeValue, len(variables))
		for _, variable := range variables {
			values[variable.Key()] = variable.Value()
		}
		nodeDefinition, _ := builtGraph.Node(targetID)
		metadata, err := messaging.NewMessageMetadata(messaging.MessageMetadataParams{
			MessageID: asyncNodeCommandMessageID(durable.WorkflowExecutionID(), targetExecutionID, nodeRecord.Attempt()), CreatedAt: at, CompanyID: durable.CompanyID(),
			WorkflowID: workflowRecord.WorkflowID(), WorkflowExecutionID: durable.WorkflowExecutionID(), NodeID: targetID, NodeExecutionID: targetExecutionID,
			Attempt: nodeRecord.Attempt(), CorrelationID: mustWorkflowCorrelation(workflowRecord), CausationID: causationID})
		if err != nil {
			return err
		}
		command, err := messaging.NewNodeCommand(metadata, nodeDefinition, nodeInput, values, nil)
		if err != nil {
			return err
		}
		if err := processor.coordinator.ScheduleNodeCommandInTransaction(ctx, tx, command, nodeRecord.LockVersion(), at); err != nil {
			return err
		}
	}
	return nil
}
func asyncTargetReady(ctx context.Context, tx repository.AsyncPersistenceTransaction, builtGraph graph.Graph, record repository.NodeExecutionRecord) (bool, runtime.NodeInput,
	error) {
	incoming := builtGraph.IncomingEdges(record.NodeID())
	inputs, err := tx.ListAsyncNodeInputs(ctx, record.CompanyID(), record.WorkflowExecutionID(), record.ID())
	if err != nil {
		return false, runtime.NodeInput{}, err
	}
	byEdge := make(map[workflow.EdgeID]repository.AsyncNodeInput, len(inputs))
	byPort := make(map[string][]runtime.Payload)
	for _, input := range inputs {
		byEdge[input.EdgeID()] = input
		byPort[input.TargetInputPort()] = append(byPort[input.TargetInputPort()], input.Payload())
	}
	for _, edge := range incoming {
		input, exists := byEdge[edge.ID()]
		if !exists || input.SourceNodeID() != edge.SourceNodeID() || input.TargetInputPort() != edge.TargetInputPort() {
			return false, runtime.NodeInput{}, nil
		}
	}
	value, err := runtime.NewNodeInput(byPort)
	return err == nil, value, err
}
func (processor *AsyncResultProcessor) skipDescendants(ctx context.Context, tx repository.AsyncPersistenceTransaction, builtGraph graph.Graph,
	durable repository.DurableWorkerResult, at time.Time) error {
	queue := []workflow.NodeID{durable.NodeID()}
	seen := map[workflow.NodeID]struct{}{durable.NodeID(): {}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range builtGraph.OutgoingEdges(current) {
			target := edge.TargetNodeID()
			if _, exists := seen[target]; exists {
				continue
			}
			seen[target] = struct{}{}
			queue = append(queue, target)
			if err := tx.LockAsyncNodeCoordination(ctx, durable.CompanyID(), durable.WorkflowExecutionID(), target); err != nil {
				return err
			}
			nodeExecutionID := execution.NodeExecutionID(durable.WorkflowExecutionID().String() + "/node/" + target.String())
			record, err := tx.GetAsyncNodeExecution(ctx, durable.CompanyID(), durable.WorkflowExecutionID(), nodeExecutionID)
			if err != nil {
				return err
			}
			if record.Status() == execution.NodeExecutionStatusPending {
				if err := tx.SkipAsyncNodeExecution(ctx, durable.CompanyID(), durable.WorkflowExecutionID(), nodeExecutionID, record.LockVersion(), at); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (processor *AsyncResultProcessor) completeWorkflowIfTerminal(ctx context.Context, tx repository.AsyncPersistenceTransaction, workflowRecord repository.WorkflowExecutionRecord,
	at time.Time) error {
	nodes, err := tx.ListAsyncNodeExecutions(ctx, workflowRecord.CompanyID(), workflowRecord.ID())
	if err != nil {
		return err
	}
	target := execution.WorkflowExecutionStatusSucceeded
	for _, node := range nodes {
		if !node.Status().IsTerminal() {
			return nil
		}
		switch node.Status() {
		case execution.NodeExecutionStatusFailed:
			target = execution.WorkflowExecutionStatusFailed
		case execution.NodeExecutionStatusTimedOut:
			if target != execution.WorkflowExecutionStatusFailed {
				target = execution.WorkflowExecutionStatusTimedOut
			}
		case execution.NodeExecutionStatusCancelled:
			if target == execution.WorkflowExecutionStatusSucceeded {
				target = execution.WorkflowExecutionStatusCancelled
			}
		}
	}
	current, err := tx.GetAsyncWorkflowExecution(ctx, workflowRecord.CompanyID(), workflowRecord.ID())
	if err != nil || current.Status().IsTerminal() {
		return err
	}
	return tx.CompleteAsyncWorkflow(ctx, repository.AsyncWorkflowCompletion{CompanyID: current.CompanyID(), WorkflowExecutionID: current.ID(),
		ExpectedLockVersion: current.LockVersion(), Status: target, FinishedAt: at})
}
func (processor *AsyncResultProcessor) recordResultInbox(ctx context.Context, tx repository.AsyncPersistenceTransaction, event messaging.NodeResultEvent,
	source *repository.InboxSourcePosition, at time.Time, result repository.InboxProcessingResult) error {
	inbox, err := processor.buildResultInbox(
		event, source, at, result,
	)
	if err != nil {
		return err
	}
	return tx.RecordInboxMessage(ctx, inbox)
}
func (processor *AsyncResultProcessor) buildResultInbox(
	event messaging.NodeResultEvent,
	source *repository.InboxSourcePosition,
	at time.Time,
	result repository.InboxProcessingResult,
) (repository.InboxMessage, error) {
	metadata := event.Metadata()
	return repository.NewInboxMessage(repository.InboxMessageParams{
		ConsumerIdentity: processor.consumerID, MessageID: repository.MessageID(metadata.MessageID()), CompanyID: metadata.CompanyID(),
		WorkflowExecutionID: metadata.WorkflowExecutionID(), NodeExecutionID: metadata.NodeExecutionID(), MessageType: processor.codec.ResultDescriptor().Type,
		MessageVersion: processor.codec.ResultDescriptor().Version, ProcessingResult: result,
		SourcePosition: source, ReceivedAt: at, ProcessedAt: at})
}
func mustWorkflowCorrelation(record repository.WorkflowExecutionRecord) string {
	value, _ := record.CorrelationID()
	return value
}
