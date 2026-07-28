package execution

import (
	errors "errors"
	workflowdomain "miletos-go/internal/features/workflow"
	testing "testing"
	time "time"
)

func TestValidationErrorFormatsFieldAndReason(t *testing.T) {
	err := &ValidationError{
		Field:  "workflowExecutionID",
		Reason: "must not be empty",
	}

	expected := "workflowExecutionID: must not be empty"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestInvalidModeErrorFormatsValue(t *testing.T) {
	err := &InvalidModeError{
		Value: "BACKGROUND",
	}

	expected := `invalid execution mode "BACKGROUND": must be one of SYNC, ASYNC`

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestTransitionErrorFormatsStatuses(t *testing.T) {
	err := &TransitionError{
		Entity:        "workflow execution",
		CurrentStatus: "SUCCEEDED",
		TargetStatus:  "RUNNING",
	}

	expected := "workflow execution cannot transition from SUCCEEDED to RUNNING"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestTimestampErrorFormatsFieldAndReason(t *testing.T) {
	err := &TimestampError{
		Field:  "startedAt",
		Reason: "must not be before createdAt",
	}

	expected := "startedAt: must not be before createdAt"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestExecutionIdentifierConstructorsNormalizeValues(t *testing.T) {
	tests := []struct {
		name      string
		construct func(string) (string, error)
	}{
		{
			name: "workflow execution ID",
			construct: func(value string) (string, error) {
				identifier, err := NewWorkflowExecutionID(value)
				return identifier.String(), err
			},
		},
		{
			name: "node execution ID",
			construct: func(value string) (string, error) {
				identifier, err := NewNodeExecutionID(value)
				return identifier.String(), err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.construct(
				"  execution-1  ",
			)
			if err != nil {
				t.Fatalf(
					"constructor returned an unexpected error: %v",
					err,
				)
			}

			if actual != "execution-1" {
				t.Fatalf(
					"identifier = %q, want %q",
					actual,
					"execution-1",
				)
			}
		})
	}
}

func TestExecutionIdentifierConstructorsRejectBlankValues(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		construct func(string) error
	}{
		{
			name:  "workflow execution ID",
			field: "workflowExecutionID",
			construct: func(value string) error {
				_, err := NewWorkflowExecutionID(value)
				return err
			},
		},
		{
			name:  "node execution ID",
			field: "nodeExecutionID",
			construct: func(value string) error {
				_, err := NewNodeExecutionID(value)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.construct("   ")
			if err == nil {
				t.Fatal(
					"constructor returned nil error for a blank identifier",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}

			if validationError.Reason != "must not be empty" {
				t.Fatalf(
					"validation reason = %q, want %q",
					validationError.Reason,
					"must not be empty",
				)
			}
		})
	}
}

func TestParseExecutionModeAcceptsSupportedModes(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected ExecutionMode
	}{
		{
			name:     "sync uppercase",
			value:    "SYNC",
			expected: ExecutionModeSync,
		},
		{
			name:     "sync lowercase and whitespace",
			value:    "  sync  ",
			expected: ExecutionModeSync,
		},
		{
			name:     "async uppercase",
			value:    "ASYNC",
			expected: ExecutionModeAsync,
		},
		{
			name:     "async mixed case and whitespace",
			value:    "  AsYnC  ",
			expected: ExecutionModeAsync,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := ParseExecutionMode(test.value)
			if err != nil {
				t.Fatalf(
					"ParseExecutionMode() returned an unexpected error: %v",
					err,
				)
			}

			if actual != test.expected {
				t.Fatalf(
					"mode = %q, want %q",
					actual,
					test.expected,
				)
			}

			if actual.String() != test.expected.String() {
				t.Fatalf(
					"String() = %q, want %q",
					actual.String(),
					test.expected.String(),
				)
			}

			if !actual.IsValid() {
				t.Fatalf(
					"IsValid() = false for supported mode %q",
					actual,
				)
			}
		})
	}
}

func TestParseExecutionModeRejectsUnsupportedMode(t *testing.T) {
	const value = " background "

	_, err := ParseExecutionMode(value)
	if err == nil {
		t.Fatal(
			"ParseExecutionMode() returned nil error for an unsupported mode",
		)
	}

	var invalidModeError *InvalidModeError
	if !errors.As(err, &invalidModeError) {
		t.Fatalf(
			"error type = %T, want *InvalidModeError",
			err,
		)
	}

	if invalidModeError.Value != value {
		t.Fatalf(
			"invalid mode value = %q, want %q",
			invalidModeError.Value,
			value,
		)
	}
}

func TestExecutionModeIsValidRejectsUnknownAndZeroValues(t *testing.T) {
	tests := map[string]ExecutionMode{
		"zero value": "",
		"unknown":    ExecutionMode("BACKGROUND"),
	}

	for name, mode := range tests {
		t.Run(name, func(t *testing.T) {
			if mode.IsValid() {
				t.Fatalf(
					"IsValid() = true for unsupported mode %q",
					mode,
				)
			}
		})
	}
}

func TestNewNodeExecutionStoresNormalizedValuesAndInitialState(
	t *testing.T,
) {
	createdAt := nodeExecutionTestTime(11, 0)

	execution, err := NewNodeExecution(
		NodeExecutionID(" node-execution-1 "),
		WorkflowExecutionID(" workflow-execution-1 "),
		workflowdomain.NodeID(" node-1 "),
		createdAt,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecution() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.ID().String(); actual != "node-execution-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"node-execution-1",
		)
	}

	if actual := execution.WorkflowExecutionID().String(); actual != "workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q, want %q",
			actual,
			"workflow-execution-1",
		)
	}

	if actual := execution.NodeID().String(); actual != "node-1" {
		t.Fatalf(
			"NodeID() = %q, want %q",
			actual,
			"node-1",
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusPending {
		t.Fatalf(
			"Status() = %q, want %q",
			actual,
			NodeExecutionStatusPending,
		)
	}

	if !execution.CreatedAt().Equal(createdAt) {
		t.Fatalf(
			"CreatedAt() = %v, want %v",
			execution.CreatedAt(),
			createdAt,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists for a newly created node execution",
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists for a newly created node execution",
		)
	}

	if execution.IsTerminal() {
		t.Fatal(
			"IsTerminal() = true for PENDING node execution",
		)
	}
}

func TestNewNodeExecutionRejectsInvalidExecutionIdentifiers(
	t *testing.T,
) {
	createdAt := nodeExecutionTestTime(11, 0)

	tests := []struct {
		name                string
		id                  NodeExecutionID
		workflowExecutionID WorkflowExecutionID
		field               string
	}{
		{
			name:                "blank node execution ID",
			id:                  NodeExecutionID(" "),
			workflowExecutionID: WorkflowExecutionID("workflow-execution-1"),
			field:               "nodeExecutionID",
		},
		{
			name:                "blank workflow execution ID",
			id:                  NodeExecutionID("node-execution-1"),
			workflowExecutionID: WorkflowExecutionID(" "),
			field:               "workflowExecutionID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewNodeExecution(
				test.id,
				test.workflowExecutionID,
				workflowdomain.NodeID("node-1"),
				createdAt,
			)
			if err == nil {
				t.Fatal(
					"NewNodeExecution() returned nil error for an invalid execution identifier",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewNodeExecutionRejectsBlankNodeID(t *testing.T) {
	_, err := NewNodeExecution(
		NodeExecutionID("node-execution-1"),
		WorkflowExecutionID("workflow-execution-1"),
		workflowdomain.NodeID(" "),
		nodeExecutionTestTime(11, 0),
	)
	if err == nil {
		t.Fatal(
			"NewNodeExecution() returned nil error for a blank node ID",
		)
	}

	var validationError *workflowdomain.ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *workflow.ValidationError",
			err,
		)
	}

	if validationError.Field != "nodeID" {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			"nodeID",
		)
	}
}

func TestNewNodeExecutionRejectsZeroCreatedAt(t *testing.T) {
	_, err := NewNodeExecution(
		NodeExecutionID("node-execution-1"),
		WorkflowExecutionID("workflow-execution-1"),
		workflowdomain.NodeID("node-1"),
		time.Time{},
	)
	if err == nil {
		t.Fatal(
			"NewNodeExecution() returned nil error for zero createdAt",
		)
	}

	var timestampError *TimestampError
	if !errors.As(err, &timestampError) {
		t.Fatalf(
			"error type = %T, want *TimestampError",
			err,
		)
	}

	if timestampError.Field != "createdAt" {
		t.Fatalf(
			"timestamp field = %q, want %q",
			timestampError.Field,
			"createdAt",
		)
	}
}

func TestNodeExecutionCompletesSyncLifecycle(t *testing.T) {
	createdAt := nodeExecutionTestTime(11, 0)
	readyAt := nodeExecutionTestTime(11, 1)
	startedAt := nodeExecutionTestTime(11, 2)
	finishedAt := nodeExecutionTestTime(11, 3)

	execution := newNodeExecutionForTest(
		t,
		createdAt,
	)

	if err := execution.MarkReady(readyAt); err != nil {
		t.Fatalf(
			"MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusReady {
		t.Fatalf(
			"status after ready = %q, want %q",
			actual,
			NodeExecutionStatusReady,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists before node entered RUNNING",
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	actualStartedAt, exists := execution.StartedAt()
	if !exists {
		t.Fatal(
			"StartedAt() does not exist after node entered RUNNING",
		)
	}

	if !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, want %v",
			actualStartedAt,
			startedAt,
		)
	}

	if err := execution.Succeed(finishedAt); err != nil {
		t.Fatalf(
			"Succeed() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusSucceeded {
		t.Fatalf(
			"final status = %q, want %q",
			actual,
			NodeExecutionStatusSucceeded,
		)
	}

	actualFinishedAt, exists := execution.FinishedAt()
	if !exists {
		t.Fatal(
			"FinishedAt() does not exist after terminal transition",
		)
	}

	if !actualFinishedAt.Equal(finishedAt) {
		t.Fatalf(
			"FinishedAt() = %v, want %v",
			actualFinishedAt,
			finishedAt,
		)
	}

	if !execution.IsTerminal() {
		t.Fatal(
			"IsTerminal() = false after SUCCEEDED transition",
		)
	}
}

func TestNodeExecutionCompletesAsyncLifecycle(t *testing.T) {
	createdAt := nodeExecutionTestTime(11, 0)
	readyAt := nodeExecutionTestTime(11, 1)
	queuedAt := nodeExecutionTestTime(11, 2)
	startedAt := nodeExecutionTestTime(11, 3)
	finishedAt := nodeExecutionTestTime(11, 4)

	execution := newNodeExecutionForTest(
		t,
		createdAt,
	)

	if err := execution.MarkReady(readyAt); err != nil {
		t.Fatalf(
			"MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Queue(queuedAt); err != nil {
		t.Fatalf(
			"Queue() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusQueued {
		t.Fatalf(
			"status after queue = %q, want %q",
			actual,
			NodeExecutionStatusQueued,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists while node is QUEUED",
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Succeed(finishedAt); err != nil {
		t.Fatalf(
			"Succeed() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusSucceeded {
		t.Fatalf(
			"final status = %q, want %q",
			actual,
			NodeExecutionStatusSucceeded,
		)
	}

	actualStartedAt, startedExists := execution.StartedAt()
	if !startedExists || !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, exists = %t, want %v",
			actualStartedAt,
			startedExists,
			startedAt,
		)
	}

	actualFinishedAt, finishedExists := execution.FinishedAt()
	if !finishedExists || !actualFinishedAt.Equal(finishedAt) {
		t.Fatalf(
			"FinishedAt() = %v, exists = %t, want %v",
			actualFinishedAt,
			finishedExists,
			finishedAt,
		)
	}
}

func TestNodeExecutionTerminalPathsSetFinishedAt(t *testing.T) {
	createdAt := nodeExecutionTestTime(11, 0)
	firstAt := nodeExecutionTestTime(11, 1)
	secondAt := nodeExecutionTestTime(11, 2)
	terminalAt := nodeExecutionTestTime(11, 3)

	tests := []struct {
		name          string
		prepare       func(*NodeExecution) error
		complete      func(*NodeExecution) error
		expected      NodeExecutionStatus
		startedExists bool
	}{
		{
			name:    "pending to skipped",
			prepare: func(*NodeExecution) error { return nil },
			complete: func(execution *NodeExecution) error {
				return execution.Skip(terminalAt)
			},
			expected:      NodeExecutionStatusSkipped,
			startedExists: false,
		},
		{
			name:    "pending to cancelled",
			prepare: func(*NodeExecution) error { return nil },
			complete: func(execution *NodeExecution) error {
				return execution.Cancel(terminalAt)
			},
			expected:      NodeExecutionStatusCancelled,
			startedExists: false,
		},
		{
			name: "ready to timed out",
			prepare: func(execution *NodeExecution) error {
				return execution.MarkReady(firstAt)
			},
			complete: func(execution *NodeExecution) error {
				return execution.Timeout(terminalAt)
			},
			expected:      NodeExecutionStatusTimedOut,
			startedExists: false,
		},
		{
			name: "queued to failed",
			prepare: func(execution *NodeExecution) error {
				if err := execution.MarkReady(firstAt); err != nil {
					return err
				}

				return execution.Queue(secondAt)
			},
			complete: func(execution *NodeExecution) error {
				return execution.Fail(terminalAt)
			},
			expected:      NodeExecutionStatusFailed,
			startedExists: false,
		},
		{
			name: "running to cancelled",
			prepare: func(execution *NodeExecution) error {
				if err := execution.MarkReady(firstAt); err != nil {
					return err
				}

				return execution.Start(secondAt)
			},
			complete: func(execution *NodeExecution) error {
				return execution.Cancel(terminalAt)
			},
			expected:      NodeExecutionStatusCancelled,
			startedExists: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution := newNodeExecutionForTest(
				t,
				createdAt,
			)

			if err := test.prepare(&execution); err != nil {
				t.Fatalf(
					"prepare transition returned an unexpected error: %v",
					err,
				)
			}

			if err := test.complete(&execution); err != nil {
				t.Fatalf(
					"terminal transition returned an unexpected error: %v",
					err,
				)
			}

			if actual := execution.Status(); actual != test.expected {
				t.Fatalf(
					"Status() = %q, want %q",
					actual,
					test.expected,
				)
			}

			if !execution.IsTerminal() {
				t.Fatal(
					"IsTerminal() = false after terminal transition",
				)
			}

			_, startedExists := execution.StartedAt()
			if startedExists != test.startedExists {
				t.Fatalf(
					"StartedAt() exists = %t, want %t",
					startedExists,
					test.startedExists,
				)
			}

			actualFinishedAt, finishedExists := execution.FinishedAt()
			if !finishedExists {
				t.Fatal(
					"FinishedAt() does not exist after terminal transition",
				)
			}

			if !actualFinishedAt.Equal(terminalAt) {
				t.Fatalf(
					"FinishedAt() = %v, want %v",
					actualFinishedAt,
					terminalAt,
				)
			}
		})
	}
}

func TestNodeExecutionTransitionMatrix(t *testing.T) {
	statuses := []NodeExecutionStatus{
		NodeExecutionStatusPending,
		NodeExecutionStatusReady,
		NodeExecutionStatusQueued,
		NodeExecutionStatusRunning,
		NodeExecutionStatusSucceeded,
		NodeExecutionStatusFailed,
		NodeExecutionStatusSkipped,
		NodeExecutionStatusCancelled,
		NodeExecutionStatusTimedOut,
	}

	type transition struct {
		current NodeExecutionStatus
		target  NodeExecutionStatus
	}

	allowed := map[transition]struct{}{
		{
			current: NodeExecutionStatusPending,
			target:  NodeExecutionStatusReady,
		}: {},
		{
			current: NodeExecutionStatusPending,
			target:  NodeExecutionStatusSkipped,
		}: {},
		{
			current: NodeExecutionStatusPending,
			target:  NodeExecutionStatusCancelled,
		}: {},
		{
			current: NodeExecutionStatusReady,
			target:  NodeExecutionStatusQueued,
		}: {},
		{
			current: NodeExecutionStatusReady,
			target:  NodeExecutionStatusRunning,
		}: {},
		{
			current: NodeExecutionStatusReady,
			target:  NodeExecutionStatusSkipped,
		}: {},
		{
			current: NodeExecutionStatusReady,
			target:  NodeExecutionStatusCancelled,
		}: {},
		{
			current: NodeExecutionStatusReady,
			target:  NodeExecutionStatusTimedOut,
		}: {},
		{
			current: NodeExecutionStatusQueued,
			target:  NodeExecutionStatusRunning,
		}: {},
		{
			current: NodeExecutionStatusQueued,
			target:  NodeExecutionStatusFailed,
		}: {},
		{
			current: NodeExecutionStatusQueued,
			target:  NodeExecutionStatusCancelled,
		}: {},
		{
			current: NodeExecutionStatusQueued,
			target:  NodeExecutionStatusTimedOut,
		}: {},
		{
			current: NodeExecutionStatusRunning,
			target:  NodeExecutionStatusSucceeded,
		}: {},
		{
			current: NodeExecutionStatusRunning,
			target:  NodeExecutionStatusFailed,
		}: {},
		{
			current: NodeExecutionStatusRunning,
			target:  NodeExecutionStatusCancelled,
		}: {},
		{
			current: NodeExecutionStatusRunning,
			target:  NodeExecutionStatusTimedOut,
		}: {},
	}

	for _, current := range statuses {
		for _, target := range statuses {
			name := current.String() + "_to_" + target.String()

			t.Run(name, func(t *testing.T) {
				_, expected := allowed[transition{
					current: current,
					target:  target,
				}]

				actual := isNodeTransitionAllowed(
					current,
					target,
				)

				if actual != expected {
					t.Fatalf(
						"isNodeTransitionAllowed(%q, %q) = %t, want %t",
						current,
						target,
						actual,
						expected,
					)
				}
			})
		}
	}
}

func TestNodeExecutionInvalidTransitionPreservesState(t *testing.T) {
	createdAt := nodeExecutionTestTime(11, 0)
	readyAt := nodeExecutionTestTime(11, 1)
	startedAt := nodeExecutionTestTime(11, 2)
	invalidAt := nodeExecutionTestTime(11, 3)

	execution := newNodeExecutionForTest(
		t,
		createdAt,
	)

	if err := execution.MarkReady(readyAt); err != nil {
		t.Fatalf(
			"MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	err := execution.Queue(invalidAt)
	if err == nil {
		t.Fatal(
			"Queue() returned nil error for RUNNING to QUEUED transition",
		)
	}

	var transitionError *TransitionError
	if !errors.As(err, &transitionError) {
		t.Fatalf(
			"error type = %T, want *TransitionError",
			err,
		)
	}

	if transitionError.CurrentStatus != "RUNNING" {
		t.Fatalf(
			"current status = %q, want %q",
			transitionError.CurrentStatus,
			"RUNNING",
		)
	}

	if transitionError.TargetStatus != "QUEUED" {
		t.Fatalf(
			"target status = %q, want %q",
			transitionError.TargetStatus,
			"QUEUED",
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusRunning {
		t.Fatalf(
			"status after failed transition = %q, want %q",
			actual,
			NodeExecutionStatusRunning,
		)
	}

	actualStartedAt, exists := execution.StartedAt()
	if !exists || !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, exists = %t, want %v",
			actualStartedAt,
			exists,
			startedAt,
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists after failed non-terminal transition",
		)
	}
}

func TestNodeExecutionTerminalStateRejectsFurtherTransitions(t *testing.T) {
	createdAt := nodeExecutionTestTime(11, 0)
	readyAt := nodeExecutionTestTime(11, 1)
	startedAt := nodeExecutionTestTime(11, 2)
	finishedAt := nodeExecutionTestTime(11, 3)
	invalidAt := nodeExecutionTestTime(11, 4)

	execution := newNodeExecutionForTest(
		t,
		createdAt,
	)

	if err := execution.MarkReady(readyAt); err != nil {
		t.Fatalf(
			"MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Succeed(finishedAt); err != nil {
		t.Fatalf(
			"Succeed() returned an unexpected error: %v",
			err,
		)
	}

	err := execution.Start(invalidAt)
	if err == nil {
		t.Fatal(
			"Start() returned nil error for a terminal node execution",
		)
	}

	var transitionError *TransitionError
	if !errors.As(err, &transitionError) {
		t.Fatalf(
			"error type = %T, want *TransitionError",
			err,
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusSucceeded {
		t.Fatalf(
			"status after rejected terminal transition = %q, want %q",
			actual,
			NodeExecutionStatusSucceeded,
		)
	}

	actualFinishedAt, exists := execution.FinishedAt()
	if !exists || !actualFinishedAt.Equal(finishedAt) {
		t.Fatalf(
			"FinishedAt() = %v, exists = %t, want %v",
			actualFinishedAt,
			exists,
			finishedAt,
		)
	}
}

func TestNodeExecutionInvalidTimestampPreservesState(t *testing.T) {
	createdAt := nodeExecutionTestTime(11, 0)
	beforeCreatedAt := nodeExecutionTestTime(10, 59)

	execution := newNodeExecutionForTest(
		t,
		createdAt,
	)

	err := execution.MarkReady(beforeCreatedAt)
	if err == nil {
		t.Fatal(
			"MarkReady() returned nil error for timestamp before createdAt",
		)
	}

	var timestampError *TimestampError
	if !errors.As(err, &timestampError) {
		t.Fatalf(
			"error type = %T, want *TimestampError",
			err,
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusPending {
		t.Fatalf(
			"status after failed timestamp validation = %q, want %q",
			actual,
			NodeExecutionStatusPending,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists after rejected timestamp",
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists after rejected timestamp",
		)
	}
}

func TestNodeExecutionRejectsZeroTransitionTimestamp(t *testing.T) {
	execution := newNodeExecutionForTest(
		t,
		nodeExecutionTestTime(11, 0),
	)

	err := execution.MarkReady(time.Time{})
	if err == nil {
		t.Fatal(
			"MarkReady() returned nil error for zero transition timestamp",
		)
	}

	var timestampError *TimestampError
	if !errors.As(err, &timestampError) {
		t.Fatalf(
			"error type = %T, want *TimestampError",
			err,
		)
	}

	if timestampError.Field != "transitionAt" {
		t.Fatalf(
			"timestamp field = %q, want %q",
			timestampError.Field,
			"transitionAt",
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusPending {
		t.Fatalf(
			"status after zero timestamp = %q, want %q",
			actual,
			NodeExecutionStatusPending,
		)
	}
}

func TestNodeExecutionTerminalTimestampCannotPrecedeStartedAt(
	t *testing.T,
) {
	createdAt := nodeExecutionTestTime(11, 0)
	readyAt := nodeExecutionTestTime(11, 1)
	startedAt := nodeExecutionTestTime(11, 3)
	invalidFinishedAt := nodeExecutionTestTime(11, 2)

	execution := newNodeExecutionForTest(
		t,
		createdAt,
	)

	if err := execution.MarkReady(readyAt); err != nil {
		t.Fatalf(
			"MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	err := execution.Succeed(invalidFinishedAt)
	if err == nil {
		t.Fatal(
			"Succeed() returned nil error for finishedAt before startedAt",
		)
	}

	var timestampError *TimestampError
	if !errors.As(err, &timestampError) {
		t.Fatalf(
			"error type = %T, want *TimestampError",
			err,
		)
	}

	if actual := execution.Status(); actual != NodeExecutionStatusRunning {
		t.Fatalf(
			"status after rejected terminal timestamp = %q, want %q",
			actual,
			NodeExecutionStatusRunning,
		)
	}

	actualStartedAt, exists := execution.StartedAt()
	if !exists || !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, exists = %t, want %v",
			actualStartedAt,
			exists,
			startedAt,
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists after rejected terminal timestamp",
		)
	}
}

func TestNilNodeExecutionRejectsTransition(t *testing.T) {
	var execution *NodeExecution

	err := execution.MarkReady(
		nodeExecutionTestTime(11, 0),
	)
	if err == nil {
		t.Fatal(
			"MarkReady() returned nil error for nil receiver",
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != "nodeExecution" {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			"nodeExecution",
		)
	}
}

func newNodeExecutionForTest(
	t *testing.T,
	createdAt time.Time,
) NodeExecution {
	t.Helper()

	execution, err := NewNodeExecution(
		NodeExecutionID("node-execution-1"),
		WorkflowExecutionID("workflow-execution-1"),
		workflowdomain.NodeID("node-1"),
		createdAt,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecution() returned an unexpected error: %v",
			err,
		)
	}

	return execution
}

func nodeExecutionTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		16,
		hour,
		minute,
		0,
		0,
		time.UTC,
	)
}

func TestNodeExecutionStatusCanTransitionTo(
	t *testing.T,
) {
	tests := []struct {
		name     string
		current  NodeExecutionStatus
		target   NodeExecutionStatus
		expected bool
	}{
		{
			name:     "pending to ready",
			current:  NodeExecutionStatusPending,
			target:   NodeExecutionStatusReady,
			expected: true,
		},
		{
			name:     "ready to queued",
			current:  NodeExecutionStatusReady,
			target:   NodeExecutionStatusQueued,
			expected: true,
		},
		{
			name:     "ready to running",
			current:  NodeExecutionStatusReady,
			target:   NodeExecutionStatusRunning,
			expected: true,
		},
		{
			name:     "queued to failed",
			current:  NodeExecutionStatusQueued,
			target:   NodeExecutionStatusFailed,
			expected: true,
		},
		{
			name:     "running to succeeded",
			current:  NodeExecutionStatusRunning,
			target:   NodeExecutionStatusSucceeded,
			expected: true,
		},
		{
			name:     "running to failed",
			current:  NodeExecutionStatusRunning,
			target:   NodeExecutionStatusFailed,
			expected: true,
		},
		{
			name:     "pending to succeeded",
			current:  NodeExecutionStatusPending,
			target:   NodeExecutionStatusSucceeded,
			expected: false,
		},
		{
			name:     "terminal to running",
			current:  NodeExecutionStatusFailed,
			target:   NodeExecutionStatusRunning,
			expected: false,
		},
		{
			name:     "invalid current",
			current:  NodeExecutionStatus("UNKNOWN"),
			target:   NodeExecutionStatusRunning,
			expected: false,
		},
		{
			name:     "invalid target",
			current:  NodeExecutionStatusRunning,
			target:   NodeExecutionStatus("UNKNOWN"),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual :=
				test.current.CanTransitionTo(
					test.target,
				)

			if actual != test.expected {
				t.Fatalf(
					"CanTransitionTo() = %t, want %t",
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestWorkflowExecutionStatusesAreValid(t *testing.T) {
	statuses := []WorkflowExecutionStatus{
		WorkflowExecutionStatusCreated,
		WorkflowExecutionStatusValidating,
		WorkflowExecutionStatusRejected,
		WorkflowExecutionStatusQueued,
		WorkflowExecutionStatusRunning,
		WorkflowExecutionStatusSucceeded,
		WorkflowExecutionStatusFailed,
		WorkflowExecutionStatusCancelled,
		WorkflowExecutionStatusTimedOut,
	}

	for _, status := range statuses {
		t.Run(status.String(), func(t *testing.T) {
			if !status.IsValid() {
				t.Fatalf(
					"IsValid() = false for workflow status %q",
					status,
				)
			}

			if status.String() == "" {
				t.Fatal(
					"String() returned an empty value for a valid workflow status",
				)
			}
		})
	}
}

func TestWorkflowExecutionStatusTerminalClassification(t *testing.T) {
	tests := []struct {
		status   WorkflowExecutionStatus
		terminal bool
	}{
		{
			status:   WorkflowExecutionStatusCreated,
			terminal: false,
		},
		{
			status:   WorkflowExecutionStatusValidating,
			terminal: false,
		},
		{
			status:   WorkflowExecutionStatusQueued,
			terminal: false,
		},
		{
			status:   WorkflowExecutionStatusRunning,
			terminal: false,
		},
		{
			status:   WorkflowExecutionStatusRejected,
			terminal: true,
		},
		{
			status:   WorkflowExecutionStatusSucceeded,
			terminal: true,
		},
		{
			status:   WorkflowExecutionStatusFailed,
			terminal: true,
		},
		{
			status:   WorkflowExecutionStatusCancelled,
			terminal: true,
		},
		{
			status:   WorkflowExecutionStatusTimedOut,
			terminal: true,
		},
	}

	for _, test := range tests {
		t.Run(test.status.String(), func(t *testing.T) {
			if actual := test.status.IsTerminal(); actual != test.terminal {
				t.Fatalf(
					"IsTerminal() = %t, want %t",
					actual,
					test.terminal,
				)
			}
		})
	}
}

func TestWorkflowExecutionStatusRejectsUnknownAndZeroValues(t *testing.T) {
	tests := map[string]WorkflowExecutionStatus{
		"zero value": "",
		"unknown":    WorkflowExecutionStatus("PAUSED"),
	}

	for name, status := range tests {
		t.Run(name, func(t *testing.T) {
			if status.IsValid() {
				t.Fatalf(
					"IsValid() = true for unsupported workflow status %q",
					status,
				)
			}

			if status.IsTerminal() {
				t.Fatalf(
					"IsTerminal() = true for unsupported workflow status %q",
					status,
				)
			}
		})
	}
}

func TestNodeExecutionStatusesAreValid(t *testing.T) {
	statuses := []NodeExecutionStatus{
		NodeExecutionStatusPending,
		NodeExecutionStatusReady,
		NodeExecutionStatusQueued,
		NodeExecutionStatusRunning,
		NodeExecutionStatusSucceeded,
		NodeExecutionStatusFailed,
		NodeExecutionStatusSkipped,
		NodeExecutionStatusCancelled,
		NodeExecutionStatusTimedOut,
	}

	for _, status := range statuses {
		t.Run(status.String(), func(t *testing.T) {
			if !status.IsValid() {
				t.Fatalf(
					"IsValid() = false for node status %q",
					status,
				)
			}

			if status.String() == "" {
				t.Fatal(
					"String() returned an empty value for a valid node status",
				)
			}
		})
	}
}

func TestNodeExecutionStatusTerminalClassification(t *testing.T) {
	tests := []struct {
		status   NodeExecutionStatus
		terminal bool
	}{
		{
			status:   NodeExecutionStatusPending,
			terminal: false,
		},
		{
			status:   NodeExecutionStatusReady,
			terminal: false,
		},
		{
			status:   NodeExecutionStatusQueued,
			terminal: false,
		},
		{
			status:   NodeExecutionStatusRunning,
			terminal: false,
		},
		{
			status:   NodeExecutionStatusSucceeded,
			terminal: true,
		},
		{
			status:   NodeExecutionStatusFailed,
			terminal: true,
		},
		{
			status:   NodeExecutionStatusSkipped,
			terminal: true,
		},
		{
			status:   NodeExecutionStatusCancelled,
			terminal: true,
		},
		{
			status:   NodeExecutionStatusTimedOut,
			terminal: true,
		},
	}

	for _, test := range tests {
		t.Run(test.status.String(), func(t *testing.T) {
			if actual := test.status.IsTerminal(); actual != test.terminal {
				t.Fatalf(
					"IsTerminal() = %t, want %t",
					actual,
					test.terminal,
				)
			}
		})
	}
}

func TestNodeExecutionStatusRejectsUnknownAndZeroValues(t *testing.T) {
	tests := map[string]NodeExecutionStatus{
		"zero value": "",
		"unknown":    NodeExecutionStatus("BLOCKED"),
	}

	for name, status := range tests {
		t.Run(name, func(t *testing.T) {
			if status.IsValid() {
				t.Fatalf(
					"IsValid() = true for unsupported node status %q",
					status,
				)
			}

			if status.IsTerminal() {
				t.Fatalf(
					"IsTerminal() = true for unsupported node status %q",
					status,
				)
			}
		})
	}
}

func TestNewWorkflowExecutionStoresNormalizedValuesAndInitialState(
	t *testing.T,
) {
	createdAt := workflowExecutionTestTime(10, 0)

	execution, err := NewWorkflowExecution(
		WorkflowExecutionID(" execution-1 "),
		workflowdomain.CompanyID(" company-1 "),
		workflowdomain.WorkflowID(" workflow-1 "),
		3,
		ExecutionModeAsync,
		createdAt,
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecution() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.ID().String(); actual != "execution-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"execution-1",
		)
	}

	if actual := execution.CompanyID().String(); actual != "company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := execution.WorkflowID().String(); actual != "workflow-1" {
		t.Fatalf(
			"WorkflowID() = %q, want %q",
			actual,
			"workflow-1",
		)
	}

	if actual := execution.WorkflowRevision(); actual != 3 {
		t.Fatalf(
			"WorkflowRevision() = %d, want %d",
			actual,
			3,
		)
	}

	if actual := execution.Mode(); actual != ExecutionModeAsync {
		t.Fatalf(
			"Mode() = %q, want %q",
			actual,
			ExecutionModeAsync,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusCreated {
		t.Fatalf(
			"Status() = %q, want %q",
			actual,
			WorkflowExecutionStatusCreated,
		)
	}

	if !execution.CreatedAt().Equal(createdAt) {
		t.Fatalf(
			"CreatedAt() = %v, want %v",
			execution.CreatedAt(),
			createdAt,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists for a newly created workflow execution",
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists for a newly created workflow execution",
		)
	}

	if execution.IsTerminal() {
		t.Fatal(
			"IsTerminal() = true for CREATED workflow execution",
		)
	}
}

func TestNewWorkflowExecutionRejectsInvalidExecutionFields(
	t *testing.T,
) {
	createdAt := workflowExecutionTestTime(10, 0)

	tests := []struct {
		name             string
		id               WorkflowExecutionID
		workflowRevision uint64
		field            string
	}{
		{
			name:             "blank workflow execution ID",
			id:               WorkflowExecutionID(" "),
			workflowRevision: 1,
			field:            "workflowExecutionID",
		},
		{
			name:             "zero workflow revision",
			id:               WorkflowExecutionID("execution-1"),
			workflowRevision: 0,
			field:            "workflowRevision",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWorkflowExecution(
				test.id,
				workflowdomain.CompanyID("company-1"),
				workflowdomain.WorkflowID("workflow-1"),
				test.workflowRevision,
				ExecutionModeSync,
				createdAt,
			)
			if err == nil {
				t.Fatal(
					"NewWorkflowExecution() returned nil error for an invalid field",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewWorkflowExecutionRejectsInvalidWorkflowReferences(
	t *testing.T,
) {
	createdAt := workflowExecutionTestTime(10, 0)

	tests := []struct {
		name       string
		companyID  workflowdomain.CompanyID
		workflowID workflowdomain.WorkflowID
		field      string
	}{
		{
			name:       "blank company ID",
			companyID:  workflowdomain.CompanyID(" "),
			workflowID: workflowdomain.WorkflowID("workflow-1"),
			field:      "companyID",
		},
		{
			name:       "blank workflow ID",
			companyID:  workflowdomain.CompanyID("company-1"),
			workflowID: workflowdomain.WorkflowID(" "),
			field:      "workflowID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWorkflowExecution(
				WorkflowExecutionID("execution-1"),
				test.companyID,
				test.workflowID,
				1,
				ExecutionModeSync,
				createdAt,
			)
			if err == nil {
				t.Fatal(
					"NewWorkflowExecution() returned nil error for an invalid workflow reference",
				)
			}

			var validationError *workflowdomain.ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *workflow.ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewWorkflowExecutionRejectsInvalidMode(t *testing.T) {
	_, err := NewWorkflowExecution(
		WorkflowExecutionID("execution-1"),
		workflowdomain.CompanyID("company-1"),
		workflowdomain.WorkflowID("workflow-1"),
		1,
		ExecutionMode("BACKGROUND"),
		workflowExecutionTestTime(10, 0),
	)
	if err == nil {
		t.Fatal(
			"NewWorkflowExecution() returned nil error for an invalid mode",
		)
	}

	var invalidModeError *InvalidModeError
	if !errors.As(err, &invalidModeError) {
		t.Fatalf(
			"error type = %T, want *InvalidModeError",
			err,
		)
	}

	if invalidModeError.Value != "BACKGROUND" {
		t.Fatalf(
			"invalid mode value = %q, want %q",
			invalidModeError.Value,
			"BACKGROUND",
		)
	}
}

func TestNewWorkflowExecutionRejectsZeroCreatedAt(t *testing.T) {
	_, err := NewWorkflowExecution(
		WorkflowExecutionID("execution-1"),
		workflowdomain.CompanyID("company-1"),
		workflowdomain.WorkflowID("workflow-1"),
		1,
		ExecutionModeSync,
		time.Time{},
	)
	if err == nil {
		t.Fatal(
			"NewWorkflowExecution() returned nil error for zero createdAt",
		)
	}

	var timestampError *TimestampError
	if !errors.As(err, &timestampError) {
		t.Fatalf(
			"error type = %T, want *TimestampError",
			err,
		)
	}

	if timestampError.Field != "createdAt" {
		t.Fatalf(
			"timestamp field = %q, want %q",
			timestampError.Field,
			"createdAt",
		)
	}
}

func TestWorkflowExecutionCompletesSyncLifecycle(t *testing.T) {
	createdAt := workflowExecutionTestTime(10, 0)
	validatingAt := workflowExecutionTestTime(10, 1)
	startedAt := workflowExecutionTestTime(10, 2)
	finishedAt := workflowExecutionTestTime(10, 3)

	execution := newWorkflowExecutionForTest(
		t,
		ExecutionModeSync,
		createdAt,
	)

	if err := execution.StartValidation(validatingAt); err != nil {
		t.Fatalf(
			"StartValidation() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusValidating {
		t.Fatalf(
			"status after validation = %q, want %q",
			actual,
			WorkflowExecutionStatusValidating,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists before workflow entered RUNNING",
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	actualStartedAt, exists := execution.StartedAt()
	if !exists {
		t.Fatal(
			"StartedAt() does not exist after workflow entered RUNNING",
		)
	}

	if !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, want %v",
			actualStartedAt,
			startedAt,
		)
	}

	if err := execution.Succeed(finishedAt); err != nil {
		t.Fatalf(
			"Succeed() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"final status = %q, want %q",
			actual,
			WorkflowExecutionStatusSucceeded,
		)
	}

	actualFinishedAt, exists := execution.FinishedAt()
	if !exists {
		t.Fatal(
			"FinishedAt() does not exist after terminal transition",
		)
	}

	if !actualFinishedAt.Equal(finishedAt) {
		t.Fatalf(
			"FinishedAt() = %v, want %v",
			actualFinishedAt,
			finishedAt,
		)
	}

	if !execution.IsTerminal() {
		t.Fatal(
			"IsTerminal() = false after SUCCEEDED transition",
		)
	}
}

func TestWorkflowExecutionCompletesAsyncLifecycle(t *testing.T) {
	createdAt := workflowExecutionTestTime(10, 0)
	validatingAt := workflowExecutionTestTime(10, 1)
	queuedAt := workflowExecutionTestTime(10, 2)
	startedAt := workflowExecutionTestTime(10, 3)
	finishedAt := workflowExecutionTestTime(10, 4)

	execution := newWorkflowExecutionForTest(
		t,
		ExecutionModeAsync,
		createdAt,
	)

	if err := execution.StartValidation(validatingAt); err != nil {
		t.Fatalf(
			"StartValidation() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Queue(queuedAt); err != nil {
		t.Fatalf(
			"Queue() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusQueued {
		t.Fatalf(
			"status after queue = %q, want %q",
			actual,
			WorkflowExecutionStatusQueued,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists while workflow is QUEUED",
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Succeed(finishedAt); err != nil {
		t.Fatalf(
			"Succeed() returned an unexpected error: %v",
			err,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"final status = %q, want %q",
			actual,
			WorkflowExecutionStatusSucceeded,
		)
	}

	actualStartedAt, startedExists := execution.StartedAt()
	if !startedExists || !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, exists = %t, want %v",
			actualStartedAt,
			startedExists,
			startedAt,
		)
	}

	actualFinishedAt, finishedExists := execution.FinishedAt()
	if !finishedExists || !actualFinishedAt.Equal(finishedAt) {
		t.Fatalf(
			"FinishedAt() = %v, exists = %t, want %v",
			actualFinishedAt,
			finishedExists,
			finishedAt,
		)
	}
}

func TestWorkflowExecutionTerminalPathsSetFinishedAt(t *testing.T) {
	createdAt := workflowExecutionTestTime(10, 0)
	firstAt := workflowExecutionTestTime(10, 1)
	secondAt := workflowExecutionTestTime(10, 2)
	terminalAt := workflowExecutionTestTime(10, 3)

	tests := []struct {
		name          string
		prepare       func(*WorkflowExecution) error
		complete      func(*WorkflowExecution) error
		expected      WorkflowExecutionStatus
		startedExists bool
	}{
		{
			name:    "created to cancelled",
			prepare: func(*WorkflowExecution) error { return nil },
			complete: func(execution *WorkflowExecution) error {
				return execution.Cancel(terminalAt)
			},
			expected:      WorkflowExecutionStatusCancelled,
			startedExists: false,
		},
		{
			name: "validating to rejected",
			prepare: func(execution *WorkflowExecution) error {
				return execution.StartValidation(firstAt)
			},
			complete: func(execution *WorkflowExecution) error {
				return execution.Reject(terminalAt)
			},
			expected:      WorkflowExecutionStatusRejected,
			startedExists: false,
		},
		{
			name: "queued to failed",
			prepare: func(execution *WorkflowExecution) error {
				if err := execution.StartValidation(firstAt); err != nil {
					return err
				}

				return execution.Queue(secondAt)
			},
			complete: func(execution *WorkflowExecution) error {
				return execution.Fail(terminalAt)
			},
			expected:      WorkflowExecutionStatusFailed,
			startedExists: false,
		},
		{
			name: "running to timed out",
			prepare: func(execution *WorkflowExecution) error {
				if err := execution.StartValidation(firstAt); err != nil {
					return err
				}

				return execution.Start(secondAt)
			},
			complete: func(execution *WorkflowExecution) error {
				return execution.Timeout(terminalAt)
			},
			expected:      WorkflowExecutionStatusTimedOut,
			startedExists: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution := newWorkflowExecutionForTest(
				t,
				ExecutionModeSync,
				createdAt,
			)

			if err := test.prepare(&execution); err != nil {
				t.Fatalf(
					"prepare transition returned an unexpected error: %v",
					err,
				)
			}

			if err := test.complete(&execution); err != nil {
				t.Fatalf(
					"terminal transition returned an unexpected error: %v",
					err,
				)
			}

			if actual := execution.Status(); actual != test.expected {
				t.Fatalf(
					"Status() = %q, want %q",
					actual,
					test.expected,
				)
			}

			if !execution.IsTerminal() {
				t.Fatal(
					"IsTerminal() = false after terminal transition",
				)
			}

			_, startedExists := execution.StartedAt()
			if startedExists != test.startedExists {
				t.Fatalf(
					"StartedAt() exists = %t, want %t",
					startedExists,
					test.startedExists,
				)
			}

			actualFinishedAt, finishedExists := execution.FinishedAt()
			if !finishedExists {
				t.Fatal(
					"FinishedAt() does not exist after terminal transition",
				)
			}

			if !actualFinishedAt.Equal(terminalAt) {
				t.Fatalf(
					"FinishedAt() = %v, want %v",
					actualFinishedAt,
					terminalAt,
				)
			}
		})
	}
}

func TestWorkflowExecutionTransitionMatrix(t *testing.T) {
	statuses := []WorkflowExecutionStatus{
		WorkflowExecutionStatusCreated,
		WorkflowExecutionStatusValidating,
		WorkflowExecutionStatusRejected,
		WorkflowExecutionStatusQueued,
		WorkflowExecutionStatusRunning,
		WorkflowExecutionStatusSucceeded,
		WorkflowExecutionStatusFailed,
		WorkflowExecutionStatusCancelled,
		WorkflowExecutionStatusTimedOut,
	}

	type transition struct {
		current WorkflowExecutionStatus
		target  WorkflowExecutionStatus
	}

	allowed := map[transition]struct{}{
		{
			current: WorkflowExecutionStatusCreated,
			target:  WorkflowExecutionStatusValidating,
		}: {},
		{
			current: WorkflowExecutionStatusCreated,
			target:  WorkflowExecutionStatusCancelled,
		}: {},
		{
			current: WorkflowExecutionStatusValidating,
			target:  WorkflowExecutionStatusRejected,
		}: {},
		{
			current: WorkflowExecutionStatusValidating,
			target:  WorkflowExecutionStatusQueued,
		}: {},
		{
			current: WorkflowExecutionStatusValidating,
			target:  WorkflowExecutionStatusRunning,
		}: {},
		{
			current: WorkflowExecutionStatusValidating,
			target:  WorkflowExecutionStatusCancelled,
		}: {},
		{
			current: WorkflowExecutionStatusValidating,
			target:  WorkflowExecutionStatusTimedOut,
		}: {},
		{
			current: WorkflowExecutionStatusQueued,
			target:  WorkflowExecutionStatusRunning,
		}: {},
		{
			current: WorkflowExecutionStatusQueued,
			target:  WorkflowExecutionStatusFailed,
		}: {},
		{
			current: WorkflowExecutionStatusQueued,
			target:  WorkflowExecutionStatusCancelled,
		}: {},
		{
			current: WorkflowExecutionStatusQueued,
			target:  WorkflowExecutionStatusTimedOut,
		}: {},
		{
			current: WorkflowExecutionStatusRunning,
			target:  WorkflowExecutionStatusSucceeded,
		}: {},
		{
			current: WorkflowExecutionStatusRunning,
			target:  WorkflowExecutionStatusFailed,
		}: {},
		{
			current: WorkflowExecutionStatusRunning,
			target:  WorkflowExecutionStatusCancelled,
		}: {},
		{
			current: WorkflowExecutionStatusRunning,
			target:  WorkflowExecutionStatusTimedOut,
		}: {},
	}

	for _, current := range statuses {
		for _, target := range statuses {
			name := current.String() + "_to_" + target.String()

			t.Run(name, func(t *testing.T) {
				_, expected := allowed[transition{
					current: current,
					target:  target,
				}]

				actual := isWorkflowTransitionAllowed(
					current,
					target,
				)

				if actual != expected {
					t.Fatalf(
						"isWorkflowTransitionAllowed(%q, %q) = %t, want %t",
						current,
						target,
						actual,
						expected,
					)
				}
			})
		}
	}
}

func TestWorkflowExecutionInvalidTransitionPreservesState(
	t *testing.T,
) {
	createdAt := workflowExecutionTestTime(10, 0)
	validatingAt := workflowExecutionTestTime(10, 1)
	startedAt := workflowExecutionTestTime(10, 2)
	invalidAt := workflowExecutionTestTime(10, 3)

	execution := newWorkflowExecutionForTest(
		t,
		ExecutionModeSync,
		createdAt,
	)

	if err := execution.StartValidation(validatingAt); err != nil {
		t.Fatalf(
			"StartValidation() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	err := execution.Queue(invalidAt)
	if err == nil {
		t.Fatal(
			"Queue() returned nil error for RUNNING to QUEUED transition",
		)
	}

	var transitionError *TransitionError
	if !errors.As(err, &transitionError) {
		t.Fatalf(
			"error type = %T, want *TransitionError",
			err,
		)
	}

	if transitionError.CurrentStatus != "RUNNING" {
		t.Fatalf(
			"current status = %q, want %q",
			transitionError.CurrentStatus,
			"RUNNING",
		)
	}

	if transitionError.TargetStatus != "QUEUED" {
		t.Fatalf(
			"target status = %q, want %q",
			transitionError.TargetStatus,
			"QUEUED",
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusRunning {
		t.Fatalf(
			"status after failed transition = %q, want %q",
			actual,
			WorkflowExecutionStatusRunning,
		)
	}

	actualStartedAt, exists := execution.StartedAt()
	if !exists || !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, exists = %t, want %v",
			actualStartedAt,
			exists,
			startedAt,
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists after failed non-terminal transition",
		)
	}
}

func TestWorkflowExecutionTerminalStateRejectsFurtherTransitions(
	t *testing.T,
) {
	createdAt := workflowExecutionTestTime(10, 0)
	validatingAt := workflowExecutionTestTime(10, 1)
	startedAt := workflowExecutionTestTime(10, 2)
	finishedAt := workflowExecutionTestTime(10, 3)
	invalidAt := workflowExecutionTestTime(10, 4)

	execution := newWorkflowExecutionForTest(
		t,
		ExecutionModeSync,
		createdAt,
	)

	if err := execution.StartValidation(validatingAt); err != nil {
		t.Fatalf(
			"StartValidation() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Succeed(finishedAt); err != nil {
		t.Fatalf(
			"Succeed() returned an unexpected error: %v",
			err,
		)
	}

	err := execution.Start(invalidAt)
	if err == nil {
		t.Fatal(
			"Start() returned nil error for a terminal workflow execution",
		)
	}

	var transitionError *TransitionError
	if !errors.As(err, &transitionError) {
		t.Fatalf(
			"error type = %T, want *TransitionError",
			err,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"status after rejected terminal transition = %q, want %q",
			actual,
			WorkflowExecutionStatusSucceeded,
		)
	}

	actualFinishedAt, exists := execution.FinishedAt()
	if !exists || !actualFinishedAt.Equal(finishedAt) {
		t.Fatalf(
			"FinishedAt() = %v, exists = %t, want %v",
			actualFinishedAt,
			exists,
			finishedAt,
		)
	}
}

func TestWorkflowExecutionInvalidTimestampPreservesState(
	t *testing.T,
) {
	createdAt := workflowExecutionTestTime(10, 0)
	beforeCreatedAt := workflowExecutionTestTime(9, 59)

	execution := newWorkflowExecutionForTest(
		t,
		ExecutionModeSync,
		createdAt,
	)

	err := execution.StartValidation(beforeCreatedAt)
	if err == nil {
		t.Fatal(
			"StartValidation() returned nil error for timestamp before createdAt",
		)
	}

	var timestampError *TimestampError
	if !errors.As(err, &timestampError) {
		t.Fatalf(
			"error type = %T, want *TimestampError",
			err,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusCreated {
		t.Fatalf(
			"status after failed timestamp validation = %q, want %q",
			actual,
			WorkflowExecutionStatusCreated,
		)
	}

	if _, exists := execution.StartedAt(); exists {
		t.Fatal(
			"StartedAt() exists after rejected timestamp",
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists after rejected timestamp",
		)
	}
}

func TestWorkflowExecutionTerminalTimestampCannotPrecedeStartedAt(
	t *testing.T,
) {
	createdAt := workflowExecutionTestTime(10, 0)
	validatingAt := workflowExecutionTestTime(10, 1)
	startedAt := workflowExecutionTestTime(10, 3)
	invalidFinishedAt := workflowExecutionTestTime(10, 2)

	execution := newWorkflowExecutionForTest(
		t,
		ExecutionModeSync,
		createdAt,
	)

	if err := execution.StartValidation(validatingAt); err != nil {
		t.Fatalf(
			"StartValidation() returned an unexpected error: %v",
			err,
		)
	}

	if err := execution.Start(startedAt); err != nil {
		t.Fatalf(
			"Start() returned an unexpected error: %v",
			err,
		)
	}

	err := execution.Succeed(invalidFinishedAt)
	if err == nil {
		t.Fatal(
			"Succeed() returned nil error for finishedAt before startedAt",
		)
	}

	var timestampError *TimestampError
	if !errors.As(err, &timestampError) {
		t.Fatalf(
			"error type = %T, want *TimestampError",
			err,
		)
	}

	if actual := execution.Status(); actual != WorkflowExecutionStatusRunning {
		t.Fatalf(
			"status after rejected terminal timestamp = %q, want %q",
			actual,
			WorkflowExecutionStatusRunning,
		)
	}

	actualStartedAt, exists := execution.StartedAt()
	if !exists || !actualStartedAt.Equal(startedAt) {
		t.Fatalf(
			"StartedAt() = %v, exists = %t, want %v",
			actualStartedAt,
			exists,
			startedAt,
		)
	}

	if _, exists := execution.FinishedAt(); exists {
		t.Fatal(
			"FinishedAt() exists after rejected terminal timestamp",
		)
	}
}

func TestNilWorkflowExecutionRejectsTransition(t *testing.T) {
	var execution *WorkflowExecution

	err := execution.StartValidation(
		workflowExecutionTestTime(10, 0),
	)
	if err == nil {
		t.Fatal(
			"StartValidation() returned nil error for nil receiver",
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != "workflowExecution" {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			"workflowExecution",
		)
	}
}

func newWorkflowExecutionForTest(
	t *testing.T,
	mode ExecutionMode,
	createdAt time.Time,
) WorkflowExecution {
	t.Helper()

	execution, err := NewWorkflowExecution(
		WorkflowExecutionID("execution-1"),
		workflowdomain.CompanyID("company-1"),
		workflowdomain.WorkflowID("workflow-1"),
		1,
		mode,
		createdAt,
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecution() returned an unexpected error: %v",
			err,
		)
	}

	return execution
}

func workflowExecutionTestTime(
	hour int,
	minute int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		16,
		hour,
		minute,
		0,
		0,
		time.UTC,
	)
}

func TestWorkflowExecutionStatusCanTransitionTo(
	t *testing.T,
) {
	tests := []struct {
		name     string
		current  WorkflowExecutionStatus
		target   WorkflowExecutionStatus
		expected bool
	}{
		{
			name:     "created to validating",
			current:  WorkflowExecutionStatusCreated,
			target:   WorkflowExecutionStatusValidating,
			expected: true,
		},
		{
			name:     "validating to running",
			current:  WorkflowExecutionStatusValidating,
			target:   WorkflowExecutionStatusRunning,
			expected: true,
		},
		{
			name:     "queued to failed",
			current:  WorkflowExecutionStatusQueued,
			target:   WorkflowExecutionStatusFailed,
			expected: true,
		},
		{
			name:     "running to succeeded",
			current:  WorkflowExecutionStatusRunning,
			target:   WorkflowExecutionStatusSucceeded,
			expected: true,
		},
		{
			name:     "created to succeeded",
			current:  WorkflowExecutionStatusCreated,
			target:   WorkflowExecutionStatusSucceeded,
			expected: false,
		},
		{
			name:     "terminal to running",
			current:  WorkflowExecutionStatusFailed,
			target:   WorkflowExecutionStatusRunning,
			expected: false,
		},
		{
			name:     "invalid current status",
			current:  WorkflowExecutionStatus("UNKNOWN"),
			target:   WorkflowExecutionStatusRunning,
			expected: false,
		},
		{
			name:     "invalid target status",
			current:  WorkflowExecutionStatusRunning,
			target:   WorkflowExecutionStatus("UNKNOWN"),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual :=
				test.current.CanTransitionTo(
					test.target,
				)

			if actual != test.expected {
				t.Fatalf(
					"CanTransitionTo() = %t, want %t",
					actual,
					test.expected,
				)
			}
		})
	}
}
