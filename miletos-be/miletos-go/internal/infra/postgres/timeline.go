package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

func nextSequenceAfterTimeline(start repository.SequenceNumber,
	timelineLength int) (repository.SequenceNumber, error) {
	if !start.IsValid() {
		return 0, fmt.Errorf("timeline start sequence must be valid")
	}
	if timelineLength <= 0 {
		return 0, fmt.Errorf("timeline must contain at least one entry")
	}
	increment := int64(timelineLength)
	if start.Int64() > math.MaxInt64-increment {
		return 0, fmt.Errorf("timeline sequence allocation exceeds PostgreSQL BIGINT capacity")
	}
	return repository.NewSequenceNumber(
		start.Int64() + increment)
}

const requireWorkflowExecutionScopeSQL = `
SELECT 1
FROM workflow_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`

func (store *Store,
) requireWorkflowExecutionScope(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) error {
	var marker int
	err := store.pool.QueryRow(ctx,
		requireWorkflowExecutionScopeSQL, companyID.String(), workflowExecutionID.String(),
	).Scan(&marker)
	if err != nil {
		return mapPostgreSQLError(
			"get", "workflow execution", err,
		)
	}
	return nil
}

func insertTimelineEntriesTransaction(ctx context.Context, tx pgx.Tx,
	startSequence repository.SequenceNumber, entries []repository.TimelineEntry) (repository.SequenceNumber, error) {
	if tx == nil {
		return 0, fmt.Errorf("timeline transaction must not be nil")
	}
	nextSequence, err := nextSequenceAfterTimeline(startSequence, len(entries))
	if err != nil {
		return 0, err
	}
	sequence := startSequence
	for _, entry := range entries {
		switch entry.Kind() {
		case repository.TimelineEntryKindEvent:
			draft, exists := entry.Event()
			if !exists {
				return 0, fmt.Errorf("event timeline entry does not contain an event draft")
			}
			record, err := draft.Record(sequence)
			if err != nil {
				return 0, fmt.Errorf("materialize execution event: %w",
					err)
			}
			if err := insertExecutionEventTransaction(ctx,
				tx, record); err != nil {
				return 0, err
			}
		case repository.TimelineEntryKindLog:
			draft, exists := entry.Log()
			if !exists {
				return 0, fmt.Errorf("log timeline entry does not contain a log draft")
			}
			record, err := draft.Record(sequence)
			if err != nil {
				return 0, fmt.Errorf("materialize execution log: %w",
					err)
			}
			if err := insertExecutionLogTransaction(ctx,
				tx, record); err != nil {
				return 0, err
			}
		default:
			return 0, fmt.Errorf("timeline entry kind %q is unsupported",
				entry.Kind())
		}
		sequence = repository.SequenceNumber(sequence.Int64() + 1)
	}
	if sequence != nextSequence {
		return 0, fmt.Errorf("timeline sequence allocation is inconsistent")
	}
	return nextSequence, nil
}

const transactionRollbackTimeout = 5 * time.Second
