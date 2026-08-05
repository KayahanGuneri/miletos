package crontrigger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"miletos-go/internal/features/workflow-runtime/cronexpr"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/database"
)

type Repository struct {
	dbClient *database.Client
}

func NewRepository(dbClient *database.Client) *Repository {
	return &Repository{dbClient: dbClient}
}

func (triggerRepository *Repository) Create(
	ctx context.Context,
	binding Binding,
	definition workflow.Workflow,
) error {
	encodedDefinition, err := json.Marshal(definition)
	if err != nil {
		return fmt.Errorf("encode cron trigger workflow snapshot: %w", err)
	}
	transaction, err := triggerRepository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cron trigger activation: %w", err)
	}
	defer transaction.Rollback(ctx)
	snapshot := map[string]any{
		"snapshot_id": binding.SnapshotID, "company_id": binding.CompanyID,
		"workflow_id": binding.WorkflowID, "workflow_revision": binding.WorkflowRevision,
		"workflow_name": definition.Name, "definition_json": encodedDefinition,
		"created_at": binding.CreatedAt,
	}
	if err := transaction.DB(ctx).
		Table("workflow_runtime.workflow_definition_snapshots").Create(snapshot).Error; err != nil {
		return fmt.Errorf("persist cron trigger workflow snapshot: %w", err)
	}
	record := recordFromBinding(binding)
	if err := transaction.DB(ctx).
		Table("workflow_runtime.cron_trigger_bindings").Create(&record).Error; err != nil {
		return fmt.Errorf("persist cron trigger binding: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit cron trigger activation: %w", err)
	}
	return nil
}

func (triggerRepository *Repository) FindByID(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	var records []bindingRecord
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.cron_trigger_bindings").
		Where("company_id = ? AND trigger_id = ?", companyID, triggerID).
		Limit(1).Find(&records)
	if result.Error != nil {
		return Binding{}, fmt.Errorf("find cron trigger binding: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return Binding{}, repository.ErrNotFound
	}
	return bindingFromRecord(records[0]), nil
}

func (triggerRepository *Repository) FindActiveByWorkflow(
	ctx context.Context,
	companyID string,
	workflowID string,
) (Binding, error) {
	var records []bindingRecord
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.cron_trigger_bindings").
		Where(
			"company_id = ? AND workflow_id = ? AND status = ?",
			companyID, workflowID, StatusActive,
		).
		Limit(1).Find(&records)
	if result.Error != nil {
		return Binding{}, fmt.Errorf("find active cron trigger by workflow: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return Binding{}, repository.ErrNotFound
	}
	return bindingFromRecord(records[0]), nil
}

func (triggerRepository *Repository) Disable(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	now := time.Now().UTC()
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.cron_trigger_bindings").
		Where("company_id = ? AND trigger_id = ? AND status = ?", companyID, triggerID, StatusActive).
		Updates(map[string]any{
			"status": StatusDisabled, "disabled_at": now, "updated_at": now,
			"lock_version": gorm.Expr("lock_version + 1"),
		})
	if result.Error != nil {
		return Binding{}, fmt.Errorf("disable cron trigger: %w", result.Error)
	}
	binding, err := triggerRepository.FindByID(ctx, companyID, triggerID)
	if err != nil {
		return Binding{}, err
	}
	if result.RowsAffected == 0 && binding.Status != StatusDisabled {
		return Binding{}, repository.ErrStateTransition
	}
	return binding, nil
}

func (triggerRepository *Repository) DisableWorkflow(
	ctx context.Context,
	companyID string,
	workflowID string,
) (int, error) {
	now := time.Now().UTC()
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.cron_trigger_bindings").
		Where("company_id = ? AND workflow_id = ? AND status = ?", companyID, workflowID, StatusActive).
		Updates(map[string]any{
			"status": StatusDisabled, "disabled_at": now, "updated_at": now,
			"lock_version": gorm.Expr("lock_version + 1"),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("disable workflow cron triggers: %w", result.Error)
	}
	return int(result.RowsAffected), nil
}

func (triggerRepository *Repository) ClaimDue(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]DueOccurrence, error) {
	if limit <= 0 {
		return nil, nil
	}
	transaction, err := triggerRepository.dbClient.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin cron trigger claim: %w", err)
	}
	defer transaction.Rollback(ctx)
	var records []bindingRecord
	if err := transaction.DB(ctx).Raw(`
		SELECT *
		FROM workflow_runtime.cron_trigger_bindings
		WHERE status = ? AND next_fire_at <= ?
		ORDER BY next_fire_at ASC, trigger_id ASC
		LIMIT ?
		FOR UPDATE SKIP LOCKED`,
		StatusActive, now.UTC(), limit,
	).Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("select due cron triggers: %w", err)
	}
	occurrences := make([]DueOccurrence, 0, len(records))
	for _, record := range records {
		binding := bindingFromRecord(record)
		// Missed occurrences are coalesced: the overdue boundary is claimed once
		// and the schedule jumps straight to the first boundary after the current
		// scheduler time instead of replaying every boundary that elapsed.
		scheduledAt := binding.NextFireAt.UTC()
		advanceFrom := scheduledAt.Add(time.Second)
		if advanceFrom.Before(now.UTC()) {
			advanceFrom = now.UTC()
		}
		nextFireAt, _, err := cronexpr.Next(binding.Expression, binding.Timezone, advanceFrom)
		if err != nil {
			return nil, fmt.Errorf("advance cron trigger %s: %w", binding.ID, err)
		}
		occurrenceID := fmt.Sprintf("cron_occurrence_%s_%d", binding.ID, scheduledAt.UnixNano())
		occurrence := occurrenceRecord{
			OccurrenceID: occurrenceID, TriggerID: binding.ID, CompanyID: binding.CompanyID,
			WorkflowID: binding.WorkflowID, SnapshotID: binding.SnapshotID,
			ScheduledAt: scheduledAt, Status: OccurrencePending, NextAttemptAt: now.UTC(),
			CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
		}
		if err := transaction.DB(ctx).Table("workflow_runtime.cron_trigger_occurrences").Create(&occurrence).Error; err != nil {
			return nil, fmt.Errorf("persist cron occurrence %s: %w", binding.ID, err)
		}
		result, err := transaction.Exec(ctx, `
			UPDATE workflow_runtime.cron_trigger_bindings
			SET next_fire_at = $4,
				last_scheduled_at = $5,
				last_fired_at = $6,
				updated_at = $6,
				lock_version = lock_version + 1
			WHERE company_id = $1
			  AND trigger_id = $2
			  AND status = 'ACTIVE'
			  AND lock_version = $3`,
			binding.CompanyID, binding.ID, binding.LockVersion,
			nextFireAt, scheduledAt, now.UTC(),
		)
		if err != nil {
			return nil, fmt.Errorf("claim cron trigger %s: %w", binding.ID, err)
		}
		if result.RowsAffected() != 1 {
			// Another worker already advanced this binding. Skip it so the rest
			// of the due set in this transaction can still be claimed.
			continue
		}
		binding.NextFireAt = nextFireAt
		binding.LastScheduledAt = &scheduledAt
		firedAt := now.UTC()
		binding.LastFiredAt = &firedAt
		occurrences = append(occurrences, DueOccurrence{
			Binding: binding, OccurrenceID: occurrenceID, ScheduledAt: scheduledAt, FiredAt: firedAt,
		})
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cron trigger claim: %w", err)
	}
	return occurrences, nil
}

func (triggerRepository *Repository) ClaimOccurrences(ctx context.Context, now time.Time, limit int) ([]Occurrence, error) {
	if limit <= 0 {
		return nil, nil
	}
	transaction, err := triggerRepository.dbClient.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin cron occurrence claim: %w", err)
	}
	defer transaction.Rollback(ctx)
	var records []occurrenceRecord
	if err := transaction.DB(ctx).Raw(`
		SELECT * FROM workflow_runtime.cron_trigger_occurrences
		WHERE (status IN ('PENDING', 'FAILED') AND next_attempt_at <= ?)
		   OR (status = 'PROCESSING' AND updated_at <= ?)
		ORDER BY next_attempt_at ASC, occurrence_id ASC LIMIT ? FOR UPDATE SKIP LOCKED`, now.UTC(), now.UTC().Add(-5*time.Minute), limit).Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("select pending cron occurrences: %w", err)
	}
	occurrences := make([]Occurrence, 0, len(records))
	for _, record := range records {
		result, err := transaction.Exec(ctx, `UPDATE workflow_runtime.cron_trigger_occurrences
			SET status = 'PROCESSING', attempt_count = attempt_count + 1, updated_at = $3, lock_version = lock_version + 1
			WHERE occurrence_id = $1 AND lock_version = $2`, record.OccurrenceID, record.LockVersion, now.UTC())
		if err != nil {
			return nil, fmt.Errorf("claim cron occurrence %s: %w", record.OccurrenceID, err)
		}
		if result.RowsAffected() == 1 {
			record.Status = OccurrenceProcessing
			record.AttemptCount++
			record.UpdatedAt = now.UTC()
			record.LockVersion++
			occurrences = append(occurrences, occurrenceFromRecord(record))
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cron occurrence claim: %w", err)
	}
	return occurrences, nil
}

func (triggerRepository *Repository) MarkOccurrenceSucceeded(ctx context.Context, occurrenceID, executionID string) error {
	now := time.Now().UTC()
	return triggerRepository.dbClient.DB(ctx).Exec(`UPDATE workflow_runtime.cron_trigger_occurrences
		SET status = 'SUCCEEDED', execution_id = $2, failure_code = NULL, failure_message = NULL, updated_at = $3, lock_version = lock_version + 1
		WHERE occurrence_id = $1`, occurrenceID, executionID, now).Error
}

func (triggerRepository *Repository) MarkOccurrenceFailed(ctx context.Context, occurrenceID string, attempt int, failureCode, failureMessage string) error {
	now := time.Now().UTC()
	retryAt := now.Add(time.Duration(min(attempt, 10)) * time.Minute)
	return triggerRepository.dbClient.DB(ctx).Exec(`UPDATE workflow_runtime.cron_trigger_occurrences
		SET status = 'FAILED', failure_code = $2, failure_message = $3, next_attempt_at = $4, updated_at = $5, lock_version = lock_version + 1
		WHERE occurrence_id = $1`, occurrenceID, failureCode, failureMessage, retryAt, now).Error
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
