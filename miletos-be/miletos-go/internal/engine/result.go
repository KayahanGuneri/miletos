package engine

import (
	"fmt"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type ExecutionResult struct {
	workflowExecution    execution.WorkflowExecution
	nodeExecutionsByNode map[workflow.NodeID]execution.NodeExecution
	nodeExecutionOrder   []workflow.NodeID
	executedNodeOrder    []workflow.NodeID
	terminalOutputs      map[workflow.NodeID]runtime.Payload
	nodeFailures         map[workflow.NodeID]runtime.RuntimeFailure
	workflowFailure      *runtime.RuntimeFailure
	stalled              bool
	initialized          bool
}

func (result ExecutionResult) WorkflowExecution() execution.WorkflowExecution {
	return result.workflowExecution
}
func (result ExecutionResult) Status() execution.WorkflowExecutionStatus {
	return result.workflowExecution.Status()
}
func (result ExecutionResult) IsTerminal() bool {
	return result.initialized &&
		result.workflowExecution.IsTerminal()
}
func (result ExecutionResult) IsSucceeded() bool {
	return result.initialized && result.workflowExecution.Status() ==
		execution.WorkflowExecutionStatusSucceeded
}
func (result ExecutionResult) IsFailed() bool {
	return result.initialized && result.workflowExecution.Status() ==
		execution.WorkflowExecutionStatusFailed
}
func (result ExecutionResult) IsCancelled() bool {
	return result.initialized && result.workflowExecution.Status() ==
		execution.WorkflowExecutionStatusCancelled
}
func (result ExecutionResult) IsTimedOut() bool {
	return result.initialized && result.workflowExecution.Status() ==
		execution.WorkflowExecutionStatusTimedOut
}
func (result ExecutionResult) IsRejected() bool {
	return result.initialized && result.workflowExecution.Status() ==
		execution.WorkflowExecutionStatusRejected
}
func (result ExecutionResult) IsStalled() bool {
	return result.initialized && result.stalled
}
func (result ExecutionResult) NodeExecutionOrder() []workflow.NodeID {
	if len(result.nodeExecutionOrder) == 0 {
		return nil
	}
	return append([]workflow.NodeID(nil),
		result.nodeExecutionOrder...)
}
func (result ExecutionResult) ExecutedNodeOrder() []workflow.NodeID {
	if len(result.executedNodeOrder) == 0 {
		return nil
	}
	return append([]workflow.NodeID(nil), result.executedNodeOrder...,
	)
}
func (result ExecutionResult) NodeExecution(nodeID workflow.NodeID) (
	execution.NodeExecution, bool, error,
) {
	normalizedNodeID, err := workflow.NewNodeID(nodeID.String())
	if err != nil {
		return execution.NodeExecution{},
			false, newValidationError("nodeID",
				err.Error())
	}
	nodeExecution, exists := result.nodeExecutionsByNode[normalizedNodeID]
	if !exists {
		return execution.NodeExecution{},
			false, nil
	}
	return nodeExecution, true,
		nil
}
func (result ExecutionResult) NodeExecutions() []execution.NodeExecution {
	if len(result.nodeExecutionOrder) == 0 {
		return nil
	}
	nodeExecutions := make(
		[]execution.NodeExecution, 0, len(result.nodeExecutionOrder),
	)
	for _, nodeID := range result.nodeExecutionOrder {
		nodeExecution, exists := result.nodeExecutionsByNode[nodeID]
		if !exists {
			continue
		}
		nodeExecutions = append(nodeExecutions,
			nodeExecution)
	}
	return nodeExecutions
}
func (result ExecutionResult) TerminalOutput(nodeID workflow.NodeID,
) (runtime.Payload, bool,
	error) {
	normalizedNodeID, err := workflow.NewNodeID(
		nodeID.String())
	if err != nil {
		return runtime.Payload{}, false, newValidationError(
			"nodeID", err.Error())
	}
	payload, exists :=
		result.terminalOutputs[normalizedNodeID]
	if !exists {
		return runtime.Payload{}, false, nil
	}
	return payload,
		true, nil
}
func (result ExecutionResult) TerminalOutputs() map[workflow.NodeID]runtime.Payload {
	if len(result.terminalOutputs) == 0 {
		return nil
	}
	outputs := make(map[workflow.NodeID]runtime.Payload, len(result.terminalOutputs))
	for nodeID, payload := range result.terminalOutputs {
		outputs[nodeID] = payload
	}
	return outputs
}
func (result ExecutionResult) NodeFailure(nodeID workflow.NodeID) (
	runtime.RuntimeFailure, bool, error,
) {
	normalizedNodeID, err := workflow.NewNodeID(nodeID.String())
	if err != nil {
		return runtime.RuntimeFailure{},
			false, newValidationError("nodeID",
				err.Error())
	}
	failure, exists := result.nodeFailures[normalizedNodeID]
	if !exists {
		return runtime.RuntimeFailure{},
			false, nil
	}
	return failure, true,
		nil
}
func (result ExecutionResult) NodeFailures() map[workflow.NodeID]runtime.RuntimeFailure {
	if len(result.nodeFailures) == 0 {
		return nil
	}
	failures := make(
		map[workflow.NodeID]runtime.RuntimeFailure, len(result.nodeFailures))
	for nodeID, failure := range result.nodeFailures {
		failures[nodeID] = failure
	}
	return failures
}
func (result ExecutionResult) WorkflowFailure() (
	runtime.RuntimeFailure, bool) {
	if result.workflowFailure == nil {
		return runtime.RuntimeFailure{}, false
	}
	return *result.workflowFailure,
		true
}

type schedulerExecutionState struct {
	executedNodeOrder []workflow.NodeID
	executedNodes     map[workflow.NodeID]struct{}
	terminalOutputs   map[workflow.NodeID]runtime.Payload
	nodeFailures      map[workflow.NodeID]runtime.RuntimeFailure
	workflowFailure   *runtime.RuntimeFailure
	stalled           bool
}

func newSchedulerExecutionState() schedulerExecutionState {
	return schedulerExecutionState{executedNodes: make(map[workflow.NodeID]struct{}), terminalOutputs: make(
		map[workflow.NodeID]runtime.Payload),
		nodeFailures: make(map[workflow.NodeID]runtime.RuntimeFailure),
	}
}
func (state *schedulerExecutionState) recordNodeExecution(nodeID workflow.NodeID) error {
	if state == nil {
		return newValidationError("schedulerExecutionState",
			"must not be nil")
	}
	normalizedNodeID, err := workflow.NewNodeID(nodeID.String())
	if err != nil {
		return newValidationError(
			"nodeID", err.Error())
	}
	if _, exists := state.executedNodes[normalizedNodeID]; exists {
		return fmt.Errorf("node %s is already recorded in the execution order", normalizedNodeID)
	}
	state.executedNodes[normalizedNodeID] = struct{}{}
	state.executedNodeOrder = append(state.executedNodeOrder, normalizedNodeID)
	return nil
}
func (state *schedulerExecutionState) recordNodeResult(
	nodeID workflow.NodeID, result runtime.NodeResult) error {
	if state == nil {
		return newValidationError("schedulerExecutionState",
			"must not be nil")
	}
	normalizedNodeID, err := workflow.NewNodeID(nodeID.String())
	if err != nil {
		return newValidationError(
			"nodeID", err.Error())
	}
	if _, exists := state.executedNodes[normalizedNodeID]; !exists {
		return fmt.Errorf("node %s must be recorded as executed before its result is recorded", normalizedNodeID)
	}
	if !result.IsValid() {
		return newValidationError("nodeResult",
			"must be valid")
	}
	if result.HasTerminalOutput() {
		if _, exists := state.terminalOutputs[normalizedNodeID]; exists {
			return fmt.Errorf("terminal output for node %s is already recorded", normalizedNodeID)
		}
		payload, exists := result.TerminalOutput()
		if !exists {
			return fmt.Errorf(
				"node %s reports terminal output but does not expose a payload", normalizedNodeID)
		}
		state.terminalOutputs[normalizedNodeID] =
			payload
	}
	if result.IsFailure() {
		failure, exists := result.Failure()
		if !exists {
			return fmt.Errorf("failed result for node %s does not expose a RuntimeFailure", normalizedNodeID)
		}
		return state.recordNodeFailure(normalizedNodeID, failure)
	}
	return nil
}
func (state *schedulerExecutionState) recordNodeFailure(nodeID workflow.NodeID, failure runtime.RuntimeFailure,
) error {
	if state == nil {
		return newValidationError(
			"schedulerExecutionState", "must not be nil")
	}
	normalizedNodeID, err := workflow.NewNodeID(
		nodeID.String())
	if err != nil {
		return newValidationError("nodeID", err.Error())
	}
	if !failure.IsValid() {
		return newValidationError("failure",
			"must be valid")
	}
	if _, exists := state.nodeFailures[normalizedNodeID]; exists {
		return fmt.Errorf(
			"failure for node %s is already recorded", normalizedNodeID)
	}
	state.nodeFailures[normalizedNodeID] =
		failure
	return nil
}
func (state *schedulerExecutionState) recordWorkflowFailure(
	failure runtime.RuntimeFailure, stalled bool) error {
	if state == nil {
		return newValidationError("schedulerExecutionState",
			"must not be nil")
	}
	if !failure.IsValid() {
		return newValidationError(
			"failure", "must be valid")
	}
	if state.workflowFailure != nil {
		return fmt.Errorf("workflow failure is already recorded")
	}
	copiedFailure := failure
	state.workflowFailure = &copiedFailure
	state.stalled = stalled
	return nil
}
func newExecutionResultFromPrepared(prepared *preparedExecution, state schedulerExecutionState,
) (ExecutionResult, error,
) {
	if prepared == nil {
		return ExecutionResult{},
			newValidationError("preparedExecution", "must not be nil")
	}
	if prepared.workflowExecution == nil {
		return ExecutionResult{}, newValidationError(
			"preparedExecution.workflowExecution", "must not be nil")
	}
	if len(prepared.nodeExecutionOrder) !=
		len(prepared.nodeExecutionsByNode) {
		return ExecutionResult{}, fmt.Errorf(
			"node execution order contains %d entries while execution registry contains %d", len(prepared.nodeExecutionOrder), len(prepared.nodeExecutionsByNode),
		)
	}
	nodeExecutionsByNode := make(map[workflow.NodeID]execution.NodeExecution, len(prepared.nodeExecutionsByNode))
	nodeExecutionOrder := append(
		[]workflow.NodeID(nil), prepared.nodeExecutionOrder...)
	for _, nodeID := range nodeExecutionOrder {
		nodeExecution, exists :=
			prepared.nodeExecutionsByNode[nodeID]
		if !exists || nodeExecution == nil {
			return ExecutionResult{}, fmt.Errorf("node execution for node %s is unavailable",
				nodeID)
		}
		nodeExecutionsByNode[nodeID] = *nodeExecution
	}
	executedNodeOrder := append(
		[]workflow.NodeID(nil), state.executedNodeOrder...)
	seenExecutedNodes := make(map[workflow.NodeID]struct{},
		len(executedNodeOrder))
	for _, nodeID := range executedNodeOrder {
		if _, exists := nodeExecutionsByNode[nodeID]; !exists {
			return ExecutionResult{},
				fmt.Errorf("executed node %s does not exist in the node execution registry", nodeID)
		}
		if _, exists := seenExecutedNodes[nodeID]; exists {
			return ExecutionResult{}, fmt.Errorf(
				"executed node order contains duplicate node %s", nodeID)
		}
		seenExecutedNodes[nodeID] =
			struct{}{}
	}
	terminalOutputs := make(map[workflow.NodeID]runtime.Payload, len(state.terminalOutputs))
	for nodeID, payload := range state.terminalOutputs {
		terminalOutputs[nodeID] = payload
	}
	nodeFailures := make(map[workflow.NodeID]runtime.RuntimeFailure,
		len(state.nodeFailures))
	for nodeID, failure := range state.nodeFailures {
		nodeFailures[nodeID] = failure
	}
	var workflowFailure *runtime.RuntimeFailure
	if state.workflowFailure != nil {
		copiedFailure :=
			*state.workflowFailure
		workflowFailure =
			&copiedFailure
	}
	return ExecutionResult{workflowExecution: *prepared.workflowExecution,
		nodeExecutionsByNode: nodeExecutionsByNode, nodeExecutionOrder: nodeExecutionOrder,
		executedNodeOrder: executedNodeOrder,
		terminalOutputs:   terminalOutputs, nodeFailures: nodeFailures,
		workflowFailure: workflowFailure,
		stalled:         state.stalled, initialized: true,
	}, nil
}
