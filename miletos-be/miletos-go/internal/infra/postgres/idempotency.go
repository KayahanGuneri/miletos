package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

var _ repository.HTTPIdempotencyStore = (*Store)(nil)
var _ repository.HTTPIdempotencyAcceptanceEvidenceStore = (*Store)(nil)

const httpIdempotencyColumns = `
company_id,
idempotency_key,
request_fingerprint,
workflow_execution_id,
state,
created_at,
accepted_at
`
const insertHTTPIdempotencyReservationSQL = `
INSERT INTO workflow_runtime.http_idempotency_keys (
	company_id,
	idempotency_key,
	request_fingerprint,
	workflow_execution_id,
	state,
	created_at,
	accepted_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	'RESERVED',
	$5,
	NULL
)
ON CONFLICT (
	company_id,
	idempotency_key
)
DO NOTHING
RETURNING
	` +
	httpIdempotencyColumns
const selectHTTPIdempotencySQL = `
SELECT
	` +
	httpIdempotencyColumns + `
FROM workflow_runtime.http_idempotency_keys
WHERE company_id = $1
  AND idempotency_key = $2
`
const acceptHTTPIdempotencySQL = `
UPDATE workflow_runtime.http_idempotency_keys
SET
	state = 'ACCEPTED',
	accepted_at = $5
WHERE company_id = $1
  AND idempotency_key = $2
  AND request_fingerprint = $3
  AND workflow_execution_id = $4
  AND state = 'RESERVED'
RETURNING
	` +
	httpIdempotencyColumns
const hasHTTPIdempotencyAcceptanceEvidenceSQL = `
SELECT (
	EXISTS (
		SELECT 1
		FROM workflow_runtime.outbox_messages
		WHERE company_id = $1
		  AND workflow_execution_id = $2
		  AND operation_kind = 'NODE_COMMAND'
	)
	OR
	EXISTS (
		SELECT 1
		FROM workflow_runtime.workflow_executions
		WHERE company_id = $1
		  AND workflow_execution_id = $2
		  AND status = 'REJECTED'
	)
)
`

func (store *Store) ReserveHTTPIdempotency(
	ctx context.Context, reservation repository.HTTPIdempotencyReservation) (
	repository.HTTPIdempotencyRecord, bool, error,
) {
	if !store.IsValid() {
		return repository.HTTPIdempotencyRecord{},
			false, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.HTTPIdempotencyRecord{}, false,
			fmt.Errorf("reserve HTTP idempotency context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.HTTPIdempotencyRecord{}, false, err
	}
	if !reservation.IsValid() {
		return repository.HTTPIdempotencyRecord{}, false, fmt.Errorf(
			"HTTP idempotency reservation must be valid")
	}
	requested := reservation.Record()
	record, err := scanHTTPIdempotencyRecord(store.pool.QueryRow(
		ctx, insertHTTPIdempotencyReservationSQL, requested.CompanyID().String(),
		requested.IdempotencyKey(), requested.RequestFingerprint(), requested.WorkflowExecutionID().String(),
		requested.CreatedAt()))
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(
		err, pgx.ErrNoRows) {
		return repository.HTTPIdempotencyRecord{}, false, mapHTTPIdempotencyWriteError(
			"reserve", err)
	}
	existing, err :=
		getHTTPIdempotencyRecord(ctx, store,
			requested.CompanyID(), requested.IdempotencyKey())
	if err != nil {
		return repository.HTTPIdempotencyRecord{}, false,
			err
	}
	return existing, false, nil
}
func (store *Store) MarkHTTPIdempotencyAccepted(
	ctx context.Context, acceptance repository.HTTPIdempotencyAcceptance) (
	repository.HTTPIdempotencyRecord, error) {
	if !store.IsValid() {
		return repository.HTTPIdempotencyRecord{}, fmt.Errorf(
			"PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.HTTPIdempotencyRecord{},
			fmt.Errorf("accept HTTP idempotency context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.HTTPIdempotencyRecord{}, err
	}
	if !acceptance.IsValid() {
		return repository.HTTPIdempotencyRecord{},
			fmt.Errorf("HTTP idempotency acceptance must be valid")
	}
	record, err :=
		scanHTTPIdempotencyRecord(store.pool.QueryRow(ctx,
			acceptHTTPIdempotencySQL, acceptance.CompanyID().String(), acceptance.IdempotencyKey(),
			acceptance.RequestFingerprint(), acceptance.WorkflowExecutionID().String(), acceptance.AcceptedAt(),
		))
	if err == nil {
		return record, nil
	}
	if !errors.Is(err,
		pgx.ErrNoRows) {
		return repository.HTTPIdempotencyRecord{},
			mapPostgreSQLError("accept", "HTTP idempotency key",
				err)
	}
	existing, err := getHTTPIdempotencyRecord(
		ctx, store, acceptance.CompanyID(),
		acceptance.IdempotencyKey())
	if err != nil {
		return repository.HTTPIdempotencyRecord{}, err
	}
	if existing.RequestFingerprint() != acceptance.RequestFingerprint() {
		return repository.HTTPIdempotencyRecord{}, repository.NewConflictError("accept",
			"HTTP idempotency key", errors.New("request fingerprint does not match"))
	}
	if existing.WorkflowExecutionID() != acceptance.WorkflowExecutionID() {
		return repository.HTTPIdempotencyRecord{}, repository.NewConflictError("accept",
			"HTTP idempotency key", errors.New("workflow execution identity does not match"))
	}
	if existing.State() == repository.HTTPIdempotencyStateAccepted {
		return existing, nil
	}
	return repository.HTTPIdempotencyRecord{}, repository.NewStaleWriteError("accept",
		"HTTP idempotency key", errors.New("idempotency reservation state changed unexpectedly"))
}
func getHTTPIdempotencyRecord(ctx context.Context,
	store *Store, companyID workflow.CompanyID, idempotencyKey string,
) (repository.HTTPIdempotencyRecord, error,
) {
	if !store.IsValid() {
		return repository.HTTPIdempotencyRecord{},
			fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.HTTPIdempotencyRecord{}, fmt.Errorf("get HTTP idempotency context must not be nil")
	}
	record, err := scanHTTPIdempotencyRecord(store.pool.QueryRow(
		ctx, selectHTTPIdempotencySQL, companyID.String(),
		idempotencyKey))
	if err != nil {
		return repository.HTTPIdempotencyRecord{},
			mapPostgreSQLError("get", "HTTP idempotency key",
				err)
	}
	return record, nil
}

func scanHTTPIdempotencyRecord(row rowScanner,
) (repository.HTTPIdempotencyRecord, error,
) {
	if row == nil {
		return repository.HTTPIdempotencyRecord{},
			fmt.Errorf("HTTP idempotency row must not be nil")
	}
	var companyID string
	var idempotencyKey string
	var requestFingerprint string
	var workflowExecutionID string
	var state string
	var createdAt time.Time
	var acceptedAt *time.Time
	err := row.Scan(&companyID,
		&idempotencyKey, &requestFingerprint, &workflowExecutionID,
		&state, &createdAt, &acceptedAt,
	)
	if err != nil {
		return repository.HTTPIdempotencyRecord{},
			err
	}
	params := repository.HTTPIdempotencyRecordParams{CompanyID: workflow.CompanyID(
		companyID), IdempotencyKey: idempotencyKey,
		RequestFingerprint: requestFingerprint, WorkflowExecutionID: execution.WorkflowExecutionID(workflowExecutionID), State: repository.HTTPIdempotencyState(state),
		CreatedAt: createdAt}
	if acceptedAt != nil {
		params.AcceptedAt =
			acceptedAt.UTC()
	}
	record, err := repository.NewHTTPIdempotencyRecord(params)
	if err != nil {
		return repository.HTTPIdempotencyRecord{},
			fmt.Errorf("hydrate HTTP idempotency record: %w", err)
	}
	return record, nil
}
func mapHTTPIdempotencyWriteError(operation string, err error,
) error {
	if err == nil {
		return nil
	}
	var postgreSQLError *pgconn.PgError
	if errors.As(err,
		&postgreSQLError) {
		switch postgreSQLError.ConstraintName {
		case "http_idempotency_keys_pk":
			return repository.NewConflictError(operation,
				"HTTP idempotency key", err)
		case "http_idempotency_keys_execution_uk":
			return repository.NewConflictError(
				operation, "HTTP idempotency execution", err,
			)
		}
	}
	return mapPostgreSQLError(operation,
		"HTTP idempotency key", err)
}
func (
	store *Store) HasHTTPIdempotencyAcceptanceEvidence(ctx context.Context,
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID) (
	bool, error) {
	if !store.IsValid() {
		return false, fmt.Errorf(
			"PostgreSQL store must be valid")
	}
	if ctx == nil {
		return false,
			fmt.Errorf("HTTP idempotency acceptance evidence context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(
		companyID.String())
	if err != nil {
		return false, fmt.Errorf(
			"company ID must be valid: %w", err)
	}
	normalizedExecutionID, err :=
		execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return false,
			fmt.Errorf("workflow execution ID must be valid: %w", err)
	}
	var exists bool
	err =
		store.pool.QueryRow(
			ctx, hasHTTPIdempotencyAcceptanceEvidenceSQL, normalizedCompanyID.String(),
			normalizedExecutionID.String()).Scan(
			&exists)
	if err != nil {
		return false, mapPostgreSQLError(
			"check", "HTTP idempotency acceptance evidence", err,
		)
	}
	return exists, nil
}
