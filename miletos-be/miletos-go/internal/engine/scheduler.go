package engine

import (
	"context"
	"errors"
	"fmt"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"sort"
)

type nodeReadinessState string

const (
	nodeReadinessReady      nodeReadinessState = "READY"
	nodeReadinessWaiting    nodeReadinessState = "WAITING"
	nodeReadinessBlocked    nodeReadinessState = "BLOCKED"
	nodeReadinessIneligible nodeReadinessState = "INELIGIBLE"
)

type nodeReadinessAssessment struct {
	state                 nodeReadinessState
	blockingUpstreamNodes []workflow.NodeID
}

func (assessment nodeReadinessAssessment) IsReady() bool {
	return assessment.state == nodeReadinessReady
}
func (assessment nodeReadinessAssessment) IsWaiting() bool {
	return assessment.state == nodeReadinessWaiting
}
func (assessment nodeReadinessAssessment) IsBlocked() bool {
	return assessment.state == nodeReadinessBlocked
}
func (assessment nodeReadinessAssessment) IsIneligible() bool {
	return assessment.state == nodeReadinessIneligible
}
func (assessment nodeReadinessAssessment) BlockingUpstreamNodes() []workflow.NodeID {
	if len(assessment.blockingUpstreamNodes) == 0 {
		return nil
	}
	copied := make(
		[]workflow.NodeID, len(assessment.blockingUpstreamNodes))
	copy(copied,
		assessment.blockingUpstreamNodes)
	return copied
}
func assessNodeReadiness(prepared *preparedExecution, nodeID workflow.NodeID,
) (nodeReadinessAssessment, error,
) {
	if prepared == nil {
		return nodeReadinessAssessment{},
			newValidationError("preparedExecution", "must not be nil")
	}
	if prepared.executionContext == nil {
		return nodeReadinessAssessment{}, newValidationError(
			"preparedExecution.executionContext", "must not be nil")
	}
	if _, exists := prepared.plan.graph.Node(nodeID); !exists {
		return nodeReadinessAssessment{}, newValidationError("nodeID",
			fmt.Sprintf("node %q is not present in the execution plan", nodeID.String()))
	}
	nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]
	if !exists || nodeExecution == nil {
		return nodeReadinessAssessment{},
			fmt.Errorf("node execution for node %s is unavailable", nodeID)
	}
	if nodeExecution.Status() != execution.NodeExecutionStatusPending {
		return nodeReadinessAssessment{
			state: nodeReadinessIneligible}, nil
	}
	incomingEdges := prepared.plan.graph.IncomingEdges(nodeID)
	if len(incomingEdges) == 0 {
		return nodeReadinessAssessment{
			state: nodeReadinessReady}, nil
	}
	sort.Slice(incomingEdges,
		func(left int, right int) bool {
			return incomingEdges[left].ID().
				String() < incomingEdges[right].ID().
				String()
		})
	waiting := false
	blockingNodes := make([]workflow.NodeID, 0)
	for _, edge := range incomingEdges {
		sourceNodeID := edge.SourceNodeID()
		sourceExecution, exists :=
			prepared.nodeExecutionsByNode[sourceNodeID]
		if !exists || sourceExecution == nil {
			return nodeReadinessAssessment{}, fmt.Errorf("source node execution %s for edge %s is unavailable",
				sourceNodeID, edge.ID())
		}
		switch sourceExecution.Status() {
		case execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusSkipped, execution.NodeExecutionStatusCancelled,
			execution.NodeExecutionStatusTimedOut:
			blockingNodes = append(
				blockingNodes, sourceNodeID)
			continue
		case execution.NodeExecutionStatusSucceeded:
			edgeRuntime, exists, err := prepared.executionContext.Edge(
				edge.ID())
			if err != nil {
				return nodeReadinessAssessment{}, fmt.Errorf("lookup runtime edge %s: %w",
					edge.ID(), err)
			}
			if !exists || edgeRuntime == nil {
				return nodeReadinessAssessment{}, fmt.Errorf("runtime edge %s is unavailable",
					edge.ID())
			}
			queue := edgeRuntime.Queue()
			if queue == nil {
				return nodeReadinessAssessment{}, fmt.Errorf("runtime edge %s contains no queue",
					edge.ID())
			}
			queueLength := queue.Len()
			switch {
			case queueLength == 0:
				waiting = true
			case queueLength == 1:
				// The incoming edge is ready for single-shot consumption.
			default:
				return nodeReadinessAssessment{},
					fmt.Errorf("runtime edge %s contains %d payloads; single-shot execution requires exactly one", edge.ID(),
						queueLength)
			}
		default:
			waiting = true
		}
	}
	if len(blockingNodes) > 0 {
		sort.Slice(blockingNodes,
			func(left int, right int) bool {
				return blockingNodes[left].String() < blockingNodes[right].String()
			})
		return nodeReadinessAssessment{state: nodeReadinessBlocked,
			blockingUpstreamNodes: append([]workflow.NodeID(nil), blockingNodes...,
			)}, nil
	}
	if waiting {
		return nodeReadinessAssessment{
			state: nodeReadinessWaiting}, nil
	}
	return nodeReadinessAssessment{state: nodeReadinessReady}, nil
}
func nextReadyNode(prepared *preparedExecution) (
	workflow.NodeID, bool, error,
) {
	if prepared == nil {
		return "",
			false, newValidationError("preparedExecution",
				"must not be nil")
	}
	for _, nodeID := range prepared.plan.topologicalOrder {
		assessment, err := assessNodeReadiness(
			prepared, nodeID)
		if err != nil {
			return "", false, err
		}
		if assessment.IsReady() {
			return nodeID, true, nil
		}
	}
	return "", false, nil
}

type outputRoutingOperation struct {
	edgeID      workflow.EdgeID
	sourcePort  string
	edgeRuntime *runtime.EdgeRuntime
	payload     runtime.Payload
}

func routeNodeOutputs(prepared *preparedExecution,
	nodeID workflow.NodeID, result runtime.NodeResult) error {
	operations, err := buildOutputRoutingOperations(prepared, nodeID,
		result)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		queue := operation.edgeRuntime.Queue()
		if queue == nil {
			return fmt.Errorf("runtime edge %s contains no queue during output routing",
				operation.edgeID)
		}
		_, _, err := queue.Push(operation.payload)
		if err != nil {
			return fmt.Errorf("route output port %q to edge %s: %w", operation.sourcePort,
				operation.edgeID, err)
		}
	}
	return nil
}
func buildOutputRoutingOperations(prepared *preparedExecution, nodeID workflow.NodeID,
	result runtime.NodeResult) ([]outputRoutingOperation,
	error) {
	if prepared == nil {
		return nil, newValidationError("preparedExecution", "must not be nil")
	}
	if prepared.executionContext == nil {
		return nil, newValidationError("preparedExecution.executionContext",
			"must not be nil")
	}
	if _, exists := prepared.plan.graph.Node(nodeID); !exists {
		return nil, newValidationError(
			"nodeID", fmt.Sprintf("node %q is not present in the execution plan",
				nodeID.String()))
	}
	descriptor, exists := prepared.plan.descriptorsByNode[nodeID]
	if !exists {
		return nil, fmt.Errorf("plugin descriptor for node %s is unavailable",
			nodeID)
	}
	if !result.IsValid() {
		return nil, newValidationError(
			"nodeResult", "must be valid")
	}
	if !result.IsSuccess() {
		return nil, newValidationError("nodeResult", "must be successful before output routing")
	}
	if result.HasTerminalOutput() {
		if result.HasRoutedOutputs() {
			return nil, newValidationError(
				"nodeResult", "must not contain routed outputs and terminal output together")
		}
		return nil, nil
	}
	outputPorts := result.OutputPorts()
	if len(outputPorts) == 0 {
		return nil, nil
	}
	sort.Strings(outputPorts)
	outgoingEdges := prepared.plan.graph.OutgoingEdges(nodeID)
	sort.Slice(outgoingEdges, func(left int, right int) bool {
		return outgoingEdges[left].ID().String() < outgoingEdges[right].ID().String()
	},
	)
	operations := make(
		[]outputRoutingOperation, 0)
	for _, outputPort := range outputPorts {
		if !descriptor.HasOutputPort(outputPort) {
			return nil, fmt.Errorf("node %s returned undeclared output port %q", nodeID,
				outputPort)
		}
		payloads, exists, err := result.OutputPayloads(outputPort)
		if err != nil {
			return nil, fmt.Errorf(
				"read output payloads for node %s port %q: %w", nodeID, outputPort,
				err)
		}
		if !exists {
			return nil, fmt.Errorf(
				"node %s output port %q is listed but contains no output entry", nodeID, outputPort,
			)
		}
		if len(payloads) != 1 {
			return nil, fmt.Errorf("node %s output port %q contains %d payloads; single-shot execution requires exactly one",
				nodeID, outputPort, len(payloads),
			)
		}
		payload := payloads[0]
		for _, edge := range outgoingEdges {
			if edge.SourceOutputPort() != outputPort {
				continue
			}
			edgeRuntime, exists, err := prepared.executionContext.Edge(edge.ID())
			if err != nil {
				return nil, fmt.Errorf(
					"lookup runtime edge %s: %w", edge.ID(), err,
				)
			}
			if !exists || edgeRuntime == nil {
				return nil, fmt.Errorf("runtime edge %s is unavailable",
					edge.ID())
			}
			if edgeRuntime.SourceNodeID() != nodeID {
				return nil, fmt.Errorf(
					"runtime edge %s belongs to source node %s instead of node %s", edge.ID(), edgeRuntime.SourceNodeID(),
					nodeID)
			}
			if edgeRuntime.SourceOutputPort() != outputPort {
				return nil, fmt.Errorf(
					"runtime edge %s uses source port %q instead of %q", edge.ID(), edgeRuntime.SourceOutputPort(),
					outputPort)
			}
			if edgeRuntime.TargetNodeID() != edge.TargetNodeID() {
				return nil, fmt.Errorf(
					"runtime edge %s target node does not match the execution plan", edge.ID())
			}
			if edgeRuntime.TargetInputPort() != edge.TargetInputPort() {
				return nil, fmt.Errorf("runtime edge %s target port does not match the execution plan", edge.ID())
			}
			if edgeRuntime.Queue() == nil {
				return nil, fmt.Errorf("runtime edge %s contains no queue",
					edge.ID())
			}
			operations = append(operations,
				outputRoutingOperation{edgeID: edge.ID(), sourcePort: outputPort,
					edgeRuntime: edgeRuntime, payload: payload},
			)
		}
	}
	return operations, nil
}

const (
	failureCodeNodeExecutionError  = "NODE_EXECUTION_ERROR"
	failureCodeWorkflowNodeFailure = "WORKFLOW_NODE_FAILURE"
	failureCodeWorkflowStalled     = "WORKFLOW_STALLED"
	failureCodeWorkflowCanceled    = "WORKFLOW_CANCELED"
	failureCodeWorkflowTimedOut    = "WORKFLOW_TIMED_OUT"
	failureCodeSchedulerInternal   = "SCHEDULER_INTERNAL_ERROR"
)

func runPreparedExecution(prepared *preparedExecution) (
	ExecutionResult, error) {
	if prepared == nil {
		return ExecutionResult{}, newValidationError(
			"preparedExecution", "must not be nil")
	}
	if prepared.workflowExecution == nil {
		return ExecutionResult{}, newValidationError("preparedExecution.workflowExecution",
			"must not be nil")
	}
	if prepared.executionContext == nil {
		return ExecutionResult{},
			newValidationError("preparedExecution.executionContext", "must not be nil")
	}
	if !prepared.dependencies.IsValid() {
		return ExecutionResult{}, newValidationError(
			"preparedExecution.dependencies", "must be valid")
	}
	if prepared.workflowExecution.Status() !=
		execution.WorkflowExecutionStatusRunning {
		return ExecutionResult{}, newValidationError(
			"preparedExecution.workflowExecution.status", fmt.Sprintf("must be RUNNING; current status is %s",
				prepared.workflowExecution.Status()))
	}
	state := newSchedulerExecutionState()
	for {
		contextErr :=
			prepared.executionContext.Err()
		if contextErr != nil {
			return finalizePreparedExecutionForContext(prepared, &state,
				contextErr)
		}
		_, err := skipBlockedPendingNodes(prepared)
		if err != nil {
			return failPreparedExecutionWithInternalError(
				prepared, &state, fmt.Errorf(
					"propagate blocked node state: %w", err),
			)
		}
		nodeID, ready, err := nextReadyNode(prepared)
		if err != nil {
			return failPreparedExecutionWithInternalError(prepared,
				&state, fmt.Errorf("select next ready node: %w",
					err))
		}
		if ready {
			result, executionErr := executeReadyNode(prepared,
				nodeID)
			started, startedErr := nodeExecutionStarted(prepared,
				nodeID)
			if startedErr != nil {
				return failPreparedExecutionWithInternalError(prepared, &state,
					fmt.Errorf("inspect execution start for node %s: %w", nodeID,
						startedErr))
			}
			if started {
				if err := state.recordNodeExecution(nodeID); err != nil {
					return failPreparedExecutionWithInternalError(prepared, &state,
						fmt.Errorf("record execution order for node %s: %w", nodeID,
							err))
				}
			}
			if executionErr != nil {
				if errors.Is(executionErr,
					context.DeadlineExceeded) {
					return finalizePreparedExecutionForContext(
						prepared, &state, context.DeadlineExceeded,
					)
				}
				if errors.Is(executionErr, context.Canceled) {
					return finalizePreparedExecutionForContext(prepared,
						&state, context.Canceled)
				}
				/*
					Preflight baÅŸarÄ±lÄ± olduÄŸu ve node READY seÃ§ildiÄŸi
					hÃ¢lde node baÅŸlamadan hata oluÅŸmasÄ± scheduler
					invariant ihlalidir.
				*/
				if !started {
					return failPreparedExecutionWithInternalError(
						prepared, &state, fmt.Errorf(
							"node %s failed before its execution started: %w", nodeID, executionErr,
						))
				}
				failure, err := newTechnicalNodeFailure(
					nodeID, executionErr)
				if err != nil {
					return failPreparedExecutionWithInternalError(prepared,
						&state, fmt.Errorf("create technical failure for node %s: %w",
							nodeID, err),
					)
				}
				if err := state.recordNodeFailure(nodeID, failure); err != nil {
					return failPreparedExecutionWithInternalError(prepared,
						&state, fmt.Errorf("record technical failure for node %s: %w",
							nodeID, err),
					)
				}
				continue
			}
			if !started {
				return failPreparedExecutionWithInternalError(prepared,
					&state, fmt.Errorf("node %s returned a result without entering RUNNING state",
						nodeID))
			}
			if err := state.recordNodeResult(
				nodeID, result); err != nil {
				return failPreparedExecutionWithInternalError(prepared, &state,
					fmt.Errorf("record result for node %s: %w", nodeID,
						err))
			}
			continue
		}
		allTerminal, err :=
			allNodeExecutionsTerminal(prepared)
		if err != nil {
			return failPreparedExecutionWithInternalError(prepared,
				&state, fmt.Errorf("inspect node terminal states: %w",
					err))
		}
		if allTerminal {
			return finalizePreparedExecutionFromNodeStates(prepared, &state)
		}
		return finalizeStalledPreparedExecution(prepared, &state)
	}
}
