package postgres

import (
	"context"
	"fmt"

	pgx "github.com/jackc/pgx/v5"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

var _ repository.AsyncPersistenceTransactor = (*Store)(nil)
var _ repository.AsyncPersistenceTransaction = (*asyncPersistenceTransaction)(nil)

type asyncPersistenceTransaction struct {
	tx pgx.Tx
}

func (transaction *asyncPersistenceTransaction) ScheduleAsyncNodeExecution(ctx context.Context, schedule repository.AsyncNodeSchedule) error {
	return scheduleAsyncNodeExecution(ctx, transaction.tx, schedule)
}
func (transaction *asyncPersistenceTransaction) CreateOutboxMessage(ctx context.Context, message repository.OutboxMessage) error {
	return createOutboxMessage(ctx, transaction.tx, message)
}
func (transaction *asyncPersistenceTransaction) ClaimPublishableOutboxMessages(ctx context.Context, request repository.OutboxClaimRequest) ([]repository.OutboxMessage, error) {
	return claimPublishableOutboxMessages(ctx, transaction.tx, request)
}
func (transaction *asyncPersistenceTransaction) MarkOutboxMessagePublished(ctx context.Context, command repository.MarkOutboxPublishedCommand) error {
	return markOutboxMessagePublished(ctx, transaction.tx, command)
}
func (transaction *asyncPersistenceTransaction) ReleaseStaleOutboxClaims(ctx context.Context, request repository.ReleaseStaleOutboxClaimsRequest) (int64, error) {
	return releaseStaleOutboxClaims(ctx, transaction.tx, request)
}
func (transaction *asyncPersistenceTransaction) HasProcessedInboxMessage(ctx context.Context, consumer repository.ConsumerIdentity, messageID repository.MessageID) (bool, error) {
	return hasProcessedInboxMessage(ctx, transaction.tx, consumer, messageID)
}
func (transaction *asyncPersistenceTransaction) RecordInboxMessage(ctx context.Context, message repository.InboxMessage) error {
	return recordInboxMessage(ctx, transaction.tx, message)
}
func (transaction *asyncPersistenceTransaction) CreateAsyncNodeInput(ctx context.Context, input repository.AsyncNodeInput) error {
	return createAsyncNodeInput(ctx, transaction.tx, input)
}
func (transaction *asyncPersistenceTransaction) ListAsyncNodeInputs(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	targetNodeExecutionID execution.NodeExecutionID) ([]repository.AsyncNodeInput, error) {
	return listAsyncNodeInputs(ctx, transaction.tx, companyID, workflowExecutionID, targetNodeExecutionID)
}
func (transaction *asyncPersistenceTransaction) CreateDurableWorkerResult(ctx context.Context, result repository.DurableWorkerResult) error {
	return createDurableWorkerResult(ctx, transaction.tx, result)
}
func (transaction *asyncPersistenceTransaction) GetDurableWorkerResult(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID, attempt int16) (repository.DurableWorkerResult, error) {
	return getDurableWorkerResult(ctx, transaction.tx, companyID, workflowExecutionID, nodeExecutionID, attempt)
}
func (transaction *asyncPersistenceTransaction) GetAsyncContextVariable(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	key string) (repository.AsyncContextVariable, error) {
	return getAsyncContextVariable(ctx, transaction.tx, companyID, workflowExecutionID, key)
}
func (transaction *asyncPersistenceTransaction) ListAsyncContextVariables(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) ([]repository.AsyncContextVariable, error) {
	return listAsyncContextVariables(ctx, transaction.tx, companyID, workflowExecutionID)
}
func (transaction *asyncPersistenceTransaction) CompareAndSwapAsyncContextVariable(ctx context.Context, write repository.AsyncContextWrite) error {
	return compareAndSwapAsyncContextVariable(ctx, transaction.tx, write)
}
func (transaction *asyncPersistenceTransaction) DeleteAsyncContextVariable(ctx context.Context, deletion repository.AsyncContextDelete) error {
	return deleteAsyncContextVariable(ctx, transaction.tx, deletion)
}
func (transaction *asyncPersistenceTransaction) ClaimAsyncNodeExecution(ctx context.Context, claim repository.AsyncNodeClaim) (int64, error) {
	return claimAsyncNodeExecution(ctx, transaction.tx, claim)
}
func (transaction *asyncPersistenceTransaction) PublishAsyncWorkerResult(
	ctx context.Context,
	publication repository.AsyncWorkerResultPublication,
) error {
	return publishAsyncWorkerResult(
		ctx, transaction.tx, publication,
	)
}
func (transaction *asyncPersistenceTransaction) ApplyAsyncNodeResult(
	ctx context.Context,
	application repository.AsyncNodeResultApplication,
) error {
	return applyAsyncNodeResult(
		ctx, transaction.tx, application,
	)
}
func (store *Store) WithinAsyncPersistenceTransaction(ctx context.Context, work repository.AsyncPersistenceWork) error {
	if work == nil {
		return fmt.Errorf("async persistence transaction work must not be nil")
	}
	return store.withinTransaction(ctx, "async persistence", func(tx pgx.Tx) error { return work(ctx, &asyncPersistenceTransaction{tx: tx}) })
}
