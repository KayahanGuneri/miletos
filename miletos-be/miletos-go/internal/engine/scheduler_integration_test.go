//go:build integration

package engine_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"miletos-go/internal/engine"
	"miletos-go/internal/model"
	"miletos-go/internal/repository"
)

func TestSchedulerDoesNotActivateTerminalExecutionsIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	terminalStatuses := []string{
		model.ExecutionSucceeded,
		model.ExecutionFailed,
		model.ExecutionCancelled,
		model.ExecutionRejected,
		model.ExecutionTimedOut,
	}

	for _, status := range terminalStatuses {
		t.Run(status, func(t *testing.T) {
			workflow := engineWorkflow()
			workflow.ID = "terminal-activation-" + strings.ToLower(status)
			execution := seedEngineExecution(t, pool, workflow, "ASYNC")
			executions := repository.NewExecutionRepository(pool)
			nodeQueue := &recordingQueue{}
			scheduler := engine.NewScheduler(
				repository.NewWorkflowRepository(pool), executions, nodeQueue, "commands",
			)
			setTerminalExecution(t, pool, execution.ID, status)
			before, err := executions.FindByID(
				context.Background(), workflow.CompanyID, execution.ID,
			)
			if err != nil {
				t.Fatalf("FindByID() before activation error = %v", err)
			}
			beforeCounts := countExecutionArtifacts(t, pool, execution.ID, nodeQueue)

			for activation := 1; activation <= 2; activation++ {
				scheduled, err := scheduler.Activate(context.Background(), execution, nil)
				if err != nil || scheduled != 0 {
					t.Fatalf("Activate() #%d = (%d, %v), want no-op", activation, scheduled, err)
				}
			}

			after, err := executions.FindByID(
				context.Background(), workflow.CompanyID, execution.ID,
			)
			if err != nil {
				t.Fatalf("FindByID() after activation error = %v", err)
			}
			afterCounts := countExecutionArtifacts(t, pool, execution.ID, nodeQueue)
			if after.Status != status ||
				!reflect.DeepEqual(after.TerminalOutputs, before.TerminalOutputs) ||
				!reflect.DeepEqual(after.Failure, before.Failure) {
				t.Fatalf("terminal execution changed: before=%#v after=%#v", before, after)
			}
			if afterCounts != beforeCounts {
				t.Fatalf("terminal artifacts changed: before=%#v after=%#v", beforeCounts, afterCounts)
			}
			nodes, err := executions.ListAllNodes(
				context.Background(), workflow.CompanyID, execution.ID,
			)
			if err != nil {
				t.Fatalf("ListAllNodes() error = %v", err)
			}
			for _, node := range nodes {
				if node.Status != model.NodePending || node.Attempt != 1 {
					t.Fatalf("node changed during terminal activation: %#v", node)
				}
			}
		})
	}
}

func TestSchedulerDoesNotContinueTerminalExecutionsIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	terminalStatuses := []string{
		model.ExecutionSucceeded,
		model.ExecutionFailed,
		model.ExecutionCancelled,
		model.ExecutionRejected,
		model.ExecutionTimedOut,
	}

	for _, status := range terminalStatuses {
		t.Run(status, func(t *testing.T) {
			workflow := engineWorkflow()
			workflow.ID = "terminal-continuation-" + strings.ToLower(status)
			execution := seedEngineExecution(t, pool, workflow, "ASYNC")
			executions := repository.NewExecutionRepository(pool)
			nodeQueue := &recordingQueue{}
			scheduler := engine.NewScheduler(
				repository.NewWorkflowRepository(pool), executions, nodeQueue, "commands",
			)
			if scheduled, err := scheduler.Activate(
				context.Background(), execution, nil,
			); err != nil || scheduled != 1 {
				t.Fatalf("Activate() = (%d, %v)", scheduled, err)
			}
			rootJob := nodeQueue.snapshot()[0]
			if started, err := executions.MarkNodeRunning(
				context.Background(), rootJob,
			); err != nil || !started {
				t.Fatalf("MarkNodeRunning() = (%v, %v)", started, err)
			}
			if err := executions.SaveNodeSuccess(
				context.Background(), rootJob, "root-output",
			); err != nil {
				t.Fatalf("SaveNodeSuccess() error = %v", err)
			}
			setTerminalExecution(t, pool, execution.ID, status)
			beforeCounts := countExecutionArtifacts(t, pool, execution.ID, nodeQueue)

			if err := scheduler.CompleteNode(context.Background(), rootJob); err != nil {
				t.Fatalf("CompleteNode() error = %v", err)
			}
			if err := scheduler.CompleteNode(context.Background(), rootJob); err != nil {
				t.Fatalf("replayed CompleteNode() error = %v", err)
			}

			afterCounts := countExecutionArtifacts(t, pool, execution.ID, nodeQueue)
			if afterCounts != beforeCounts {
				t.Fatalf("terminal continuation changed artifacts: before=%#v after=%#v",
					beforeCounts, afterCounts)
			}
			persisted, err := executions.FindByID(
				context.Background(), workflow.CompanyID, execution.ID,
			)
			if err != nil || persisted.Status != status ||
				persisted.TerminalOutputs["preservedOutput"] != "value" ||
				persisted.Failure["message"] != "preserved error" {
				t.Fatalf("terminal execution after continuation = (%#v, %v)", persisted, err)
			}
			next, err := executions.FindNode(
				context.Background(), workflow.CompanyID, execution.ID, "terminal",
			)
			if err != nil || next.Status != model.NodePending {
				t.Fatalf("dependent node after continuation = (%#v, %v)", next, err)
			}
		})
	}
}

func TestSchedulerActivatesRunningExecutionWithoutReopeningItIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	workflow.ID = "running-activation"
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	if _, err := pool.Exec(context.Background(), `
		UPDATE workflow_runtime.workflow_executions
		SET status = 'RUNNING', started_at = created_at, updated_at = created_at
		WHERE workflow_execution_id = $1`,
		execution.ID,
	); err != nil {
		t.Fatalf("set execution RUNNING: %v", err)
	}
	executions := repository.NewExecutionRepository(pool)
	nodeQueue := &recordingQueue{}
	scheduler := engine.NewScheduler(
		repository.NewWorkflowRepository(pool), executions, nodeQueue, "commands",
	)

	scheduled, err := scheduler.Activate(context.Background(), execution, nil)

	if err != nil || scheduled != 1 || len(nodeQueue.snapshot()) != 1 {
		t.Fatalf("Activate() = (%d, %v), jobs=%#v", scheduled, err, nodeQueue.snapshot())
	}
	persisted, err := executions.FindByID(
		context.Background(), workflow.CompanyID, execution.ID,
	)
	if err != nil || persisted.Status != model.ExecutionRunning {
		t.Fatalf("running execution after activation = (%#v, %v)", persisted, err)
	}
	if repeated, err := scheduler.Activate(
		context.Background(), execution, nil,
	); err != nil || repeated != 0 || len(nodeQueue.snapshot()) != 1 {
		t.Fatalf("repeated Activate() = (%d, %v), jobs=%#v",
			repeated, err, nodeQueue.snapshot())
	}
}

func TestSchedulerSchedulesAndContinuesWorkflowIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(pool)
	nodeQueue := &recordingQueue{}
	scheduler := engine.NewScheduler(
		repository.NewWorkflowRepository(pool), executions, nodeQueue, "commands",
	)

	scheduled, err := scheduler.Activate(context.Background(), execution, map[string]any{"initial": true})
	if err != nil || scheduled != 1 {
		t.Fatalf("Activate() = (%d, %v)", scheduled, err)
	}
	jobs := nodeQueue.snapshot()
	if len(jobs) != 1 || jobs[0].NodeID != "root" ||
		jobs[0].CompanyID != workflow.CompanyID ||
		jobs[0].CorrelationID != execution.CorrelationID {
		t.Fatalf("root jobs = %#v", jobs)
	}
	if payload, ok := jobs[0].Payload.(map[string]any); !ok || payload["initial"] != true {
		t.Fatalf("root payload = %#v", jobs[0].Payload)
	}
	if duplicate, err := scheduler.Activate(context.Background(), execution, nil); err != nil || duplicate != 0 {
		t.Fatalf("duplicate Activate() = (%d, %v)", duplicate, err)
	}

	rootJob := jobs[0]
	if started, err := executions.MarkNodeRunning(context.Background(), rootJob); err != nil || !started {
		t.Fatalf("root MarkNodeRunning() = (%v, %v)", started, err)
	}
	if err := executions.SaveNodeSuccess(context.Background(), rootJob, "root-output"); err != nil {
		t.Fatalf("root SaveNodeSuccess() error = %v", err)
	}
	if err := scheduler.CompleteNode(context.Background(), rootJob); err != nil {
		t.Fatalf("root CompleteNode() error = %v", err)
	}
	jobs = nodeQueue.snapshot()
	if len(jobs) != 2 || jobs[1].NodeID != "terminal" || jobs[1].Payload != "root-output" {
		t.Fatalf("continued jobs = %#v", jobs)
	}
	terminalJob := jobs[1]
	if started, err := executions.MarkNodeRunning(context.Background(), terminalJob); err != nil || !started {
		t.Fatalf("terminal MarkNodeRunning() = (%v, %v)", started, err)
	}
	if err := executions.SaveNodeSuccess(context.Background(), terminalJob, "terminal-output"); err != nil {
		t.Fatalf("terminal SaveNodeSuccess() error = %v", err)
	}
	if err := scheduler.CompleteNode(context.Background(), terminalJob); err != nil {
		t.Fatalf("terminal CompleteNode() error = %v", err)
	}
	completed, err := executions.FindByID(context.Background(), workflow.CompanyID, execution.ID)
	if err != nil || completed.Status != model.ExecutionSucceeded ||
		completed.TerminalOutputs["terminal"] == nil {
		t.Fatalf("completed execution = (%#v, %v)", completed, err)
	}
}

func TestSchedulerContinuesIndependentBranchAndSkipsBlockedNodesIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := model.Workflow{
		ID: "branch-workflow", CompanyID: "company-1", Name: "Branches", Revision: 1,
		Nodes: []model.Node{
			{ID: "failed-root", Type: "test.node", Version: "v1"},
			{ID: "blocked", Type: "test.node", Version: "v1"},
			{ID: "independent", Type: "test.node", Version: "v1"},
		},
		Edges: []model.Edge{{
			ID: "edge-1", SourceNodeID: "failed-root", SourceOutputPort: "output",
			TargetNodeID: "blocked", TargetInputPort: "input",
		}},
	}
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(pool)
	nodeQueue := &recordingQueue{}
	scheduler := engine.NewScheduler(
		repository.NewWorkflowRepository(pool), executions, nodeQueue, "commands",
	)
	if scheduled, err := scheduler.Activate(context.Background(), execution, nil); err != nil || scheduled != 2 {
		t.Fatalf("Activate() = (%d, %v)", scheduled, err)
	}
	jobs := nodeQueue.snapshot()
	var failedJob, independentJob model.NodeJob
	for _, job := range jobs {
		switch job.NodeID {
		case "failed-root":
			failedJob = job
		case "independent":
			independentJob = job
		}
	}
	if started, err := executions.MarkNodeRunning(context.Background(), failedJob); err != nil || !started {
		t.Fatalf("failed root MarkNodeRunning() = (%v, %v)", started, err)
	}
	failedAt := nodeStartedAt(t, pool, failedJob.NodeExecutionID)
	if err := executions.SaveNodeFailure(
		context.Background(), failedJob,
		map[string]any{"category": "EXECUTION", "code": "FAILED", "message": "failed"},
		false, failedAt, time.Time{}, 1, time.Second,
	); err != nil {
		t.Fatalf("SaveNodeFailure() error = %v", err)
	}
	if err := scheduler.CompleteNode(context.Background(), failedJob); err != nil {
		t.Fatalf("failed root CompleteNode() error = %v", err)
	}
	midway, err := executions.FindByID(context.Background(), workflow.CompanyID, execution.ID)
	if err != nil || midway.Status != model.ExecutionRunning {
		t.Fatalf("midway execution = (%#v, %v)", midway, err)
	}
	if started, err := executions.MarkNodeRunning(context.Background(), independentJob); err != nil || !started {
		t.Fatalf("independent MarkNodeRunning() = (%v, %v)", started, err)
	}
	if err := executions.SaveNodeSuccess(context.Background(), independentJob, "success"); err != nil {
		t.Fatalf("independent SaveNodeSuccess() error = %v", err)
	}
	if err := scheduler.CompleteNode(context.Background(), independentJob); err != nil {
		t.Fatalf("independent CompleteNode() error = %v", err)
	}
	completed, err := executions.FindByID(context.Background(), workflow.CompanyID, execution.ID)
	if err != nil || completed.Status != model.ExecutionFailed {
		t.Fatalf("completed execution = (%#v, %v)", completed, err)
	}
	blocked, err := executions.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "blocked",
	)
	if err != nil || blocked.Status != model.NodeSkipped {
		t.Fatalf("blocked node = (%#v, %v)", blocked, err)
	}
}

func TestSchedulerKeepsRetryPendingExecutionActiveIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(pool)
	nodeQueue := &recordingQueue{}
	scheduler := engine.NewScheduler(
		repository.NewWorkflowRepository(pool), executions, nodeQueue, "commands",
	)
	if _, err := scheduler.Activate(context.Background(), execution, nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	job := nodeQueue.snapshot()[0]
	if started, err := executions.MarkNodeRunning(context.Background(), job); err != nil || !started {
		t.Fatalf("MarkNodeRunning() = (%v, %v)", started, err)
	}
	failedAt := nodeStartedAt(t, pool, job.NodeExecutionID)
	if err := executions.SaveNodeFailure(
		context.Background(), job,
		map[string]any{"category": "EXECUTION", "code": "RETRY", "message": "retry"},
		true, failedAt, failedAt.Add(time.Second), 3, time.Second,
	); err != nil {
		t.Fatalf("SaveNodeFailure() error = %v", err)
	}
	if err := scheduler.CompleteNode(context.Background(), job); err != nil {
		t.Fatalf("CompleteNode() error = %v", err)
	}
	active, err := executions.FindByID(context.Background(), workflow.CompanyID, execution.ID)
	if err != nil || active.FinishedAt != nil {
		t.Fatalf("active execution = (%#v, %v)", active, err)
	}
}

func TestSchedulerRestoresNodeAfterQueueFailureIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(pool)
	queueError := errors.New("queue unavailable")
	scheduler := engine.NewScheduler(
		repository.NewWorkflowRepository(pool), executions,
		&recordingQueue{err: queueError}, "commands",
	)

	_, err := scheduler.Activate(context.Background(), execution, nil)

	if !errors.Is(err, queueError) {
		t.Fatalf("Activate() error = %v, want %v", err, queueError)
	}
	root, err := executions.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil || root.Status != model.NodePending {
		t.Fatalf("root after queue failure = (%#v, %v)", root, err)
	}
}
