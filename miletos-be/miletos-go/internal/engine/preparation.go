package engine

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	sharedclock "miletos-go/internal/shared/clock"
)

type executionPreparation struct {
	workflowExecution *execution.WorkflowExecution
	validationReport  PreflightValidationReport
	prepared          *preparedExecution
}

func (preparation executionPreparation) WorkflowExecution() execution.WorkflowExecution {
	if preparation.workflowExecution == nil {
		return execution.WorkflowExecution{}
	}
	return *preparation.workflowExecution
}
func (preparation executionPreparation,
) ValidationReport() PreflightValidationReport {
	return preparation.validationReport
}
func (preparation executionPreparation,
) IsPrepared() bool {
	if preparation.prepared == nil || preparation.workflowExecution == nil {
		return false
	}
	return preparation.workflowExecution.Status() == execution.WorkflowExecutionStatusRunning
}
func (preparation executionPreparation,
) IsRejected() bool {
	if preparation.workflowExecution == nil {
		return false
	}
	return preparation.workflowExecution.Status() ==
		execution.WorkflowExecutionStatusRejected
}

type preparedExecution struct {
	request              ExecutionRequest
	dependencies         EngineDependencies
	plan                 validatedExecutionPlan
	workflowExecution    *execution.WorkflowExecution
	nodeExecutionsByNode map[workflow.NodeID]*execution.NodeExecution
	nodeExecutionOrder   []workflow.NodeID
	executionContext     *runtime.ExecutionContext
}

func prepareExecution(parent context.Context,
	request ExecutionRequest, dependencies EngineDependencies) (
	executionPreparation, error) {
	if parent == nil {
		return executionPreparation{}, newValidationError(
			"context", "must not be nil")
	}
	if !request.IsValid() {
		return executionPreparation{}, newValidationError("request",
			"must be valid")
	}
	if !dependencies.IsValid() {
		return executionPreparation{},
			newValidationError("dependencies", "must be valid")
	}
	createdAt, err := executionTime(dependencies.Clock(), "workflowExecution.createdAt")
	if err != nil {
		return executionPreparation{}, err
	}
	definition := request.Definition()
	workflowExecution, err := execution.NewWorkflowExecution(
		request.WorkflowExecutionID(), definition.CompanyID(), definition.ID(),
		definition.Revision(), request.Mode(), createdAt,
	)
	if err != nil {
		return executionPreparation{},
			fmt.Errorf("create workflow execution: %w", err)
	}
	workflowExecutionPointer := &workflowExecution
	err = dependencies.LifecycleRecorder().RecordWorkflowCreation(
		parent, WorkflowCreationObservation{Request: request,
			WorkflowExecution: *workflowExecutionPointer},
	)
	if err != nil {
		return executionPreparation{},
			fmt.Errorf("record workflow creation: %w", err)
	}
	validatingAt, err := executionTime(dependencies.Clock(), "workflowExecution.validatingAt")
	if err != nil {
		return executionPreparation{}, err
	}
	beforeValidation :=
		*workflowExecutionPointer
	err = workflowExecutionPointer.StartValidation(
		validatingAt)
	if err != nil {
		return executionPreparation{}, fmt.Errorf("start workflow validation: %w",
			err)
	}
	err = dependencies.LifecycleRecorder().
		RecordWorkflowTransition(parent, WorkflowTransitionObservation{
			Request: request, Before: beforeValidation,
			After:        *workflowExecutionPointer,
			TransitionAt: validatingAt})
	if err != nil {
		return executionPreparation{}, fmt.Errorf(
			"record workflow validation start: %w", err)
	}
	plan, validationReport, err :=
		validateExecutionRequest(request, dependencies)
	if err != nil {
		return executionPreparation{},
			fmt.Errorf("validate execution request: %w", err)
	}
	if !validationReport.IsValid() {
		return rejectInvalidExecution(parent,
			request, dependencies, workflowExecutionPointer,
			validationReport)
	}
	edgeRuntimes, err := buildPreparedEdgeRuntimes(
		plan, dependencies.RuntimeLimits())
	if err != nil {
		return executionPreparation{}, fmt.Errorf(
			"prepare edge runtimes: %w", err)
	}
	nodeCreatedAt, err := executionTime(
		dependencies.Clock(), "nodeExecutions.createdAt")
	if err != nil {
		return executionPreparation{}, err
	}
	nodeExecutionsByNode, nodeExecutionOrder, err := buildPendingNodeExecutions(
		plan, workflowExecutionPointer.ID(), nodeCreatedAt,
	)
	if err != nil {
		return executionPreparation{},
			fmt.Errorf("prepare node executions: %w", err)
	}
	retryPolicy, hasRetryPolicy := dependencies.RetryPolicy()
	nodeCreationItems, err := buildNodeExecutionCreationItems(plan,
		nodeExecutionsByNode, nodeExecutionOrder, retryPolicy, hasRetryPolicy)
	if err != nil {
		return executionPreparation{}, fmt.Errorf(
			"build node execution creation observation: %w", err)
	}
	err = dependencies.
		LifecycleRecorder().RecordNodeExecutionsCreation(parent,
		NodeExecutionsCreationObservation{Request: request,
			WorkflowExecution: *workflowExecutionPointer, Items: nodeCreationItems,
		})
	if err != nil {
		return executionPreparation{}, fmt.Errorf("record node execution creation: %w",
			err)
	}
	startedAt, err := executionTime(dependencies.Clock(),
		"workflowExecution.startedAt")
	if err != nil {
		return executionPreparation{}, err
	}
	beforeStart := *workflowExecutionPointer
	err = workflowExecutionPointer.Start(startedAt)
	if err != nil {
		return executionPreparation{}, fmt.Errorf(
			"start workflow execution: %w", err)
	}
	executionContext, err :=
		runtime.NewExecutionContext(parent, *workflowExecutionPointer,
			request.CorrelationID(), edgeRuntimes, request.InitialVariables(),
		)
	if err != nil {
		return executionPreparation{},
			fmt.Errorf("create execution context: %w", err)
	}
	err = dependencies.LifecycleRecorder().RecordWorkflowTransition(
		parent, WorkflowTransitionObservation{Request: request,
			Before: beforeStart,
			After:  *workflowExecutionPointer, TransitionAt: startedAt,
		})
	if err != nil {
		return executionPreparation{}, fmt.Errorf("record workflow execution start: %w",
			err)
	}
	prepared := &preparedExecution{request: request,
		dependencies: dependencies,
		plan:         plan, workflowExecution: workflowExecutionPointer,
		nodeExecutionsByNode: nodeExecutionsByNode,
		nodeExecutionOrder: append([]workflow.NodeID(nil), nodeExecutionOrder...,
		), executionContext: executionContext,
	}
	return executionPreparation{
		workflowExecution: workflowExecutionPointer, validationReport: validationReport,
		prepared: prepared}, nil
}
func rejectInvalidExecution(
	parent context.Context, request ExecutionRequest, dependencies EngineDependencies,
	workflowExecution *execution.WorkflowExecution, validationReport PreflightValidationReport) (
	executionPreparation, error) {
	if workflowExecution == nil {
		return executionPreparation{}, newValidationError(
			"workflowExecution", "must not be nil")
	}
	validationFailure, err :=
		newWorkflowValidationFailure(validationReport)
	if err != nil {
		return executionPreparation{}, fmt.Errorf(
			"create workflow validation failure: %w", err)
	}
	rejectedAt, err := executionTime(
		dependencies.Clock(), "workflowExecution.rejectedAt")
	if err != nil {
		return executionPreparation{}, err
	}
	beforeRejection := *workflowExecution
	err = workflowExecution.Reject(rejectedAt)
	if err != nil {
		return executionPreparation{},
			fmt.Errorf("reject workflow execution: %w", err)
	}
	err = dependencies.LifecycleRecorder().RecordWorkflowTransition(
		parent, WorkflowTransitionObservation{Request: request,
			Before: beforeRejection,
			After:  *workflowExecution, TransitionAt: rejectedAt,
			Failure:    validationFailure,
			HasFailure: true})
	if err != nil {
		return executionPreparation{}, fmt.Errorf(
			"record workflow validation rejection: %w", err)
	}
	return executionPreparation{
		workflowExecution: workflowExecution, validationReport: validationReport,
	}, nil
}
func buildNodeExecutionCreationItems(plan validatedExecutionPlan, nodeExecutionsByNode map[workflow.NodeID]*execution.NodeExecution,
	nodeExecutionOrder []workflow.NodeID, retryPolicy execution.RetryPolicy,
	hasRetryPolicy bool) ([]NodeExecutionCreationItem,
	error) {
	if len(nodeExecutionOrder) !=
		len(nodeExecutionsByNode) {
		return nil, fmt.Errorf("node execution order contains %d entries while node execution registry contains %d",
			len(nodeExecutionOrder), len(nodeExecutionsByNode))
	}
	if hasRetryPolicy && !retryPolicy.IsValid() {
		return nil, newValidationError(
			"retryPolicy", "must be valid when provided")
	}
	items := make(
		[]NodeExecutionCreationItem, 0, len(nodeExecutionOrder),
	)
	for _, nodeID := range nodeExecutionOrder {
		nodeDefinition, exists := plan.graph.Node(nodeID)
		if !exists {
			return nil, fmt.Errorf("node definition %s is unavailable", nodeID)
		}
		nodeExecution, exists := nodeExecutionsByNode[nodeID]
		if !exists ||
			nodeExecution == nil {
			return nil, fmt.Errorf("node execution %s is unavailable",
				nodeID)
		}
		items = append(items,
			NodeExecutionCreationItem{Definition: nodeDefinition,
				Execution:   *nodeExecution,
				RetryPolicy: retryPolicy, HasRetryPolicy: hasRetryPolicy})
	}
	return items, nil
}
func buildPreparedEdgeRuntimes(
	plan validatedExecutionPlan, limits runtime.RuntimeLimits) (
	[]*runtime.EdgeRuntime, error) {
	edges := plan.graph.Edges()
	if len(edges) == 0 {
		return nil, nil
	}
	edgeRuntimes := make([]*runtime.EdgeRuntime,
		0, len(edges))
	for _, edge := range edges {
		targetNode, found :=
			plan.graph.Node(edge.TargetNodeID())
		if !found {
			return nil, fmt.Errorf("target node %s for edge %s is unavailable",
				edge.TargetNodeID(), edge.ID())
		}
		targetDescriptor, found :=
			plan.descriptorsByNode[edge.TargetNodeID()]
		if !found {
			return nil, fmt.Errorf(
				"target descriptor for node %s is unavailable", edge.TargetNodeID())
		}
		edgeRuntime, err :=
			runtime.NewEdgeRuntime(edge, targetNode,
				targetDescriptor, limits)
		if err != nil {
			return nil, fmt.Errorf("create runtime for edge %s: %w",
				edge.ID(), err)
		}
		edgeRuntimes = append(
			edgeRuntimes, edgeRuntime)
	}
	return edgeRuntimes, nil
}
func buildPendingNodeExecutions(
	plan validatedExecutionPlan, workflowExecutionID execution.WorkflowExecutionID, createdAt time.Time,
) (map[workflow.NodeID]*execution.NodeExecution, []workflow.NodeID,
	error) {
	if createdAt.IsZero() {
		return nil, nil, newValidationError(
			"nodeExecutions.createdAt", "must not be zero")
	}
	nodeExecutionsByNode := make(
		map[workflow.NodeID]*execution.NodeExecution, len(plan.topologicalOrder))
	nodeExecutionOrder := make([]workflow.NodeID,
		0, len(plan.topologicalOrder))
	for _, nodeID := range plan.topologicalOrder {
		nodeExecutionID, err :=
			execution.NewNodeExecutionID(fmt.Sprintf("%s/node/%s",
				workflowExecutionID.String(), nodeID.String()),
			)
		if err != nil {
			return nil,
				nil, fmt.Errorf("create execution ID for node %s: %w",
					nodeID, err)
		}
		nodeExecution, err :=
			execution.NewNodeExecution(nodeExecutionID, workflowExecutionID,
				nodeID, createdAt)
		if err != nil {
			return nil, nil,
				fmt.Errorf("create execution for node %s: %w", nodeID,
					err)
		}
		nodeExecutionPointer := new(execution.NodeExecution)
		*nodeExecutionPointer = nodeExecution
		nodeExecutionsByNode[nodeID] = nodeExecutionPointer
		nodeExecutionOrder = append(nodeExecutionOrder,
			nodeID)
	}
	return nodeExecutionsByNode, nodeExecutionOrder,
		nil
}
func executionTime(clock sharedclock.Clock, field string,
) (time.Time, error,
) {
	if clock == nil {
		return time.Time{},
			newValidationError("clock", "must not be nil")
	}
	current := clock.Now()
	if current.IsZero() {
		return time.Time{}, newValidationError(
			field, "clock must return a non-zero time")
	}
	return current.UTC(), nil
}

const failureCodeWorkflowValidationRejected = "WORKFLOW_VALIDATION_REJECTED"

func newWorkflowValidationFailure(report PreflightValidationReport,
) (runtime.RuntimeFailure, error,
) {
	if report.IsValid() {
		return runtime.RuntimeFailure{},
			newValidationError("validationReport", "must be invalid for a rejected workflow")
	}
	if report.Len() <= 0 {
		return runtime.RuntimeFailure{}, newValidationError(
			"validationReport", "must contain at least one issue")
	}
	return runtime.NewRuntimeFailure(
		runtime.FailureCategoryValidation, failureCodeWorkflowValidationRejected, "Workflow validation failed",
		false, map[string]string{"issueCount": strconv.Itoa(
			report.Len()), "structuralIssueCount": strconv.Itoa(
			len(report.StructuralIssues()),
		), "pluginIssueCount": strconv.Itoa(len(
			report.PluginIssues())),
			"engineIssueCount": strconv.Itoa(len(report.EngineIssues()))},
	)
}
