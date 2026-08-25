package plugin

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"
)

var selectSQLKeyword = regexp.MustCompile(`(?i)^SELECT\b`)

var forbiddenSQLKeyword = regexp.MustCompile(
	`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|MERGE|COPY|INTO)\b`,
)

var databaseCursorColumn = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func databaseInputRegistration(runtime InputNodeRuntime) NodeRegistration {
	return NodeRegistration{
		Key:                     "core.database-input",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateDatabaseInput,
		Handler:                 databaseInputNodeHandler(),
		AllowedExecutionSources: []string{"MANUAL_DIRECT", ExecutionSourceDataArrival},
		DataArrivalSource:       databaseArrivalSource(runtime.Database, runtime.Secrets),
		RecordSource:            &RecordSource{Materialize: databaseInputRecordMaterializer(runtime)},
	}
}

func validateDatabaseInput(configuration map[string]any) error {
	if err := validateDatabaseConnectionConfiguration(configuration, "DATABASE_INPUT"); err != nil {
		return err
	}
	query := configString(configuration, "query")
	if query == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_INPUT_QUERY_REQUIRED",
			Message:  "configuration.query is required",
		}
	}
	if !isConservativeSelectQuery(query) {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_INPUT_QUERY_INVALID",
			Message:  "configuration.query must be a single SELECT statement",
		}
	}
	return nil
}

func validateDatabaseArrivalInput(configuration map[string]any) error {
	if err := validateDatabaseInput(configuration); err != nil {
		return err
	}
	cursorColumn := configString(configuration, "cursorColumn")
	if !databaseCursorColumn.MatchString(cursorColumn) {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_INPUT_CURSOR_COLUMN_INVALID",
			Message:  "configuration.cursorColumn must be a simple returned column name",
		}
	}
	return nil
}

func databaseInputNodeHandler() NodeHandler {
	return func(nodeContext *Context) error {
		nodeContext.Lifecycles.OnRun(func() (any, error) {
			configuration := nodeContext.configuration
			if err := validateDatabaseInput(configuration); err != nil {
				return nil, err
			}
			if payload, arrived := dataArrivalPayload(nodeContext.Payload); arrived {
				return requireSourceObject(payload, "DATABASE_INPUT_ARRIVAL_INVALID")
			}
			return nil, sourceDispatchRequired("DATABASE_INPUT")
		})
		return nil
	}
}

func databaseInputRecordMaterializer(runtime InputNodeRuntime) RecordMaterializer {
	return func(
		ctx context.Context,
		_ string,
		configuration map[string]any,
	) ([]map[string]any, error) {
		if err := validateDatabaseInput(configuration); err != nil {
			return nil, err
		}
		connection, err := openDatabaseConnection(
			ctx,
			runtime.Database,
			runtime.Secrets,
			configuration,
			"DATABASE_INPUT",
		)
		if err != nil {
			return nil, databaseInputQueryError(err)
		}
		defer connection.Close()
		query := configString(configuration, "query")
		rows, err := connection.Query(ctx, query)
		if err != nil {
			return nil, databaseInputQueryError(err)
		}
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "DATABASE_INPUT_READ_FAILED",
				Message:  "The database input result columns could not be read.",
				CanRetry: isRetryablePostgreSQLConnectionError(err),
			}
		}
		results := make([]map[string]any, 0)
		for rows.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for index := range values {
				destinations[index] = &values[index]
			}
			if err := rows.Scan(destinations...); err != nil {
				return nil, &NodeError{
					Category: "EXECUTION",
					Code:     "DATABASE_INPUT_READ_FAILED",
					Message:  "A database input row could not be read.",
					CanRetry: false,
				}
			}
			record := make(map[string]any, len(columns))
			for index, column := range columns {
				record[column] = normalizeSQLValue(values[index])
			}
			results = append(results, record)
		}
		if err := rows.Err(); err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "DATABASE_INPUT_READ_FAILED",
				Message:  "The database input result stream failed.",
				CanRetry: isRetryablePostgreSQLConnectionError(err),
			}
		}
		return results, nil
	}
}

func requireSourceObject(payload any, code string) (map[string]any, error) {
	object, ok := payload.(map[string]any)
	if !ok || object == nil {
		return nil, &NodeError{
			Category: "INTERNAL",
			Code:     code,
			Message:  "The source record payload is invalid.",
		}
	}
	return object, nil
}

func sourceDispatchRequired(prefix string) *NodeError {
	return &NodeError{
		Category: "INTERNAL",
		Code:     prefix + "_DISPATCH_REQUIRED",
		Message:  "Manual record sources must be dispatched before node execution.",
	}
}

func databaseInputQueryError(err error) error {
	if errors.Is(err, ErrCapabilityUnavailable) {
		return &NodeError{
			Category: "INTERNAL",
			Code:     "DATABASE_INPUT_UNAVAILABLE",
			Message:  "Database input is not available in this runtime.",
		}
	}
	return &NodeError{
		Category: "EXECUTION",
		Code:     "DATABASE_INPUT_QUERY_FAILED",
		Message:  "The database input query failed.",
		CanRetry: isRetryablePostgreSQLConnectionError(err),
	}
}

func isRetryablePostgreSQLConnectionError(err error) bool {
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return true
	}
	var postgresError interface{ SQLState() string }
	if !errors.As(err, &postgresError) {
		return false
	}
	sqlState := postgresError.SQLState()
	return strings.HasPrefix(sqlState, "08") ||
		sqlState == "57P01" ||
		sqlState == "57P02" ||
		sqlState == "57P03"
}

func isConservativeSelectQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" || strings.Contains(trimmed, ";") {
		return false
	}
	if !selectSQLKeyword.MatchString(trimmed) {
		return false
	}
	return !forbiddenSQLKeyword.MatchString(trimmed)
}

func normalizeSQLValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return value
	}
}
