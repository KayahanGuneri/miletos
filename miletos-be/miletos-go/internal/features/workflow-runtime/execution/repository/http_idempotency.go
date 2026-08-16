package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type HTTPIdempotencyState string

const (
	HTTPIdempotencyReserved HTTPIdempotencyState = "RESERVED"
	HTTPIdempotencyAccepted HTTPIdempotencyState = "ACCEPTED"
)

type HTTPIdempotencyReservation struct {
	CompanyID           string               `gorm:"column:company_id"`
	IdempotencyKey      string               `gorm:"column:idempotency_key"`
	RequestFingerprint  string               `gorm:"column:request_fingerprint"`
	WorkflowExecutionID string               `gorm:"column:workflow_execution_id"`
	State               HTTPIdempotencyState `gorm:"column:state"`
	CreatedAt           time.Time            `gorm:"column:created_at"`
	AcceptedAt          *time.Time           `gorm:"column:accepted_at"`
}

func (repository *ExecutionRepository) ReserveHTTPIdempotency(
	ctx context.Context,
	companyID string,
	key string,
	fingerprint string,
) (HTTPIdempotencyReservation, bool, error) {
	reservation := HTTPIdempotencyReservation{
		CompanyID: companyID, IdempotencyKey: key,
		RequestFingerprint: fingerprint, WorkflowExecutionID: newID("exec_"),
		State: HTTPIdempotencyReserved, CreatedAt: time.Now().UTC(),
	}
	err := repository.dbClient.QueryRow(ctx, `
		INSERT INTO workflow_runtime.http_idempotency_keys (
			company_id,
			idempotency_key,
			request_fingerprint,
			workflow_execution_id,
			state,
			created_at,
			accepted_at
		)
		VALUES ($1, $2, $3, $4, 'RESERVED', $5, NULL)
		ON CONFLICT (company_id, idempotency_key) DO NOTHING
		RETURNING company_id, idempotency_key, request_fingerprint,
			workflow_execution_id, state, created_at, accepted_at`,
		reservation.CompanyID,
		reservation.IdempotencyKey,
		reservation.RequestFingerprint,
		reservation.WorkflowExecutionID,
		reservation.CreatedAt,
	).Scan(
		&reservation.CompanyID,
		&reservation.IdempotencyKey,
		&reservation.RequestFingerprint,
		&reservation.WorkflowExecutionID,
		&reservation.State,
		&reservation.CreatedAt,
		&reservation.AcceptedAt,
	)
	if err == nil {
		return reservation, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return HTTPIdempotencyReservation{}, false,
			fmt.Errorf("reserve HTTP idempotency key: %w", err)
	}

	var existing HTTPIdempotencyReservation
	err = repository.dbClient.DB(ctx).
		Table("workflow_runtime.http_idempotency_keys").
		Where("company_id = ? AND idempotency_key = ?", companyID, key).
		Take(&existing).Error
	if err != nil {
		return HTTPIdempotencyReservation{}, false, fmt.Errorf(
			"load concurrent HTTP idempotency reservation: %w",
			mapNoRows(err),
		)
	}
	return existing, false, nil
}

func (repository *ExecutionRepository) MarkHTTPIdempotencyAccepted(
	ctx context.Context,
	companyID string,
	key string,
	fingerprint string,
	executionID string,
) error {
	return markHTTPIdempotencyAccepted(
		ctx, repository.dbClient, companyID, key, fingerprint,
		executionID, time.Now().UTC(),
	)
}

func markHTTPIdempotencyAccepted(
	ctx context.Context,
	executor gormExecutor,
	companyID string,
	key string,
	fingerprint string,
	executionID string,
	acceptedAt time.Time,
) error {
	result := executor.DB(ctx).
		Table("workflow_runtime.http_idempotency_keys").
		Where(
			"company_id = ? AND idempotency_key = ? AND request_fingerprint = ? "+
				"AND workflow_execution_id = ? AND state = ?",
			companyID, key, fingerprint, executionID, HTTPIdempotencyReserved,
		).
		Updates(map[string]any{
			"state":       HTTPIdempotencyAccepted,
			"accepted_at": acceptedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("accept HTTP idempotency key: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var existing HTTPIdempotencyReservation
	err := executor.DB(ctx).
		Table("workflow_runtime.http_idempotency_keys").
		Where("company_id = ? AND idempotency_key = ?", companyID, key).
		Take(&existing).Error
	if err != nil {
		return fmt.Errorf("inspect HTTP idempotency acceptance: %w", mapNoRows(err))
	}
	if existing.RequestFingerprint != fingerprint {
		return ErrIdempotencyConflict
	}
	if existing.WorkflowExecutionID == executionID &&
		existing.State == HTTPIdempotencyAccepted {
		return nil
	}
	return fmt.Errorf(
		"%w: HTTP idempotency reservation cannot be accepted",
		ErrStateTransition,
	)
}
