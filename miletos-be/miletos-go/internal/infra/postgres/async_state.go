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

var _ repository.AsyncContextStore = (*Store)(nil)

const asyncContextColumns = `company_id, workflow_execution_id, variable_key, value_json, created_at, updated_at, version`
const insertAsyncContextSQL = `INSERT INTO workflow_runtime.async_context_variables (` + asyncContextColumns + `)
VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7)`
const updateAsyncContextSQL = `WITH target AS MATERIALIZED (
 SELECT version FROM workflow_runtime.async_context_variables
 WHERE company_id=$1 AND workflow_execution_id=$2 AND variable_key=$3
), updated AS (
 UPDATE workflow_runtime.async_context_variables
 SET value_json=$4::jsonb, updated_at=$5, version=$6
 WHERE company_id=$1 AND workflow_execution_id=$2 AND variable_key=$3 AND version=$7
 RETURNING version
) SELECT EXISTS(SELECT 1 FROM target), EXISTS(SELECT 1 FROM updated)`

func (store *Store) CompareAndSwapAsyncContextVariable(ctx context.Context, write repository.AsyncContextWrite) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	return compareAndSwapAsyncContextVariable(ctx, store.pool, write)
}
func compareAndSwapAsyncContextVariable(ctx context.Context, db asyncPersistenceDatabase, write repository.AsyncContextWrite) error {
	if ctx == nil {
		return fmt.Errorf("compare and swap async context context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || !write.IsValid() {
		return fmt.Errorf("async context database and write must be valid")
	}
	variable := write.Variable()
	if write.IsInsert() {
		_, err := db.Exec(ctx, insertAsyncContextSQL,
			variable.CompanyID().String(), variable.WorkflowExecutionID().String(), variable.Key(), string(variable.Value().Bytes()), variable.CreatedAt(), variable.UpdatedAt(),
			variable.Version())
		if err != nil {
			return mapAsyncContextWriteError("insert", err)
		}
		return nil
	}
	var found, updated bool
	err := db.QueryRow(ctx, updateAsyncContextSQL,
		variable.CompanyID().String(), variable.WorkflowExecutionID().String(), variable.Key(), string(variable.Value().Bytes()), variable.UpdatedAt(), variable.Version(),
		write.ExpectedVersion()).Scan(&found, &updated)
	if err != nil {
		return mapPostgreSQLError("update", "async context variable", err)
	}
	if !found {
		return repository.NewNotFoundError("update", "async context variable", errors.New("async context variable does not exist"))
	}
	if !updated {
		return repository.NewStaleWriteError("update", "async context variable", errors.New("async context version does not match"))
	}
	return nil
}

const getAsyncContextSQL = `SELECT ` + asyncContextColumns + `
FROM workflow_runtime.async_context_variables
WHERE company_id=$1 AND workflow_execution_id=$2 AND variable_key=$3`
const listAsyncContextSQL = `SELECT ` + asyncContextColumns + `
FROM workflow_runtime.async_context_variables
WHERE company_id=$1 AND workflow_execution_id=$2
ORDER BY variable_key`

func (store *Store) ListAsyncContextVariables(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) ([]repository.AsyncContextVariable, error) {
	if !store.IsValid() {
		return nil, fmt.Errorf("PostgreSQL store must be valid")
	}
	return listAsyncContextVariables(ctx, store.pool, companyID, workflowExecutionID)
}
func listAsyncContextVariables(ctx context.Context, db asyncPersistenceDatabase, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) ([]repository.AsyncContextVariable, error) {
	if ctx == nil || db == nil {
		return nil, fmt.Errorf("list async context dependencies must be valid")
	}
	validatedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return nil, fmt.Errorf("company ID must be valid")
	}
	validatedExecutionID, err := execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return nil, fmt.Errorf("workflow execution ID must be valid")
	}
	rows, err := db.Query(ctx, listAsyncContextSQL, validatedCompanyID.String(), validatedExecutionID.String())
	if err != nil {
		return nil, mapPostgreSQLError("list", "async context variables", err)
	}
	defer rows.Close()
	variables := make([]repository.AsyncContextVariable, 0)
	for rows.Next() {
		variable, err := scanAsyncContextVariable(rows)
		if err != nil {
			return nil, err
		}
		variables = append(variables, variable)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgreSQLError("list", "async context variables", err)
	}
	return variables, nil
}
func (store *Store) GetAsyncContextVariable(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	key string) (repository.AsyncContextVariable, error) {
	if !store.IsValid() {
		return repository.AsyncContextVariable{}, fmt.Errorf("PostgreSQL store must be valid")
	}
	return getAsyncContextVariable(ctx, store.pool, companyID, workflowExecutionID, key)
}
func getAsyncContextVariable(ctx context.Context, db asyncPersistenceDatabase, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	key string) (repository.AsyncContextVariable, error) {
	if ctx == nil {
		return repository.AsyncContextVariable{}, fmt.Errorf("get async context context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.AsyncContextVariable{}, err
	}
	if db == nil {
		return repository.AsyncContextVariable{}, fmt.Errorf("async context database must not be nil")
	}
	validatedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return repository.AsyncContextVariable{}, fmt.Errorf("company ID must be valid")
	}
	validatedExecutionID, err := execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return repository.AsyncContextVariable{}, fmt.Errorf("workflow execution ID must be valid")
	}
	placeholder, err := runtime.NewRuntimeValue([]byte("null"))
	if err != nil {
		return repository.AsyncContextVariable{}, err
	}
	validated, err := repository.NewAsyncContextVariable(repository.AsyncContextVariableParams{CompanyID: validatedCompanyID, WorkflowExecutionID: validatedExecutionID, Key: key,
		Value: placeholder, CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(), Version: 1})
	if err != nil {
		return repository.AsyncContextVariable{}, fmt.Errorf("async context key must be valid")
	}
	return scanAsyncContextVariable(db.QueryRow(ctx, getAsyncContextSQL, validatedCompanyID.String(), validatedExecutionID.String(), validated.Key()))
}

func scanAsyncContextVariable(row rowScanner) (repository.AsyncContextVariable, error) {
	var companyID, workflowExecutionID, key string
	var value []byte
	var createdAt, updatedAt time.Time
	var version int64
	if err := row.Scan(&companyID, &workflowExecutionID, &key, &value, &createdAt, &updatedAt, &version); err != nil {
		return repository.AsyncContextVariable{}, mapPostgreSQLError("hydrate", "async context variable", err)
	}
	runtimeValue, err := runtime.NewRuntimeValue(value)
	if err != nil {
		return repository.AsyncContextVariable{}, &databaseError{operation: "hydrate", resource: "async context variable", cause: err}
	}
	variable, err := repository.NewAsyncContextVariable(repository.AsyncContextVariableParams{CompanyID: workflow.CompanyID(companyID),
		WorkflowExecutionID: execution.WorkflowExecutionID(workflowExecutionID), Key: key, Value: runtimeValue, CreatedAt: createdAt, UpdatedAt: updatedAt, Version: version})
	if err != nil {
		return repository.AsyncContextVariable{}, &databaseError{operation: "hydrate", resource: "async context variable", cause: err}
	}
	return variable, nil
}

const deleteAsyncContextSQL = `WITH target AS MATERIALIZED (
 SELECT version FROM workflow_runtime.async_context_variables
 WHERE company_id=$1 AND workflow_execution_id=$2 AND variable_key=$3
), deleted AS (
 DELETE FROM workflow_runtime.async_context_variables
 WHERE company_id=$1 AND workflow_execution_id=$2 AND variable_key=$3 AND version=$4
 RETURNING version
) SELECT EXISTS(SELECT 1 FROM target), EXISTS(SELECT 1 FROM deleted)`

func (store *Store) DeleteAsyncContextVariable(ctx context.Context, deletion repository.AsyncContextDelete) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	return deleteAsyncContextVariable(ctx, store.pool, deletion)
}
func deleteAsyncContextVariable(ctx context.Context, db asyncPersistenceDatabase, deletion repository.AsyncContextDelete) error {
	if ctx == nil {
		return fmt.Errorf("delete async context context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || !deletion.IsValid() {
		return fmt.Errorf("async context database and deletion must be valid")
	}
	var found, deleted bool
	err := db.QueryRow(ctx, deleteAsyncContextSQL, deletion.CompanyID().String(), deletion.WorkflowExecutionID().String(), deletion.Key(), deletion.ExpectedVersion()).Scan(&found,
		&deleted)
	if err != nil {
		return mapPostgreSQLError("delete", "async context variable", err)
	}
	if !found {
		return repository.NewNotFoundError("delete", "async context variable", errors.New("async context variable does not exist"))
	}
	if !deleted {
		return repository.NewStaleWriteError("delete", "async context variable", errors.New("async context version does not match"))
	}
	return nil
}
func mapAsyncContextWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "async_context_variables_pk" {
		return repository.NewConflictError(operation, "async context variable identity", err)
	}
	return mapPostgreSQLError(operation, "async context variable", err)
}
