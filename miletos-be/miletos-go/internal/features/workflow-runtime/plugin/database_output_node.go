package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type databaseOutputConfiguration struct {
	Operation     string
	Schema        string
	Table         string
	KeyField      string
	KeyColumn     string
	PayloadColumn string
}

func databaseOutputRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                  "core.database-output",
		InputMode:            NodeInputSingle,
		InputPorts:           standardInputPorts(),
		InputEdgeConstraint:  fixedEdgeConstraint(1),
		OutputEdgeConstraint: fixedEdgeConstraint(0),
		RoutingMode:          OutputRoutingBroadcast,
		Validator:            validateDatabaseOutput,
		Handler:              databaseOutputNodeHandler(),
	}
}

func validateDatabaseOutput(configuration map[string]any) error {
	_, err := resolveDatabaseOutputConfiguration(configuration)
	return err
}

func resolveDatabaseOutputConfiguration(
	configuration map[string]any,
) (databaseOutputConfiguration, error) {
	if err := validateDatabaseConnectionConfiguration(configuration, "DATABASE_OUTPUT"); err != nil {
		return databaseOutputConfiguration{}, err
	}
	resolved := databaseOutputConfiguration{
		Operation:     "INSERT",
		PayloadColumn: "payload",
	}
	if raw, exists := configuration["operation"]; exists && raw != nil {
		value, ok := raw.(string)
		if !ok {
			return databaseOutputConfiguration{}, validationNodeError(
				"DATABASE_OUTPUT_OPERATION_INVALID",
				"configuration.operation must be INSERT, UPDATE, or UPSERT",
			)
		}
		if value = strings.ToUpper(strings.TrimSpace(value)); value != "" {
			resolved.Operation = value
		}
	}
	switch resolved.Operation {
	case "INSERT", "UPDATE", "UPSERT":
	default:
		return databaseOutputConfiguration{}, validationNodeError(
			"DATABASE_OUTPUT_OPERATION_INVALID",
			"configuration.operation must be INSERT, UPDATE, or UPSERT",
		)
	}
	schema, schemaOK := configuration["schema"].(string)
	table, tableOK := configuration["table"].(string)
	resolved.Schema = strings.TrimSpace(schema)
	resolved.Table = strings.TrimSpace(table)
	if !schemaOK || resolved.Schema == "" {
		return databaseOutputConfiguration{}, &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_SCHEMA_REQUIRED",
			Message:  "configuration.schema is required",
		}
	}
	if !tableOK || resolved.Table == "" {
		return databaseOutputConfiguration{}, &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_TABLE_REQUIRED",
			Message:  "configuration.table is required",
		}
	}
	if !sqlIdentifierPattern.MatchString(resolved.Schema) {
		return databaseOutputConfiguration{}, &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_SCHEMA_INVALID",
			Message:  "configuration.schema must be a simple PostgreSQL identifier",
		}
	}
	if !sqlIdentifierPattern.MatchString(resolved.Table) {
		return databaseOutputConfiguration{}, &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_TABLE_INVALID",
			Message:  "configuration.table must be a simple PostgreSQL identifier",
		}
	}
	if raw, exists := configuration["payloadColumn"]; exists && raw != nil {
		value, ok := raw.(string)
		if !ok {
			return databaseOutputConfiguration{}, validationNodeError(
				"DATABASE_OUTPUT_PAYLOAD_COLUMN_INVALID",
				"configuration.payloadColumn must be a simple PostgreSQL identifier",
			)
		}
		if value = strings.TrimSpace(value); value != "" {
			resolved.PayloadColumn = value
		}
	}
	if !sqlIdentifierPattern.MatchString(resolved.PayloadColumn) {
		return databaseOutputConfiguration{}, validationNodeError(
			"DATABASE_OUTPUT_PAYLOAD_COLUMN_INVALID",
			"configuration.payloadColumn must be a simple PostgreSQL identifier",
		)
	}
	if resolved.Operation == "INSERT" {
		return resolved, nil
	}
	keyField, keyFieldOK := configuration["keyField"].(string)
	resolved.KeyField = strings.TrimSpace(keyField)
	if !keyFieldOK || resolved.KeyField == "" {
		return databaseOutputConfiguration{}, validationNodeError(
			"DATABASE_OUTPUT_KEY_FIELD_REQUIRED", "configuration.keyField is required",
		)
	}
	if strings.Contains(resolved.KeyField, ".") {
		return databaseOutputConfiguration{}, validationNodeError(
			"DATABASE_OUTPUT_KEY_FIELD_INVALID",
			"configuration.keyField must name one top-level payload field",
		)
	}
	keyColumn, keyColumnOK := configuration["keyColumn"].(string)
	resolved.KeyColumn = strings.TrimSpace(keyColumn)
	if !keyColumnOK || resolved.KeyColumn == "" {
		return databaseOutputConfiguration{}, validationNodeError(
			"DATABASE_OUTPUT_KEY_COLUMN_REQUIRED", "configuration.keyColumn is required",
		)
	}
	if !sqlIdentifierPattern.MatchString(resolved.KeyColumn) {
		return databaseOutputConfiguration{}, validationNodeError(
			"DATABASE_OUTPUT_KEY_COLUMN_INVALID",
			"configuration.keyColumn must be a simple PostgreSQL identifier",
		)
	}
	if resolved.KeyColumn == resolved.PayloadColumn {
		return databaseOutputConfiguration{}, validationNodeError(
			"DATABASE_OUTPUT_COLUMNS_CONFLICT",
			"configuration.keyColumn and configuration.payloadColumn must be different",
		)
	}
	return resolved, nil
}

func databaseOutputNodeHandler() NodeHandler {
	return func(nodeContext *Context) error {
		nodeContext.Lifecycles.OnRun(func() (any, error) {
			return databaseOutputNode(nodeContext)
		})
		return nil
	}
}

func databaseOutputNode(nodeContext *Context) (any, error) {
	configuration := nodeContext.configuration
	input, err := objectPayload(
		nodeContext.Payload,
		"DATABASE_OUTPUT_PAYLOAD_INVALID",
		"Database output requires a JSON object payload.",
	)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveDatabaseOutputConfiguration(configuration)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_PAYLOAD_INVALID",
			Message:  "Incoming payload could not be serialized as JSON.",
		}
	}
	query, arguments, err := databaseOutputStatement(resolved, input, string(encoded))
	if err != nil {
		return nil, err
	}
	connection, err := openDatabaseConnection(
		nodeContext.runtime,
		nodeContext.Infra.Database,
		nodeContext.Infra.Secrets,
		configuration,
		"DATABASE_OUTPUT",
	)
	if err != nil {
		return nil, databaseOutputExecError(err)
	}
	defer connection.Close()
	result, err := connection.Exec(nodeContext.runtime, query, arguments...)
	if err != nil {
		return nil, databaseOutputExecError(err)
	}
	if result == nil {
		return nil, &NodeError{
			Category: "INTERNAL",
			Code:     "DATABASE_OUTPUT_RESULT_INVALID",
			Message:  "The database output capability returned an invalid result.",
		}
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, databaseOutputExecError(err)
	}
	return map[string]any{
		"operation":    resolved.Operation,
		"rowsAffected": rowsAffected,
		"schema":       resolved.Schema,
		"table":        resolved.Table,
	}, nil
}

func databaseOutputStatement(
	configuration databaseOutputConfiguration,
	payload any,
	encodedPayload string,
) (string, []any, error) {
	qualifiedTable := fmt.Sprintf(
		"%s.%s",
		quotePostgreSQLIdentifier(configuration.Schema),
		quotePostgreSQLIdentifier(configuration.Table),
	)
	payloadColumn := quotePostgreSQLIdentifier(configuration.PayloadColumn)
	switch configuration.Operation {
	case "INSERT":
		return fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES ($1::jsonb)", qualifiedTable, payloadColumn,
		), []any{encodedPayload}, nil
	case "UPDATE":
		key, err := databaseOutputKey(payload, configuration.KeyField)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf(
			"UPDATE %s SET %s = $1::jsonb WHERE %s = $2",
			qualifiedTable, payloadColumn, quotePostgreSQLIdentifier(configuration.KeyColumn),
		), []any{encodedPayload, key}, nil
	case "UPSERT":
		key, err := databaseOutputKey(payload, configuration.KeyField)
		if err != nil {
			return "", nil, err
		}
		keyColumn := quotePostgreSQLIdentifier(configuration.KeyColumn)
		return fmt.Sprintf(
			"INSERT INTO %s (%s, %s) VALUES ($1, $2::jsonb) ON CONFLICT (%s) DO UPDATE SET %s = EXCLUDED.%s",
			qualifiedTable, keyColumn, payloadColumn, keyColumn, payloadColumn, payloadColumn,
		), []any{key, encodedPayload}, nil
	default:
		return "", nil, validationNodeError(
			"DATABASE_OUTPUT_OPERATION_INVALID",
			"configuration.operation must be INSERT, UPDATE, or UPSERT",
		)
	}
}

func databaseOutputKey(payload any, field string) (any, error) {
	object, ok := payload.(map[string]any)
	if !ok || object == nil {
		return nil, validationNodeError(
			"DATABASE_OUTPUT_KEY_INPUT_INVALID",
			"UPDATE and UPSERT require a JSON object payload.",
		)
	}
	value, exists := object[field]
	if !exists {
		return nil, validationNodeError(
			"DATABASE_OUTPUT_KEY_MISSING", "The configured key field is missing from the payload.",
		)
	}
	if value == nil {
		return nil, validationNodeError(
			"DATABASE_OUTPUT_KEY_NULL", "The configured key field must not be null.",
		)
	}
	switch typed := value.(type) {
	case string, bool:
		return typed, nil
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer, nil
		}
		decimal, err := typed.Float64()
		if err == nil && !math.IsNaN(decimal) && !math.IsInf(decimal, 0) {
			return decimal, nil
		}
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
			return typed, nil
		}
	case float32:
		value := float64(typed)
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			return value, nil
		}
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		return normalizeUnsignedDatabaseOutputKey(uint64(typed)), nil
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		return normalizeUnsignedDatabaseOutputKey(typed), nil
	}
	return nil, validationNodeError(
		"DATABASE_OUTPUT_KEY_INVALID", "The configured key field must contain a scalar value.",
	)
}

func normalizeUnsignedDatabaseOutputKey(value uint64) any {
	if value <= math.MaxInt64 {
		return int64(value)
	}
	return strconv.FormatUint(value, 10)
}

func databaseOutputExecError(err error) error {
	if errors.Is(err, ErrCapabilityUnavailable) {
		return &NodeError{
			Category: "INTERNAL",
			Code:     "DATABASE_OUTPUT_UNAVAILABLE",
			Message:  "Database output is not available in this runtime.",
		}
	}
	return &NodeError{
		Category: "EXECUTION",
		Code:     "DATABASE_OUTPUT_WRITE_FAILED",
		Message:  "The database output write failed.",
		CanRetry: isRetryableDatabaseOutputError(err),
		Cause:    err,
	}
}

func isRetryableDatabaseOutputError(err error) bool {
	if isRetryablePostgreSQLConnectionError(err) {
		return true
	}

	var postgresError interface{ SQLState() string }
	if !errors.As(err, &postgresError) {
		return false
	}

	switch postgresError.SQLState() {
	case "40001", "40P01", "53300":
		return true
	default:
		return false
	}
}

func quotePostgreSQLIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
