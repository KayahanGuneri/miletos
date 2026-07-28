package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"

	"miletos-go/internal/engine/graph"
	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/messaging"
	repository "miletos-go/internal/ports/persistence"
)

type AuthoritativeExecutionStore interface {
	repository.WorkflowExecutionReader
	repository.WorkflowSnapshotRepository
	repository.AsyncInputStore
	repository.AsyncContextStore
}
type RegistryExecutor struct {
	Registry runtime.ExecutorRegistry
	Plugins  plugin.Registry
	Limits   runtime.RuntimeLimits
	Store    AuthoritativeExecutionStore
}

type AttemptExecutionOutcome struct {
	result          runtime.NodeResult
	technicalDetail string
}

func NewAttemptExecutionOutcome(
	result runtime.NodeResult,
	technicalDetail string,
) (AttemptExecutionOutcome, error) {
	if !result.IsValid() {
		return AttemptExecutionOutcome{},
			fmt.Errorf("worker attempt result must be valid")
	}
	normalizedDetail := strings.TrimSpace(technicalDetail)
	if normalizedDetail != "" {
		runes := []rune(normalizedDetail)
		if len(runes) >
			repository.MaximumDurableWorkerTechnicalDetailCharacters {
			normalizedDetail = string(
				runes[:repository.MaximumDurableWorkerTechnicalDetailCharacters],
			)
		}
	}
	return AttemptExecutionOutcome{
		result: result, technicalDetail: normalizedDetail,
	}, nil
}

func (outcome AttemptExecutionOutcome) Result() runtime.NodeResult {
	return outcome.result
}

func (outcome AttemptExecutionOutcome) TechnicalDetail() (string, bool) {
	return outcome.technicalDetail, outcome.technicalDetail != ""
}

func (outcome AttemptExecutionOutcome) IsValid() bool {
	normalized, err := NewAttemptExecutionOutcome(
		outcome.result, outcome.technicalDetail,
	)
	return err == nil &&
		normalized.technicalDetail == outcome.technicalDetail
}

func (executor RegistryExecutor) Execute(ctx context.Context,
	command messaging.NodeCommand,
	record repository.NodeExecutionRecord,
) (AttemptExecutionOutcome, error) {
	if ctx == nil || executor.Registry.IsEmpty() || executor.Plugins.IsEmpty() || !executor.Limits.IsValid() || executor.Store == nil {
		return AttemptExecutionOutcome{}, fmt.Errorf("authoritative worker executor dependencies must be valid")
	}
	if record.Status() != execution.NodeExecutionStatusRunning {
		return AttemptExecutionOutcome{}, fmt.Errorf("persisted node execution must be RUNNING")
	}
	workflowRecord, err := executor.Store.GetWorkflowExecution(ctx, record.CompanyID(), record.WorkflowExecutionID())
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	if workflowRecord.Mode() != execution.ExecutionModeAsync || workflowRecord.Status() != execution.WorkflowExecutionStatusRunning {
		return AttemptExecutionOutcome{}, fmt.Errorf("persisted workflow execution must be running asynchronously")
	}
	snapshot, err := executor.Store.GetByID(ctx, record.CompanyID(), workflowRecord.SnapshotID())
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	definition, err := workflow.DecodePersistedDefinition(snapshot.DefinitionJSON().Bytes())
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	if definition.CompanyID() != record.CompanyID() || definition.ID() != workflowRecord.WorkflowID() ||
		definition.Revision() != workflowRecord.WorkflowRevision() {
		return AttemptExecutionOutcome{}, fmt.Errorf("persisted workflow snapshot identity does not match execution")
	}
	builtGraph, graphReport := graph.BuildValidated(definition)
	if !graphReport.IsValid() {
		return AttemptExecutionOutcome{}, fmt.Errorf("persisted workflow snapshot graph is invalid")
	}
	nodeDefinition, exists := builtGraph.Node(record.NodeID())
	if !exists {
		return AttemptExecutionOutcome{}, fmt.Errorf("persisted node does not belong to workflow snapshot")
	}
	if err := validateAuthoritativeCommand(command, workflowRecord, record, nodeDefinition); err != nil {
		return AttemptExecutionOutcome{}, err
	}
	edgeRuntimes, err := executor.buildEdgeRuntimes(builtGraph)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	variables, err := executor.loadVariables(ctx, record)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	nodeInput, incomingEdgeIDs, err := executor.loadNodeInput(ctx, builtGraph, record)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	workflowExecution, err := runningWorkflowExecution(workflowRecord)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	nodeExecution, err := runningNodeExecution(record)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	correlation, ok := workflowRecord.CorrelationID()
	if !ok {
		return AttemptExecutionOutcome{}, fmt.Errorf("persisted asynchronous workflow requires correlation identity")
	}
	executionContext, err := runtime.NewExecutionContext(ctx, workflowExecution, correlation, edgeRuntimes, variables)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	nodeContext, err := runtime.NewNodeExecutionContext(executionContext, nodeExecution, incomingEdgeIDs)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	identity, err := plugin.NewPluginIdentity(nodeDefinition.PluginType(), nodeDefinition.PluginVersion())
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	descriptor, found := executor.Plugins.Lookup(identity)
	if !found || !descriptor.Distribution().SupportsAsync() {
		return AttemptExecutionOutcome{}, fmt.Errorf("worker plugin %s is not distributable", identity.String())
	}
	nodeExecutor, found := executor.Registry.Lookup(identity)
	if !found {
		return AttemptExecutionOutcome{}, fmt.Errorf("worker plugin %s is not registered", identity.String())
	}
	return invokeNodeExecutor(
		nodeExecutor, nodeContext, nodeInput,
		nodeDefinition.Configuration(),
	)
}

func invokeNodeExecutor(
	nodeExecutor runtime.NodeExecutor,
	nodeContext *runtime.NodeExecutionContext,
	nodeInput runtime.NodeInput,
	configuration workflow.JSONObject,
) (AttemptExecutionOutcome, error) {
	var (
		result       runtime.NodeResult
		executionErr error
		didPanic     bool
		panicValue   any
		panicStack   []byte
	)
	func() {
		completed := false
		defer func() {
			if !completed {
				didPanic = true
				panicValue = recover()
				panicStack = debug.Stack()
			}
		}()
		result, executionErr = nodeExecutor.Execute(
			nodeContext, nodeInput, configuration,
		)
		completed = true
	}()
	if didPanic {
		return technicalFailureOutcome(
			"NODE_EXECUTOR_PANIC",
			"Node executor panicked",
			fmt.Sprintf(
				"panic type: %T\npanic value: %v\n%s",
				panicValue, panicValue, panicStack,
			),
		)
	}
	if executionErr != nil {
		return technicalFailureOutcome(
			"NODE_EXECUTOR_ERROR",
			"Node executor returned a technical error",
			executionErr.Error(),
		)
	}
	if !result.IsValid() {
		return technicalFailureOutcome(
			"NODE_EXECUTOR_ERROR",
			"Node executor returned a technical error",
			"node executor returned an invalid result",
		)
	}
	return NewAttemptExecutionOutcome(result, "")
}

func technicalFailureOutcome(
	code string,
	safeMessage string,
	technicalDetail string,
) (AttemptExecutionOutcome, error) {
	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryInternal,
		code,
		safeMessage,
		false,
		nil,
	)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	result, err := runtime.NewNodeFailureResult(failure)
	if err != nil {
		return AttemptExecutionOutcome{}, err
	}
	return NewAttemptExecutionOutcome(result, technicalDetail)
}
func validateAuthoritativeCommand(command messaging.NodeCommand,
	workflowRecord repository.WorkflowExecutionRecord, record repository.NodeExecutionRecord, node workflow.NodeDefinition,
) error {
	metadata := command.Metadata()
	if metadata.CompanyID() != record.CompanyID() || metadata.WorkflowExecutionID() != record.WorkflowExecutionID() || metadata.WorkflowID() != workflowRecord.WorkflowID() {
		return fmt.Errorf("Kafka command workflow identity does not match authoritative persisted execution")
	}
	if metadata.NodeExecutionID() != record.ID() || metadata.NodeID() != record.NodeID() || metadata.Attempt() != record.Attempt() {
		return fmt.Errorf("Kafka command node identity does not match authoritative persisted execution")
	}
	if command.NodeDefinition().PluginType() != node.PluginType() || command.NodeDefinition().PluginVersion() != node.PluginVersion() {
		return fmt.Errorf("Kafka command plugin identity does not match authoritative persisted execution")
	}
	if !jsonObjectsEqual(command.NodeDefinition().Configuration().Bytes(), node.Configuration().Bytes()) {
		return fmt.Errorf("Kafka command configuration does not match authoritative persisted execution")
	}
	return nil
}
func jsonObjectsEqual(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return jsonValuesEqual(leftValue, rightValue)
}

func jsonValuesEqual(left, right any) bool {
	switch leftValue := left.(type) {
	case nil:
		return right == nil
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	case float64:
		rightValue, ok := right.(float64)
		return ok && leftValue == rightValue
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for index := range leftValue {
			if !jsonValuesEqual(leftValue[index], rightValue[index]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, value := range leftValue {
			rightItem, exists := rightValue[key]
			if !exists || !jsonValuesEqual(value, rightItem) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
func (executor RegistryExecutor) buildEdgeRuntimes(builtGraph graph.Graph) ([]*runtime.EdgeRuntime, error) {
	edges := builtGraph.Edges()
	runtimes := make([]*runtime.EdgeRuntime, 0, len(edges))
	for _, edge := range edges {
		target, found := builtGraph.Node(edge.TargetNodeID())
		if !found {
			return nil, fmt.Errorf("edge target node %s is unavailable", edge.TargetNodeID())
		}
		identity, err := plugin.NewPluginIdentity(target.PluginType(), target.PluginVersion())
		if err != nil {
			return nil, err
		}
		descriptor, found := executor.Plugins.Lookup(identity)
		if !found {
			return nil, fmt.Errorf("edge target plugin %s is unavailable", identity.String())
		}
		edgeRuntime, err := runtime.NewEdgeRuntime(edge, target, descriptor, executor.Limits)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, edgeRuntime)
	}
	return runtimes, nil
}
func (executor RegistryExecutor) loadVariables(ctx context.Context, record repository.NodeExecutionRecord) (map[string]runtime.RuntimeValue, error) {
	variables, err := executor.Store.ListAsyncContextVariables(ctx, record.CompanyID(), record.WorkflowExecutionID())
	if err != nil {
		return nil, err
	}
	values := make(map[string]runtime.RuntimeValue, len(variables))
	for _, variable := range variables {
		if variable.CompanyID() != record.CompanyID() || variable.WorkflowExecutionID() != record.WorkflowExecutionID() {
			return nil, fmt.Errorf("async context variable escaped execution scope")
		}
		values[variable.Key()] = variable.Value()
	}
	return values, nil
}
func (executor RegistryExecutor) loadNodeInput(
	ctx context.Context, builtGraph graph.Graph, record repository.NodeExecutionRecord,
) (runtime.NodeInput, []workflow.EdgeID, error) {
	incoming := builtGraph.IncomingEdges(record.NodeID())
	sort.Slice(incoming, func(left, right int) bool { return incoming[left].ID().String() < incoming[right].ID().String() })
	incomingByID := make(map[workflow.EdgeID]workflow.EdgeDefinition, len(incoming))
	incomingIDs := make([]workflow.EdgeID, 0, len(incoming))
	for _, edge := range incoming {
		incomingByID[edge.ID()] = edge
		incomingIDs = append(incomingIDs, edge.ID())
	}
	inputs, err := executor.Store.ListAsyncNodeInputs(ctx, record.CompanyID(), record.WorkflowExecutionID(), record.ID())
	if err != nil {
		return runtime.NodeInput{}, nil, err
	}
	byPort := make(map[string][]runtime.Payload)
	for _, input := range inputs {
		edge, found := incomingByID[input.EdgeID()]
		if !found || input.CompanyID() != record.CompanyID() ||
			input.WorkflowExecutionID() != record.WorkflowExecutionID() || input.TargetNodeExecutionID() != record.ID() || input.TargetNodeID() != record.NodeID() ||
			input.SourceNodeID() != edge.SourceNodeID() || input.TargetNodeID() != edge.TargetNodeID() || input.SourceOutputPort() != edge.SourceOutputPort() ||
			input.TargetInputPort() != edge.TargetInputPort() {
			return runtime.NodeInput{}, nil, fmt.Errorf("durable async input does not match persisted graph")
		}
		byPort[edge.TargetInputPort()] = append(byPort[edge.TargetInputPort()], input.Payload())
	}
	nodeInput, err := runtime.NewNodeInput(byPort)
	if err != nil {
		return runtime.NodeInput{}, nil, err
	}
	return nodeInput, incomingIDs, nil
}
func runningWorkflowExecution(record repository.WorkflowExecutionRecord) (execution.WorkflowExecution, error) {
	value, err := execution.NewWorkflowExecution(record.ID(), record.CompanyID(), record.WorkflowID(), record.WorkflowRevision(), record.Mode(), record.CreatedAt())
	if err != nil {
		return execution.WorkflowExecution{}, err
	}
	validatingAt, ok := record.ValidatingAt()
	if !ok {
		return execution.WorkflowExecution{}, fmt.Errorf("persisted workflow validating time is missing")
	}
	if err := value.StartValidation(validatingAt); err != nil {
		return execution.WorkflowExecution{}, err
	}
	startedAt, ok := record.StartedAt()
	if !ok {
		return execution.WorkflowExecution{}, fmt.Errorf("persisted workflow start time is missing")
	}
	if err := value.Start(startedAt); err != nil {
		return execution.WorkflowExecution{}, err
	}
	return value, nil
}
func runningNodeExecution(record repository.NodeExecutionRecord) (execution.NodeExecution, error) {
	value, err := execution.NewNodeExecution(record.ID(), record.WorkflowExecutionID(), record.NodeID(), record.CreatedAt())
	if err != nil {
		return execution.NodeExecution{}, err
	}
	readyAt, ok := record.ReadyAt()
	if !ok {
		return execution.NodeExecution{}, fmt.Errorf("persisted node ready time is missing")
	}
	if err := value.MarkReady(readyAt); err != nil {
		return execution.NodeExecution{}, err
	}
	queuedAt, ok := record.QueuedAt()
	if !ok {
		return execution.NodeExecution{}, fmt.Errorf("persisted node queued time is missing")
	}
	if err := value.Queue(queuedAt); err != nil {
		return execution.NodeExecution{}, err
	}
	startedAt, ok := record.StartedAt()
	if !ok {
		return execution.NodeExecution{}, fmt.Errorf("persisted node start time is missing")
	}
	if err := value.Start(startedAt); err != nil {
		return execution.NodeExecution{}, err
	}
	return value, nil
}
