//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"miletos-go/internal/model"
)

func TestObservabilityRepositoryIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()
	_, execution := seedIntegrationExecution(t, pool, workflow, "SYNC")
	repository := NewExecutionRepository(pool)
	node, err := repository.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil {
		t.Fatalf("FindNode() error = %v", err)
	}
	job := model.NodeJob{
		CompanyID: workflow.CompanyID, WorkflowID: workflow.ID,
		ExecutionID: execution.ID, NodeID: "root", NodeExecutionID: node.ID, Attempt: 1,
	}

	if err := repository.RecordEvent(
		context.Background(), execution.ID, node.ID, "TEST_EVENT",
		model.NodePending, model.NodeReady, "test event",
	); err != nil {
		t.Fatalf("RecordEvent() error = %v", err)
	}
	if err := repository.RecordLog(
		context.Background(), execution.ID, node.ID, "INFO", "test log",
		map[string]any{"source": "integration"},
	); err != nil {
		t.Fatalf("RecordLog() error = %v", err)
	}
	if err := repository.RecordError(
		context.Background(), job,
		map[string]any{
			"category": "EXECUTION", "code": "TEST_ERROR",
			"message": "test error", "failedNodeCount": "1",
		},
		false,
		time.Date(2026, time.July, 28, 10, 30, 0, 0, time.UTC),
	); err != nil {
		t.Fatalf("RecordError() error = %v", err)
	}

	events, err := repository.ListEvents(
		context.Background(), workflow.CompanyID, execution.ID, "", 100,
	)
	if err != nil || len(events.Items) < 2 {
		t.Fatalf("ListEvents() = (%#v, %v)", events, err)
	}
	for index := 1; index < len(events.Items); index++ {
		if events.Items[index].Sequence <= events.Items[index-1].Sequence {
			t.Fatalf("event sequence is not stable: %#v", events.Items)
		}
	}
	logs, err := repository.ListLogs(
		context.Background(), workflow.CompanyID, execution.ID, "", 100,
	)
	if err != nil || len(logs.Items) != 1 || logs.Items[0].Message != "test log" {
		t.Fatalf("ListLogs() = (%#v, %v)", logs, err)
	}
	errorsPage, err := repository.ListErrors(
		context.Background(), workflow.CompanyID, execution.ID, "", 100,
	)
	if err != nil || len(errorsPage.Items) != 1 ||
		errorsPage.Items[0].Code != "TEST_ERROR" ||
		errorsPage.Items[0].Details["failedNodeCount"] != "1" {
		t.Fatalf("ListErrors() = (%#v, %v)", errorsPage, err)
	}
	isolated, err := repository.ListEvents(
		context.Background(), "another-company", execution.ID, "", 100,
	)
	if err != nil || len(isolated.Items) != 0 {
		t.Fatalf("company-isolated ListEvents() = (%#v, %v)", isolated, err)
	}
}
