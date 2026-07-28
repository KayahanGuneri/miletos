package persistence

import (
	"context"
	"fmt"
	"sync"

	"miletos-go/internal/engine"
	"miletos-go/internal/features/execution"
	repository "miletos-go/internal/ports/persistence"
)

type Recorder struct {
	store    repository.ExecutionLifecycleStore
	statesMu sync.Mutex
	states   map[string]*executionPersistenceState
}

type executionPersistenceState struct {
	mu              sync.Mutex
	initialized     bool
	workflow        repository.WorkflowExecutionRecord
	nodes           map[string]repository.NodeExecutionRecord
	terminalOutputs map[string]payloadSummary
}

var _ engine.LifecycleRecorder = (*Recorder)(nil)

func NewRecorder(store repository.ExecutionLifecycleStore,
) (*Recorder, error) {
	if store == nil {
		return nil, fmt.Errorf(
			"execution lifecycle store must not be nil")
	}
	return &Recorder{store: store,
		states: make(map[string]*executionPersistenceState),
	}, nil
}

func (recorder *Recorder) RecordWorkflowCreation(ctx context.Context, observation engine.WorkflowCreationObservation,
) error {
	if err := recorder.validateContextAndStore(ctx); err != nil {
		return err
	}
	if !observation.Request.IsValid() {
		return fmt.Errorf("workflow creation request must be valid")
	}
	workflowExecution := observation.WorkflowExecution
	if workflowExecution.Status() != execution.WorkflowExecutionStatusCreated {
		return fmt.Errorf(
			"workflow creation observation must contain CREATED status")
	}
	if err := validateCreationIdentity(observation.Request,
		workflowExecution); err != nil {
		return err
	}
	snapshot, err := buildDefinitionSnapshot(
		observation.Request, workflowExecution)
	if err != nil {
		return fmt.Errorf("build workflow definition snapshot: %w",
			err)
	}
	workflowRecord, err := buildCreatedWorkflowRecord(observation.Request,
		workflowExecution, snapshot.ID())
	if err != nil {
		return fmt.Errorf("build created workflow record: %w",
			err)
	}
	timeline, err := buildWorkflowCreationTimeline(observation.Request,
		workflowRecord)
	if err != nil {
		return fmt.Errorf("build workflow creation timeline: %w", err)
	}
	command, err := repository.NewCreateExecutionCommand(repository.CreateExecutionCommandParams{Snapshot: snapshot,
		WorkflowExecution: workflowRecord, Timeline: timeline},
	)
	if err != nil {
		return fmt.Errorf(
			"build create execution command: %w", err)
	}
	persistedWorkflow, err := advanceWorkflowConcurrency(
		workflowRecord, len(timeline), 0,
	)
	if err != nil {
		return fmt.Errorf(
			"advance workflow creation state: %w", err)
	}
	state, err := recorder.reserveState(
		workflowExecution.ID())
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := recorder.store.CreateExecution(ctx, command); err != nil {
		recorder.removeStateIfSame(workflowExecution.ID(),
			state)
		return fmt.Errorf("persist workflow creation: %w", err)
	}
	state.workflow = persistedWorkflow
	state.nodes = make(map[string]repository.NodeExecutionRecord)
	state.terminalOutputs = make(map[string]payloadSummary)
	state.initialized = true
	return nil
}

func (recorder *Recorder) RecordWorkflowTransition(ctx context.Context, observation engine.WorkflowTransitionObservation,
) error {
	if err := recorder.validateContextAndStore(ctx); err != nil {
		return err
	}
	state, err := recorder.stateFor(
		observation.After.ID())
	if err != nil {
		return err
	}
	state.mu.Lock()
	if !state.initialized {
		state.mu.Unlock()
		return fmt.Errorf("workflow persistence state is not initialized")
	}
	if err := validateWorkflowTransitionObservation(state.workflow, observation); err != nil {
		state.mu.Unlock()
		return err
	}
	timeline, eventID, err := buildWorkflowTransitionTimeline(
		observation, state.workflow.NextSequenceNumber())
	if err != nil {
		state.mu.Unlock()
		return fmt.Errorf(
			"build workflow transition timeline: %w", err)
	}
	updatedWorkflow, err := buildTransitionedWorkflowRecord(
		state.workflow, observation, state.terminalOutputs,
		len(timeline))
	if err != nil {
		state.mu.Unlock()
		return fmt.Errorf("build transitioned workflow record: %w",
			err)
	}
	executionErrors, err := buildWorkflowTransitionErrors(observation,
		eventID)
	if err != nil {
		state.mu.Unlock()
		return fmt.Errorf("build workflow transition errors: %w",
			err)
	}
	command, err := repository.NewWorkflowTransitionCommand(repository.WorkflowTransitionCommandParams{
		CompanyID: observation.After.CompanyID(), WorkflowExecutionID: observation.After.ID(), ExpectedStatus: state.workflow.Status(),
		ExpectedLockVersion: state.workflow.LockVersion(), ExpectedNextSequenceNumber: state.workflow.NextSequenceNumber(), WorkflowExecution: updatedWorkflow,
		Timeline: timeline, Errors: executionErrors},
	)
	if err != nil {
		state.mu.Unlock()
		return fmt.Errorf("build workflow transition command: %w", err)
	}
	if err := recorder.store.ApplyWorkflowTransition(ctx, command); err != nil {
		state.mu.Unlock()
		return fmt.Errorf(
			"persist workflow transition: %w", err)
	}
	state.workflow = updatedWorkflow
	terminal := updatedWorkflow.Status().IsTerminal()
	state.mu.Unlock()
	if terminal {
		recorder.removeStateIfSame(updatedWorkflow.ID(),
			state)
	}
	return nil
}

func (recorder *Recorder) RecordNodeExecutionsCreation(ctx context.Context,
	observation engine.NodeExecutionsCreationObservation) error {
	if err := recorder.validateContextAndStore(ctx); err != nil {
		return err
	}
	state, err := recorder.stateFor(observation.WorkflowExecution.ID())
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.initialized {
		return fmt.Errorf(
			"workflow persistence state is not initialized")
	}
	if state.workflow.Status() != execution.WorkflowExecutionStatusValidating {
		return fmt.Errorf("node executions can only be created while workflow is VALIDATING")
	}
	if observation.WorkflowExecution.Status() !=
		execution.WorkflowExecutionStatusValidating {
		return fmt.Errorf("node creation observation must contain VALIDATING workflow status")
	}
	if err := validateWorkflowObservationIdentity(state.workflow, observation.Request,
		observation.WorkflowExecution); err != nil {
		return err
	}
	if len(state.nodes) != 0 {
		return fmt.Errorf("node executions have already been created for workflow %s", observation.WorkflowExecution.ID())
	}
	nodeRecords, timeline, err := buildNodeCreationRecordsAndTimeline(observation, state.workflow.NextSequenceNumber())
	if err != nil {
		return err
	}
	command, err := repository.NewCreateNodeExecutionsCommand(
		repository.CreateNodeExecutionsCommandParams{CompanyID: observation.WorkflowExecution.CompanyID(), WorkflowExecutionID: observation.WorkflowExecution.ID(),
			ExpectedWorkflowStatus: state.workflow.Status(), ExpectedWorkflowLockVersion: state.workflow.LockVersion(), ExpectedNextSequenceNumber: state.workflow.NextSequenceNumber(),
			NodeExecutions: nodeRecords, Timeline: timeline},
	)
	if err != nil {
		return fmt.Errorf(
			"build create node executions command: %w", err)
	}
	advancedWorkflow, err := advanceWorkflowConcurrency(
		state.workflow, len(timeline), 1,
	)
	if err != nil {
		return fmt.Errorf(
			"advance workflow after node creation: %w", err)
	}
	if err := recorder.store.CreateNodeExecutions(
		ctx, command); err != nil {
		return fmt.Errorf("persist node execution creation: %w", err)
	}
	nodes := make(map[string]repository.NodeExecutionRecord, len(nodeRecords))
	for _, record := range nodeRecords {
		nodes[record.ID().String()] = record
	}
	state.workflow = advancedWorkflow
	state.nodes = nodes
	return nil
}

func (recorder *Recorder) RecordNodeTransition(ctx context.Context, observation engine.NodeTransitionObservation,
) error {
	if err := recorder.validateContextAndStore(ctx); err != nil {
		return err
	}
	state, err := recorder.stateFor(
		observation.WorkflowExecution.ID())
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.initialized {
		return fmt.Errorf("workflow persistence state is not initialized")
	}
	if state.workflow.Status() != execution.WorkflowExecutionStatusRunning {
		return fmt.Errorf(
			"node transitions require a RUNNING workflow")
	}
	nodeExecutionKey := observation.After.ID().String()
	currentNode, exists := state.nodes[nodeExecutionKey]
	if !exists {
		return fmt.Errorf(
			"node persistence state %s was not found", observation.After.ID())
	}
	if err := validateNodeTransitionObservation(
		state.workflow, currentNode, observation,
	); err != nil {
		return err
	}
	timeline, eventID, err := buildNodeTransitionTimeline(observation,
		state.workflow.NextSequenceNumber())
	if err != nil {
		return fmt.Errorf("build node transition timeline: %w", err)
	}
	updatedNode, terminalOutput, hasTerminalOutput, err := buildTransitionedNodeRecord(currentNode,
		observation)
	if err != nil {
		return fmt.Errorf("build transitioned node record: %w", err)
	}
	attemptMutation, hasAttemptMutation, err := buildNodeAttemptMutation(
		state.workflow.Mode(), observation, currentNode, updatedNode,
	)
	if err != nil {
		return fmt.Errorf("build node attempt mutation: %w", err)
	}
	executionErrors, err := buildNodeTransitionErrors(observation, eventID)
	if err != nil {
		return fmt.Errorf(
			"build node transition errors: %w", err)
	}
	command, err := repository.NewNodeTransitionCommand(
		repository.NodeTransitionCommandParams{CompanyID: observation.WorkflowExecution.CompanyID(), WorkflowExecutionID: observation.WorkflowExecution.ID(),
			NodeExecutionID: observation.After.ID(), ExpectedWorkflowStatus: state.workflow.Status(), ExpectedWorkflowLockVersion: state.workflow.LockVersion(),
			ExpectedNextSequenceNumber: state.workflow.NextSequenceNumber(), ExpectedNodeStatus: currentNode.Status(), ExpectedNodeLockVersion: currentNode.LockVersion(),
			ExpectedNodeAttempt: currentNode.Attempt(),
			ExecutionMode:       state.workflow.Mode(),
			NodeExecution:       updatedNode,
			AttemptMutation:     attemptMutation,
			HasAttemptMutation:  hasAttemptMutation,
			Timeline:            timeline,
			Errors:              executionErrors,
		})
	if err != nil {
		return fmt.Errorf("build node transition command: %w", err)
	}
	advancedWorkflow, err := advanceWorkflowConcurrency(state.workflow, len(timeline),
		1)
	if err != nil {
		return fmt.Errorf("advance workflow after node transition: %w", err)
	}
	if err := recorder.store.ApplyNodeTransition(ctx, command); err != nil {
		return fmt.Errorf("persist node transition: %w",
			err)
	}
	state.workflow = advancedWorkflow
	state.nodes[updatedNode.ID().String()] = updatedNode
	if hasTerminalOutput {
		nodeIDKey := updatedNode.NodeID().String()
		state.terminalOutputs[nodeIDKey] = terminalOutput
	}
	return nil
}

func (recorder *Recorder) validateContextAndStore(ctx context.Context) error {
	if recorder == nil || recorder.store == nil {
		return fmt.Errorf(
			"persistent lifecycle recorder must be valid")
	}
	if ctx == nil {
		return fmt.Errorf(
			"lifecycle recorder context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (recorder *Recorder) reserveState(
	workflowExecutionID execution.WorkflowExecutionID) (*executionPersistenceState, error) {
	key := workflowExecutionID.String()
	if key == "" {
		return nil, fmt.Errorf("workflow execution ID must not be empty")
	}
	state := &executionPersistenceState{}
	recorder.statesMu.Lock()
	defer recorder.statesMu.Unlock()
	if _, exists := recorder.states[key]; exists {
		return nil, fmt.Errorf("workflow persistence state %s already exists", workflowExecutionID)
	}
	recorder.states[key] = state
	return state, nil
}

func (recorder *Recorder) stateFor(
	workflowExecutionID execution.WorkflowExecutionID) (*executionPersistenceState, error) {
	recorder.statesMu.Lock()
	defer recorder.statesMu.Unlock()
	key := workflowExecutionID.String()
	state, exists := recorder.states[key]
	if !exists || state == nil {
		return nil, fmt.Errorf(
			"workflow persistence state %s was not found", workflowExecutionID)
	}
	return state, nil
}

func (recorder *Recorder) removeStateIfSame(
	workflowExecutionID execution.WorkflowExecutionID, expected *executionPersistenceState) {
	recorder.statesMu.Lock()
	defer recorder.statesMu.Unlock()
	key := workflowExecutionID.String()
	current, exists := recorder.states[key]
	if exists && current == expected {
		delete(recorder.states, key)
	}
}
