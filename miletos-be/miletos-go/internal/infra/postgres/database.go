package postgres

import (
	"context"
	"errors"
	"fmt"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"miletos-go/internal/config"
	repository "miletos-go/internal/ports/persistence"
	"net/url"
	"strings"
	"time"
)

type rowScanner interface{ Scan(...any) error }

const (
	postgreSQLUniqueViolationCode     = "23505"
	postgreSQLForeignKeyViolationCode = "23503"
	postgreSQLCheckViolationCode      = "23514"
)

var knownPostgreSQLConstraints = map[string]map[string]struct{}{postgreSQLUniqueViolationCode: constraintSet(
	"workflow_snapshots_pk", "workflow_snapshots_tenant_uk", "workflow_executions_pk",
	"workflow_executions_tenant_uk", "node_executions_pk", "node_exec_identity_uk",
	"node_exec_attempt_uk", "execution_events_pk", "execution_events_identity_uk",
	"execution_events_sequence_uk", "execution_logs_pk", "execution_logs_sequence_uk",
	"execution_errors_pk", "outbox_messages_pk", "outbox_messages_operation_uk",
	"inbox_messages_pk", "async_node_inputs_pk", "async_node_inputs_logical_uk",
	"async_worker_results_pk", "async_worker_results_node_attempt_uk", "async_context_variables_pk",
	"node_attempts_pk", "partial_recovery_requests_pk",
	"partial_recovery_requests_recovery_uk", "partial_recovery_requests_source_uk",
), postgreSQLForeignKeyViolationCode: constraintSet("workflow_exec_snapshot_fk",
	"node_exec_parent_fk", "execution_events_parent_fk", "execution_events_node_fk",
	"execution_logs_parent_fk", "execution_logs_node_fk", "execution_errors_parent_fk",
	"execution_errors_node_fk", "execution_errors_event_fk", "outbox_messages_execution_fk",
	"outbox_messages_node_fk", "inbox_messages_execution_fk", "inbox_messages_node_fk",
	"async_node_inputs_execution_fk", "async_node_inputs_target_fk", "async_node_inputs_source_fk",
	"async_worker_results_node_fk", "async_context_variables_execution_fk",
	"node_attempts_node_fk", "partial_recovery_requests_source_fk",
	"partial_recovery_requests_recovery_fk"),
	postgreSQLCheckViolationCode: constraintSet("outbox_messages_id_length", "outbox_messages_company_not_blank",
		"outbox_messages_execution_not_blank", "outbox_messages_node_identity", "outbox_messages_operation_kind_valid",
		"outbox_messages_operation_key_length", "outbox_messages_type_length", "outbox_messages_version_positive",
		"outbox_messages_destination_length", "outbox_messages_key_length", "outbox_messages_payload_size",
		"outbox_messages_state_valid", "outbox_messages_claim_state", "outbox_messages_published_state",
		"outbox_messages_version_nonnegative", "outbox_messages_available_time",
		"inbox_messages_consumer_length", "inbox_messages_id_length",
		"inbox_messages_company_not_blank", "inbox_messages_execution_not_blank", "inbox_messages_node_not_blank",
		"inbox_messages_type_length", "inbox_messages_version_positive", "inbox_messages_result_valid",
		"inbox_messages_source_position", "inbox_messages_time_order", "async_node_inputs_id_length",
		"async_node_inputs_identifiers_not_blank", "async_node_inputs_port_length", "async_node_inputs_attempt_positive",
		"async_node_inputs_payload_object", "async_worker_results_id_length", "async_worker_results_identifiers_not_blank",
		"async_worker_results_attempt_positive", "async_worker_results_status_valid", "async_worker_results_routed_outputs",
		"async_worker_results_terminal_output", "async_worker_results_context_changes", "async_worker_results_failure",
		"async_worker_results_shape", "async_worker_results_time_order", "async_worker_results_technical_detail",
		"async_context_variables_key_length",
		"async_context_variables_value_size", "async_context_variables_time_order", "async_context_variables_version_positive",
		"node_exec_retry_policy_shape", "node_exec_retry_policy_values", "node_exec_next_attempt_shape",
		"node_attempts_attempt_positive", "node_attempts_status_valid", "node_attempts_time_order",
		"node_attempts_status_time_shape", "node_attempts_summary_shape", "node_attempts_json_shape",
		"node_attempts_retry_decision_shape",
		"partial_recovery_requests_key_length", "partial_recovery_requests_fingerprint_length",
		"partial_recovery_requests_distinct_execution", "partial_recovery_requests_plan_object",
		"partial_recovery_requests_node_counts",
	)}

func constraintSet(names ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		result[name] = struct{}{}
	}
	return result
}

type databaseError struct {
	operation string
	resource  string
	cause     error
}

func (
	err *databaseError) Error() string {
	if err == nil {
		return "PostgreSQL operation failed"
	}
	operation := strings.TrimSpace(err.operation)
	if operation == "" {
		operation = "operate on"
	}
	resource := strings.TrimSpace(err.resource)
	if resource == "" {
		resource = "record"
	}
	return fmt.Sprintf("PostgreSQL %s %s: operation failed", operation,
		resource)
}
func (err *databaseError,
) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}
func mapPostgreSQLError(
	operation string, resource string, err error,
) error {
	if err == nil {
		return nil
	}
	if errors.Is(
		err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err,
		pgx.ErrNoRows) {
		return repository.NewNotFoundError(
			operation, resource, err,
		)
	}
	var postgreSQLError *pgconn.PgError
	if errors.As(err, &postgreSQLError) &&
		isKnownPostgreSQLConstraint(postgreSQLError.Code, postgreSQLError.ConstraintName) {
		return repository.NewConflictError(operation,
			resource, err)
	}
	return &databaseError{
		operation: operation, resource: resource, cause: err,
	}
}
func isKnownPostgreSQLConstraint(code string, name string) bool {
	constraints, exists := knownPostgreSQLConstraints[code]
	if !exists {
		return false
	}
	_, exists = constraints[name]
	return exists
}

type asyncPersistenceDatabase interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

const applicationName = "miletos-go"

func NewPoolConfig(
	configuration config.PostgreSQLConfig) (*pgxpool.Config, error) {
	connectionString := buildConnectionString(
		configuration)
	poolConfiguration, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL pool configuration: %w",
			err)
	}
	poolConfiguration.MaxConns = configuration.MaxConnections
	poolConfiguration.MinConns = configuration.MinConnections
	poolConfiguration.ConnConfig.ConnectTimeout = configuration.ConnectTimeout
	if poolConfiguration.ConnConfig.RuntimeParams == nil {
		poolConfiguration.ConnConfig.RuntimeParams =
			make(map[string]string)
	}
	poolConfiguration.ConnConfig.RuntimeParams["search_path"] = configuration.Schema
	poolConfiguration.ConnConfig.RuntimeParams["application_name"] = applicationName
	return poolConfiguration, nil
}
func OpenPool(parentContext context.Context, configuration config.PostgreSQLConfig,
) (*pgxpool.Pool, error) {
	if parentContext == nil {
		return nil, fmt.Errorf(
			"PostgreSQL pool parent context must not be nil")
	}
	poolConfiguration, err := NewPoolConfig(configuration)
	if err != nil {
		return nil, err
	}
	connectContext, cancelConnect := context.WithTimeout(
		parentContext, configuration.ConnectTimeout)
	defer cancelConnect()
	pool, err := pgxpool.NewWithConfig(
		connectContext, poolConfiguration)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL connection pool: %w",
			err)
	}
	if err := pool.Ping(connectContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL database: %w",
			err)
	}
	return pool, nil
}
func buildConnectionString(configuration config.PostgreSQLConfig,
) string {
	connectionURL := &url.URL{Scheme: "postgres",
		User: url.UserPassword(configuration.User, configuration.Password), Host: configuration.Address(), Path: configuration.Database,
	}
	query := connectionURL.Query()
	query.Set("sslmode", configuration.SSLMode)
	connectionURL.RawQuery = query.Encode()
	return connectionURL.String()
}

type beginTransactionFunc func(
	ctx context.Context, options pgx.TxOptions) (pgx.Tx, error)
type Store struct {
	pool    *pgxpool.Pool
	beginTx beginTransactionFunc
}

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, fmt.Errorf("PostgreSQL store pool must not be nil")
	}
	return &Store{pool: pool, beginTx: pool.BeginTx}, nil
}
func (store *Store) IsValid() bool {
	return store != nil && store.pool != nil && store.beginTx != nil
}

type transactionWork func(tx pgx.Tx) error

func (store *Store,
) withinTransaction(ctx context.Context, operation string,
	work transactionWork) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("PostgreSQL transaction context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(operation) == "" {
		return fmt.Errorf("PostgreSQL transaction operation must not be empty")
	}
	if work == nil {
		return fmt.Errorf("PostgreSQL transaction work must not be nil")
	}
	tx, err := store.beginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite, DeferrableMode: pgx.NotDeferrable,
	})
	if err != nil {
		return mapPostgreSQLError(operation, "transaction",
			err)
	}
	if tx == nil {
		return mapPostgreSQLError(
			operation, "transaction", errors.New(
				"PostgreSQL transaction begin returned nil transaction"))
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackContext, cancelRollback := context.WithTimeout(context.Background(),
			transactionRollbackTimeout)
		defer cancelRollback()
		_ = tx.Rollback(rollbackContext)
	}()
	if err := work(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return mapPostgreSQLError(
			operation, "transaction", err,
		)
	}
	committed = true
	return nil
}

func nullableTextValue(
	value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
func nullableTimestampValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
