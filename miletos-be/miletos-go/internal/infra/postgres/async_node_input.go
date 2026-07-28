package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

func (store *Store) CreateAsyncNodeInput(ctx context.Context, input repository.AsyncNodeInput) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	return createAsyncNodeInput(ctx, store.pool, input)
}
func createAsyncNodeInput(ctx context.Context, db asyncPersistenceDatabase, input repository.AsyncNodeInput) error {
	if ctx == nil {
		return fmt.Errorf("create async node input context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || !input.IsValid() {
		return fmt.Errorf("async input database and input must be valid")
	}
	payload, err := encodeRuntimePayload(input.Payload())
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, insertAsyncInputSQL,
		input.IdentityKey(), input.CompanyID().String(), input.WorkflowExecutionID().String(),
		input.TargetNodeExecutionID().String(), input.SourceNodeExecutionID().String(), input.TargetNodeID().String(),
		input.SourceNodeID().String(), input.EdgeID().String(), input.SourceOutputPort(),
		input.TargetInputPort(), input.SourceAttempt(), string(payload),
		input.CreatedAt())
	if err != nil {
		return mapAsyncInputWriteError("create", err)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewConflictError("create", "async node input graph identity",
			errors.New("source or target node identity does not match the workflow execution"))
	}
	return nil
}

const listAsyncInputsSQL = `SELECT ` + asyncInputColumns + `
FROM workflow_runtime.async_node_inputs
WHERE company_id=$1 AND workflow_execution_id=$2 AND target_node_execution_id=$3
ORDER BY edge_id, source_attempt, source_node_execution_id, async_input_id`

func (store *Store) ListAsyncNodeInputs(
	ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	targetNodeExecutionID execution.NodeExecutionID) ([]repository.AsyncNodeInput, error) {
	if !store.IsValid() {
		return nil, fmt.Errorf("PostgreSQL store must be valid")
	}
	return listAsyncNodeInputs(ctx, store.pool, companyID, workflowExecutionID, targetNodeExecutionID)
}
func listAsyncNodeInputs(
	ctx context.Context, db asyncPersistenceDatabase, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, targetNodeExecutionID execution.NodeExecutionID) ([]repository.AsyncNodeInput, error) {
	if ctx == nil {
		return nil, fmt.Errorf("list async node inputs context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, fmt.Errorf("async input database must not be nil")
	}
	validatedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return nil, fmt.Errorf("company ID must be valid")
	}
	validatedExecutionID, err := execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return nil, fmt.Errorf("workflow execution ID must be valid")
	}
	validatedTargetID, err := execution.NewNodeExecutionID(targetNodeExecutionID.String())
	if err != nil {
		return nil, fmt.Errorf("target node execution ID must be valid")
	}
	rows, err := db.Query(ctx, listAsyncInputsSQL, validatedCompanyID.String(), validatedExecutionID.String(), validatedTargetID.String())
	if err != nil {
		return nil, mapPostgreSQLError("list", "async node inputs", err)
	}
	defer rows.Close()
	inputs := make([]repository.AsyncNodeInput, 0)
	for rows.Next() {
		input, err := scanAsyncNodeInput(rows)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgreSQLError("list", "async node inputs", err)
	}
	return inputs, nil
}

func scanAsyncNodeInput(row rowScanner) (repository.AsyncNodeInput, error) {
	var asyncInputID, companyID, workflowExecutionID string
	var targetNodeExecutionID, sourceNodeExecutionID string
	var targetNodeID, sourceNodeID, edgeID string
	var sourceOutputPort, targetInputPort string
	var sourceAttempt int16
	var payload []byte
	var createdAt time.Time
	if err := row.Scan(
		&asyncInputID, &companyID, &workflowExecutionID,
		&targetNodeExecutionID, &sourceNodeExecutionID, &targetNodeID,
		&sourceNodeID, &edgeID, &sourceOutputPort,
		&targetInputPort, &sourceAttempt, &payload,
		&createdAt); err != nil {
		return repository.AsyncNodeInput{}, mapPostgreSQLError("hydrate", "async node input", err)
	}
	decodedPayload, err := decodeRuntimePayload(payload)
	if err != nil {
		return repository.AsyncNodeInput{}, err
	}
	input, err := repository.NewAsyncNodeInput(repository.AsyncNodeInputParams{
		CompanyID: workflow.CompanyID(companyID), WorkflowExecutionID: execution.WorkflowExecutionID(workflowExecutionID),
		TargetNodeExecutionID: execution.NodeExecutionID(targetNodeExecutionID),
		SourceNodeExecutionID: execution.NodeExecutionID(sourceNodeExecutionID), TargetNodeID: workflow.NodeID(targetNodeID), SourceNodeID: workflow.NodeID(sourceNodeID),
		EdgeID: workflow.EdgeID(edgeID), SourceOutputPort: sourceOutputPort, TargetInputPort: targetInputPort,
		SourceAttempt: sourceAttempt, Payload: decodedPayload, CreatedAt: createdAt,
	})
	if err != nil || input.IdentityKey() != asyncInputID {
		if err == nil {
			err = errors.New("persisted async input ID does not match logical identity")
		}
		return repository.AsyncNodeInput{}, &databaseError{operation: "hydrate", resource: "async node input", cause: err}
	}
	return input, nil
}
func mapAsyncInputWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "async_node_inputs_pk":
			return repository.NewConflictError(operation, "async node input ID", err)
		case "async_node_inputs_logical_uk":
			return repository.NewConflictError(operation, "async node input logical identity", err)
		}
	}
	return mapPostgreSQLError(operation, "async node input", err)
}
