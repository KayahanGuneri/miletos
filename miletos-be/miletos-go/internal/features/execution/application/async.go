package executionfeature

import (
	"context"
	"errors"
	"fmt"
	"miletos-go/internal/engine"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
	"time"
)

var (
	ErrIdempotencyKeyReused         = errors.New("idempotency key was reused for a different request")
	ErrIdempotencyRequestInProgress = errors.New(
		"idempotent request is still reserved")
)

type AsyncExecutionApplication interface {
	ExecuteAsync(
		context.Context, engine.ExecutionRequest, string,
		string) (ExecutionOutcome,
		bool, error)
}

type AsyncExecutionPersistence interface {
	repository.HTTPIdempotencyStore
	repository.HTTPIdempotencyAcceptanceEvidenceStore
	GetWorkflowExecution(context.Context, workflow.CompanyID,
		execution.WorkflowExecutionID) (repository.WorkflowExecutionRecord,
		error)
}

type IdempotentExecutionApplication struct {
	delegate    ExecutionApplication
	persistence AsyncExecutionPersistence
}

func NewIdempotentExecutionApplication(delegate ExecutionApplication, persistence AsyncExecutionPersistence,
) (IdempotentExecutionApplication, error,
) {
	if delegate == nil {
		return IdempotentExecutionApplication{},
			fmt.Errorf("execution application must not be nil")
	}
	if persistence == nil {
		return IdempotentExecutionApplication{}, fmt.Errorf("async execution persistence must not be nil")
	}
	return IdempotentExecutionApplication{delegate: delegate, persistence: persistence}, nil
}

func (application IdempotentExecutionApplication) Execute(
	ctx context.Context, request engine.ExecutionRequest) (
	ExecutionOutcome, error) {
	return application.delegate.Execute(ctx, request)
}

func (application IdempotentExecutionApplication) AsyncEnabled() bool {
	return application.delegate != nil && application.persistence != nil && application.delegate.AsyncEnabled()
}

func (
	application IdempotentExecutionApplication) ExecuteAsync(ctx context.Context,
	request engine.ExecutionRequest, idempotencyKey string, requestFingerprint string,
) (ExecutionOutcome, bool,
	error) {
	if ctx == nil {
		return ExecutionOutcome{}, false, fmt.Errorf(
			"async execution context must not be nil")
	}
	if !request.IsValid() || request.Mode() !=
		execution.ExecutionModeAsync {
		return ExecutionOutcome{},
			false, fmt.Errorf("async execution request must be valid")
	}
	if !application.AsyncEnabled() {
		return ExecutionOutcome{}, false,
			engine.ErrAsyncExecutionUnavailable
	}
	reservation, err := repository.NewHTTPIdempotencyReservation(repository.HTTPIdempotencyReservationParams{
		CompanyID:          request.Definition().CompanyID(),
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: requestFingerprint, WorkflowExecutionID: request.WorkflowExecutionID(),
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return ExecutionOutcome{}, false,
			fmt.Errorf("create HTTP idempotency reservation: %w", err)
	}
	requested := reservation.Record()
	record, created, err := application.persistence.
		ReserveHTTPIdempotency(ctx, reservation)
	if err != nil {
		return ExecutionOutcome{}, false, fmt.Errorf(
			"reserve HTTP idempotency key: %w", err)
	}
	if !created {
		if record.RequestFingerprint() != requested.RequestFingerprint() {
			return ExecutionOutcome{}, false, ErrIdempotencyKeyReused
		}
		return application.
			replayHTTPIdempotency(ctx, record)
	}
	outcome, err := application.delegate.
		Execute(ctx, request)
	if err != nil {
		return outcome, false, err
	}
	if outcome.ExecutionID !=
		request.WorkflowExecutionID().String() {
		return ExecutionOutcome{}, false,
			fmt.Errorf("async execution result identity does not match reserved execution")
	}
	if _, err :=
		application.acceptHTTPIdempotency(ctx,
			record); err != nil {
		return outcome, false, err
	}
	return outcome,
		false, nil
}

func (application IdempotentExecutionApplication,
) replayHTTPIdempotency(ctx context.Context, record repository.HTTPIdempotencyRecord,
) (ExecutionOutcome, bool,
	error) {
	if record.State() ==
		repository.HTTPIdempotencyStateReserved {
		hasEvidence, err :=
			application.persistence.HasHTTPIdempotencyAcceptanceEvidence(
				ctx, record.CompanyID(), record.WorkflowExecutionID(),
			)
		if err != nil {
			return ExecutionOutcome{}, false, fmt.Errorf(
				"check HTTP idempotency acceptance evidence: %w", err)
		}
		if !hasEvidence {
			return ExecutionOutcome{}, false, ErrIdempotencyRequestInProgress
		}
		acceptedRecord, err :=
			application.acceptHTTPIdempotency(ctx,
				record)
		if err != nil {
			return ExecutionOutcome{}, false,
				err
		}
		record = acceptedRecord
	}
	if record.State() != repository.HTTPIdempotencyStateAccepted {
		return ExecutionOutcome{}, false,
			fmt.Errorf("HTTP idempotency record has unsupported state %q", record.State())
	}
	persistedExecution, err := application.persistence.
		GetWorkflowExecution(ctx, record.CompanyID(),
			record.WorkflowExecutionID())
	if err != nil {
		return ExecutionOutcome{}, false,
			fmt.Errorf("load replayed workflow execution: %w", err)
	}
	return newExecutionOutcomeFromRecord(persistedExecution),
		true, nil
}

func (application IdempotentExecutionApplication,
) acceptHTTPIdempotency(ctx context.Context, record repository.HTTPIdempotencyRecord,
) (repository.HTTPIdempotencyRecord, error,
) {
	acceptance, err := repository.NewHTTPIdempotencyAcceptance(
		repository.HTTPIdempotencyAcceptanceParams{CompanyID: record.CompanyID(),
			IdempotencyKey: record.IdempotencyKey(), RequestFingerprint: record.RequestFingerprint(),
			WorkflowExecutionID: record.WorkflowExecutionID(),
			AcceptedAt:          time.Now().UTC()},
	)
	if err != nil {
		return repository.HTTPIdempotencyRecord{}, fmt.Errorf("create HTTP idempotency acceptance: %w",
			err)
	}
	acceptedRecord, err := application.
		persistence.MarkHTTPIdempotencyAccepted(ctx,
		acceptance)
	if err != nil {
		return repository.HTTPIdempotencyRecord{}, fmt.Errorf(
			"mark HTTP idempotency accepted: %w", err)
	}
	return acceptedRecord,
		nil
}

func newExecutionOutcomeFromRecord(record repository.WorkflowExecutionRecord) ExecutionOutcome {
	var startedAt *time.Time
	var finishedAt *time.Time
	if value, exists := record.StartedAt(); exists {
		copied := value.UTC()
		startedAt = &copied
	}
	if value, exists := record.FinishedAt(); exists {
		copied := value.UTC()
		finishedAt = &copied
	}
	correlationID, _ :=
		record.CorrelationID()
	return ExecutionOutcome{
		ExecutionID: record.ID().String(),
		WorkflowID: record.WorkflowID().
			String(), WorkflowRevision: record.WorkflowRevision(),
		Mode: record.Mode().
			String(), Status: record.
			Status().String(),
		CorrelationID: correlationID, CreatedAt: record.
				CreatedAt().UTC(),
		StartedAt: startedAt, FinishedAt: finishedAt,
		Rejected: record.Status() == execution.
			WorkflowExecutionStatusRejected}
}
