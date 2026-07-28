package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

var _ repository.DurableWorkerResultStore = (*Store)(nil)

const durableWorkerResultColumns = `worker_result_id, company_id, workflow_execution_id, node_execution_id, node_id, attempt, result_status, routed_outputs_json, terminal_output_json, context_changes_json, failure_json, technical_detail, started_at, finished_at, created_at`
const insertDurableWorkerResultSQL = `INSERT INTO workflow_runtime.async_worker_results (` + durableWorkerResultColumns + `)
SELECT $1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10::jsonb,$11::jsonb,$12,$13,$14,$15
FROM workflow_runtime.node_executions AS node
WHERE node.node_execution_id=$4 AND node.workflow_execution_id=$3 AND node.company_id=$2 AND node.node_id=$5`

func (store *Store) CreateDurableWorkerResult(ctx context.Context, result repository.DurableWorkerResult) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	return createDurableWorkerResult(ctx, store.pool, result)
}
func createDurableWorkerResult(ctx context.Context, db asyncPersistenceDatabase, result repository.DurableWorkerResult) error {
	if ctx == nil {
		return fmt.Errorf("create durable worker result context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || !result.IsValid() {
		return fmt.Errorf("worker result database and result must be valid")
	}
	routedOutputs, terminalOutput, contextChanges, failure, err := encodeDurableWorkerResultParts(result)
	if err != nil {
		return err
	}
	technicalDetail := any(nil)
	if value, exists := result.TechnicalDetail(); exists {
		technicalDetail = value
	}
	tag, err := db.Exec(ctx, insertDurableWorkerResultSQL, result.IdentityKey(), result.CompanyID().String(), result.WorkflowExecutionID().String(),
		result.NodeExecutionID().String(), result.NodeID().String(), result.Attempt(), string(result.Status()), routedOutputs, terminalOutput, contextChanges, failure,
		technicalDetail, result.StartedAt(), result.FinishedAt(), result.CreatedAt(),
	)
	if err != nil {
		return mapDurableWorkerResultWriteError("create", err)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewConflictError("create", "durable worker result node identity", errors.New("node identity does not match workflow execution"))
	}
	return nil
}
func encodeDurableWorkerResultParts(result repository.DurableWorkerResult) (string, any, string, any, error) {
	emptyObject := "{}"
	switch result.Status() {
	case repository.DurableWorkerResultSucceeded:
		runtimeResult, exists := result.Result()
		if !exists {
			return "", nil, "", nil, errors.New("successful durable result has no runtime result")
		}
		encodedContext, err := encodeContextChanges(runtimeResult.ContextChanges())
		if err != nil {
			return "", nil, "", nil, err
		}
		if terminal, exists := runtimeResult.TerminalOutput(); exists {
			encodedTerminal, err := encodeRuntimePayload(terminal)
			if err != nil {
				return "", nil, "", nil, err
			}
			return emptyObject, string(encodedTerminal), string(encodedContext), nil, nil
		}
		encodedOutputs, err := encodeRoutedOutputs(runtimeResult.OutputsSnapshot())
		if err != nil {
			return "", nil, "", nil, err
		}
		return string(encodedOutputs), nil, string(encodedContext), nil, nil
	case repository.DurableWorkerResultFailed:
		runtimeResult, exists := result.Result()
		if !exists {
			return "", nil, "", nil, errors.New("failed durable result has no runtime result")
		}
		resultFailure, exists := runtimeResult.Failure()
		if !exists {
			return "", nil, "", nil, errors.New("failed runtime result has no failure")
		}
		encodedFailure, err := encodeRuntimeFailure(resultFailure)
		if err != nil {
			return "", nil, "", nil, err
		}
		return emptyObject, nil, emptyObject, string(encodedFailure), nil
	case repository.DurableWorkerResultCancelled, repository.DurableWorkerResultTimedOut:
		resultFailure, exists := result.Failure()
		if !exists {
			return "", nil, "", nil, errors.New("interrupted durable result has no failure")
		}
		encodedFailure, err := encodeRuntimeFailure(resultFailure)
		if err != nil {
			return "", nil, "", nil, err
		}
		return emptyObject, nil, emptyObject, string(encodedFailure), nil
	default:
		return "", nil, "", nil, errors.New("unsupported durable worker result status")
	}
}

const getDurableWorkerResultSQL = `SELECT ` + durableWorkerResultColumns + `
FROM workflow_runtime.async_worker_results
WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3 AND attempt=$4`

func (store *Store) GetDurableWorkerResult(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, nodeExecutionID execution.NodeExecutionID, attempt int16,
) (repository.DurableWorkerResult, error) {
	if !store.IsValid() {
		return repository.DurableWorkerResult{}, fmt.Errorf("PostgreSQL store must be valid")
	}
	return getDurableWorkerResult(ctx, store.pool, companyID, workflowExecutionID, nodeExecutionID, attempt)
}
func getDurableWorkerResult(ctx context.Context,
	db asyncPersistenceDatabase, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID, attempt int16) (repository.DurableWorkerResult, error) {
	if ctx == nil {
		return repository.DurableWorkerResult{}, fmt.Errorf("get durable worker result context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.DurableWorkerResult{}, err
	}
	if db == nil || attempt <= 0 {
		return repository.DurableWorkerResult{}, fmt.Errorf("worker result database and attempt must be valid")
	}
	validatedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return repository.DurableWorkerResult{}, fmt.Errorf("company ID must be valid")
	}
	validatedExecutionID, err := execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return repository.DurableWorkerResult{}, fmt.Errorf("workflow execution ID must be valid")
	}
	validatedNodeID, err := execution.NewNodeExecutionID(nodeExecutionID.String())
	if err != nil {
		return repository.DurableWorkerResult{}, fmt.Errorf("node execution ID must be valid")
	}
	return scanDurableWorkerResult(db.QueryRow(ctx, getDurableWorkerResultSQL, validatedCompanyID.String(), validatedExecutionID.String(), validatedNodeID.String(), attempt))
}

func scanDurableWorkerResult(row rowScanner) (repository.DurableWorkerResult, error) {
	var resultID, companyID, workflowExecutionID, nodeExecutionID, nodeID, status string
	var attempt int16
	var routedOutputs, contextChanges []byte
	var terminalOutput, failure []byte
	var technicalDetail *string
	var startedAt, finishedAt, createdAt time.Time
	if err := row.Scan(&resultID, &companyID, &workflowExecutionID, &nodeExecutionID, &nodeID, &attempt, &status, &routedOutputs, &terminalOutput, &contextChanges, &failure,
		&technicalDetail, &startedAt, &finishedAt, &createdAt); err != nil {
		return repository.DurableWorkerResult{}, mapPostgreSQLError("hydrate", "durable worker result", err)
	}
	params := repository.DurableWorkerResultParams{
		CompanyID: workflow.CompanyID(companyID), WorkflowExecutionID: execution.WorkflowExecutionID(workflowExecutionID), NodeExecutionID: execution.NodeExecutionID(nodeExecutionID),
		NodeID: workflow.NodeID(nodeID), Attempt: attempt, Status: repository.DurableWorkerResultStatus(status), StartedAt: startedAt, FinishedAt: finishedAt, CreatedAt: createdAt,
	}
	if technicalDetail != nil {
		params.TechnicalDetail = *technicalDetail
	}
	switch params.Status {
	case repository.DurableWorkerResultSucceeded:
		changes, err := decodeContextChanges(contextChanges)
		if err != nil {
			return repository.DurableWorkerResult{}, err
		}
		if len(terminalOutput) > 0 {
			payload, err := decodeRuntimePayload(terminalOutput)
			if err != nil {
				return repository.DurableWorkerResult{}, err
			}
			params.Result, err = runtime.NewTerminalNodeSuccessResult(payload, changes)
			if err != nil {
				return repository.DurableWorkerResult{}, &databaseError{operation: "hydrate", resource: "durable worker result", cause: err}
			}
		} else {
			outputs, err := decodeRoutedOutputs(routedOutputs)
			if err != nil {
				return repository.DurableWorkerResult{}, err
			}
			params.Result, err = runtime.NewNodeSuccessResult(outputs, changes)
			if err != nil {
				return repository.DurableWorkerResult{}, &databaseError{operation: "hydrate", resource: "durable worker result", cause: err}
			}
		}
	case repository.DurableWorkerResultFailed:
		decodedFailure, err := decodeRuntimeFailure(failure)
		if err != nil {
			return repository.DurableWorkerResult{}, err
		}
		params.Result, err = runtime.NewNodeFailureResult(decodedFailure)
		if err != nil {
			return repository.DurableWorkerResult{}, &databaseError{operation: "hydrate", resource: "durable worker result", cause: err}
		}
	case repository.DurableWorkerResultCancelled, repository.DurableWorkerResultTimedOut:
		decodedFailure, err := decodeRuntimeFailure(failure)
		if err != nil {
			return repository.DurableWorkerResult{}, err
		}
		params.Failure = decodedFailure
	default:
		return repository.DurableWorkerResult{}, &databaseError{operation: "hydrate", resource: "durable worker result", cause: errors.New("unsupported persisted result status")}
	}
	result, err := repository.NewDurableWorkerResult(params)
	if err != nil || result.IdentityKey() != resultID {
		if err == nil {
			err = errors.New("persisted worker result ID does not match logical identity")
		}
		return repository.DurableWorkerResult{}, &databaseError{operation: "hydrate", resource: "durable worker result", cause: err}
	}
	return result, nil
}
func mapDurableWorkerResultWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "async_worker_results_pk":
			return repository.NewConflictError(operation, "durable worker result ID", err)
		case "async_worker_results_node_attempt_uk":
			return repository.NewConflictError(operation, "durable worker result node attempt", err)
		}
	}
	return mapPostgreSQLError(operation, "durable worker result", err)
}
