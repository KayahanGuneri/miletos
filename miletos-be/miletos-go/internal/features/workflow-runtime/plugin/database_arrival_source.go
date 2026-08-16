package plugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	databaseArrivalBatchSize      = 100
	databaseSnapshotCursorVersion = 2
)

type databaseCursor struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type databaseSnapshotRow struct {
	Identity    databaseCursor `json:"identity"`
	Fingerprint string         `json:"fingerprint"`
	Row         map[string]any `json:"row"`
}

type databaseSnapshotCursor struct {
	Version int                            `json:"version"`
	Rows    map[string]databaseSnapshotRow `json:"rows"`
}

type databaseChange struct {
	Kind                string
	IdentityKey         string
	Current             *databaseSnapshotRow
	Previous            *databaseSnapshotRow
	CurrentFingerprint  string
	PreviousFingerprint string
}

func databaseArrivalSource(
	db DatabaseInfrastructure,
	secrets SecretDecryptor,
) *DataArrivalSource {
	if db == nil {
		db = unavailableDatabase{}
	}
	return &DataArrivalSource{
		Enabled: func(configuration map[string]any) bool {
			return configString(configuration, "cursorColumn") != ""
		},
		Validate: validateDatabaseArrivalInput,
		Poll: func(
			ctx context.Context,
			_ string,
			configuration map[string]any,
			cursor any,
		) ([]DataArrival, error) {
			if err := validateDatabaseArrivalInput(configuration); err != nil {
				return nil, err
			}
			return pollDatabaseArrivals(ctx, db, secrets, configuration, cursor)
		},
	}
}

func pollDatabaseArrivals(
	ctx context.Context,
	db DatabaseInfrastructure,
	secrets SecretDecryptor,
	configuration map[string]any,
	stored any,
) ([]DataArrival, error) {
	connection, err := openDatabaseConnection(
		ctx, db, secrets, configuration, "DATABASE_INPUT",
	)
	if err != nil {
		return nil, databaseArrivalError(
			"DATABASE_INPUT_QUERY_FAILED", "The database input query failed.", err,
		)
	}
	defer connection.Close()
	cursorColumn := configString(configuration, "cursorColumn")
	query := fmt.Sprintf(
		`SELECT * FROM (%s) AS miletos_arrivals ORDER BY "%s" ASC`,
		configString(configuration, "query"),
		cursorColumn,
	)
	rows, err := connection.Query(ctx, query)
	if err != nil {
		return nil, databaseArrivalError("DATABASE_INPUT_QUERY_FAILED", "The database input query failed.", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, databaseArrivalError("DATABASE_INPUT_READ_FAILED", "The database input result columns could not be read.", err)
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, databaseArrivalError("DATABASE_INPUT_READ_FAILED", "The database input result column types could not be read.", err)
	}
	cursorIndex := -1
	for index, column := range columns {
		if column == cursorColumn {
			cursorIndex = index
			break
		}
	}
	if cursorIndex < 0 {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_INPUT_CURSOR_COLUMN_MISSING",
			Message:  "configuration.cursorColumn must identify a returned column",
		}
	}
	current := databaseSnapshotCursor{
		Version: databaseSnapshotCursorVersion,
		Rows:    make(map[string]databaseSnapshotRow),
	}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, databaseArrivalError("DATABASE_INPUT_READ_FAILED", "A database input row could not be read.", err)
		}
		identity, err := databaseCursorFromValue(
			values[cursorIndex], columnTypes[cursorIndex].DatabaseTypeName(),
		)
		if err != nil {
			return nil, err
		}
		identityKey := databaseCursorKey(identity)
		if _, duplicate := current.Rows[identityKey]; duplicate {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "DATABASE_INPUT_CURSOR_NOT_UNIQUE",
				Message:  "configuration.cursorColumn must contain unique values",
			}
		}
		record := make(map[string]any, len(columns))
		for index, column := range columns {
			record[column] = normalizeSQLValue(values[index])
		}
		fingerprint, err := databaseRowFingerprint(record)
		if err != nil {
			return nil, err
		}
		current.Rows[identityKey] = databaseSnapshotRow{
			Identity: identity, Fingerprint: fingerprint, Row: record,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, databaseArrivalError("DATABASE_INPUT_READ_FAILED", "The database input result stream failed.", err)
	}
	previous, legacy, err := decodeDatabaseSnapshot(stored)
	if err != nil {
		return nil, err
	}
	if legacy {
		return []DataArrival{{Cursor: current, CheckpointOnly: true}}, nil
	}
	changes, err := databaseSnapshotChanges(previous, current)
	if err != nil {
		return nil, err
	}
	progressed := cloneDatabaseSnapshot(previous)
	eventCount := len(changes)
	if eventCount > databaseArrivalBatchSize {
		eventCount = databaseArrivalBatchSize
	}
	events := make([]DataArrival, 0, eventCount)
	for _, change := range changes[:eventCount] {
		var payload map[string]any
		switch change.Kind {
		case "INSERT", "UPDATE":
			progressed.Rows[change.IdentityKey] = *change.Current
			payload = cloneObject(change.Current.Row)
		case "DELETE":
			delete(progressed.Rows, change.IdentityKey)
			payload = cloneObject(change.Previous.Row)
		}
		events = append(events, DataArrival{
			Cursor:  cloneDatabaseSnapshot(progressed),
			Key:     databaseChangeKey(change),
			Payload: payload,
		})
	}
	return events, nil
}

func databaseSnapshotChanges(
	previous databaseSnapshotCursor,
	current databaseSnapshotCursor,
) ([]databaseChange, error) {
	changes := make([]databaseChange, 0)
	for identityKey, row := range current.Rows {
		previousRow, exists := previous.Rows[identityKey]
		if !exists {
			currentCopy := row
			changes = append(changes, databaseChange{
				Kind: "INSERT", IdentityKey: identityKey,
				Current: &currentCopy, CurrentFingerprint: row.Fingerprint,
			})
			continue
		}
		if previousRow.Identity != row.Identity {
			return nil, invalidDatabaseSnapshot()
		}
		if previousRow.Fingerprint != row.Fingerprint {
			currentCopy := row
			previousCopy := previousRow
			changes = append(changes, databaseChange{
				Kind: "UPDATE", IdentityKey: identityKey,
				Current: &currentCopy, Previous: &previousCopy,
				CurrentFingerprint:  row.Fingerprint,
				PreviousFingerprint: previousRow.Fingerprint,
			})
		}
	}
	for identityKey, row := range previous.Rows {
		if _, exists := current.Rows[identityKey]; exists {
			continue
		}
		previousCopy := row
		changes = append(changes, databaseChange{
			Kind: "DELETE", IdentityKey: identityKey,
			Previous: &previousCopy, PreviousFingerprint: row.Fingerprint,
		})
	}
	sort.SliceStable(changes, func(left, right int) bool {
		leftIdentity := databaseChangeIdentity(changes[left])
		rightIdentity := databaseChangeIdentity(changes[right])
		comparison, err := compareDatabaseCursors(leftIdentity, rightIdentity)
		if err != nil {
			return changes[left].IdentityKey < changes[right].IdentityKey
		}
		if comparison == 0 {
			return changes[left].Kind < changes[right].Kind
		}
		return comparison < 0
	})
	return changes, nil
}

func databaseChangeIdentity(change databaseChange) databaseCursor {
	if change.Current != nil {
		return change.Current.Identity
	}
	return change.Previous.Identity
}

func databaseChangeKey(change databaseChange) string {
	identity := databaseChangeIdentity(change)
	digest := sha256.Sum256([]byte(strings.Join([]string{
		identity.Kind,
		identity.Value,
		change.Kind,
		change.CurrentFingerprint,
		change.PreviousFingerprint,
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func databaseRowFingerprint(row map[string]any) (string, error) {
	encoded, err := json.Marshal(row)
	if err != nil {
		return "", &NodeError{
			Category: "EXECUTION",
			Code:     "DATABASE_INPUT_ROW_FINGERPRINT_FAILED",
			Message:  "A database input row could not be fingerprinted.",
			Cause:    err,
		}
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func decodeDatabaseSnapshot(value any) (databaseSnapshotCursor, bool, error) {
	empty := databaseSnapshotCursor{
		Version: databaseSnapshotCursorVersion,
		Rows:    make(map[string]databaseSnapshotRow),
	}
	if value == nil {
		return empty, false, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return databaseSnapshotCursor{}, false, invalidDatabaseSnapshot()
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var snapshot databaseSnapshotCursor
	if err := decoder.Decode(&snapshot); err == nil &&
		snapshot.Version == databaseSnapshotCursorVersion && snapshot.Rows != nil {
		for identityKey, row := range snapshot.Rows {
			if row.Identity.Kind == "" || row.Fingerprint == "" || row.Row == nil ||
				databaseCursorKey(row.Identity) != identityKey {
				return databaseSnapshotCursor{}, false, invalidDatabaseSnapshot()
			}
		}
		return snapshot, false, nil
	}
	var legacy databaseCursor
	if err := json.Unmarshal(encoded, &legacy); err == nil && legacy.Kind != "" {
		return empty, true, nil
	}
	return databaseSnapshotCursor{}, false, invalidDatabaseSnapshot()
}

func cloneDatabaseSnapshot(source databaseSnapshotCursor) databaseSnapshotCursor {
	result := databaseSnapshotCursor{
		Version: databaseSnapshotCursorVersion,
		Rows:    make(map[string]databaseSnapshotRow, len(source.Rows)),
	}
	for key, row := range source.Rows {
		row.Row = cloneObject(row.Row)
		result.Rows[key] = row
	}
	return result
}

func invalidDatabaseSnapshot() error {
	return &NodeError{
		Category: "INTERNAL",
		Code:     "DATABASE_INPUT_CURSOR_STATE_INVALID",
		Message:  "The persisted database input snapshot is invalid.",
	}
}

func databaseArrivalError(code, message string, cause error) error {
	if errors.Is(cause, ErrCapabilityUnavailable) {
		return &NodeError{
			Category: "INTERNAL",
			Code:     "DATABASE_INPUT_UNAVAILABLE",
			Message:  "Database input is not available in this runtime.",
		}
	}
	return &NodeError{
		Category: "EXECUTION", Code: code, Message: message,
		CanRetry: isRetryablePostgreSQLConnectionError(cause), Cause: cause,
	}
}

func databaseCursorFromValue(value any, databaseType string) (databaseCursor, error) {
	databaseType = strings.ToUpper(strings.TrimSpace(databaseType))
	if value == nil {
		return databaseCursor{}, invalidDatabaseCursor()
	}
	if timestamp, ok := value.(time.Time); ok {
		return databaseCursor{Kind: "time", Value: timestamp.UTC().Format(time.RFC3339Nano)}, nil
	}
	switch value := value.(type) {
	case bool:
		return databaseCursor{Kind: "boolean", Value: strconv.FormatBool(value)}, nil
	case int64:
		return databaseCursor{Kind: "integer", Value: strconv.FormatInt(value, 10)}, nil
	case int32:
		return databaseCursor{Kind: "integer", Value: strconv.FormatInt(int64(value), 10)}, nil
	case int:
		return databaseCursor{Kind: "integer", Value: strconv.Itoa(value)}, nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return databaseCursor{}, invalidDatabaseCursor()
		}
		return databaseCursor{Kind: "decimal", Value: strconv.FormatFloat(value, 'f', -1, 64)}, nil
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return databaseCursor{}, invalidDatabaseCursor()
		}
		return databaseCursor{Kind: "decimal", Value: strconv.FormatFloat(float64(value), 'f', -1, 32)}, nil
	}
	text := ""
	switch value := value.(type) {
	case []byte:
		text = string(value)
	case string:
		text = value
	default:
		return databaseCursor{}, invalidDatabaseCursor()
	}
	if isIntegerDatabaseType(databaseType) {
		if _, ok := new(big.Int).SetString(text, 10); !ok {
			return databaseCursor{}, invalidDatabaseCursor()
		}
		return databaseCursor{Kind: "integer", Value: text}, nil
	}
	if isDecimalDatabaseType(databaseType) {
		if _, ok := new(big.Rat).SetString(text); !ok {
			return databaseCursor{}, invalidDatabaseCursor()
		}
		return databaseCursor{Kind: "decimal", Value: text}, nil
	}
	return databaseCursor{Kind: "string", Value: text}, nil
}

func compareDatabaseCursors(left, right databaseCursor) (int, error) {
	if left.Kind != right.Kind {
		return 0, invalidDatabaseCursor()
	}
	switch left.Kind {
	case "boolean":
		leftValue, leftErr := strconv.ParseBool(left.Value)
		rightValue, rightErr := strconv.ParseBool(right.Value)
		if leftErr != nil || rightErr != nil {
			return 0, invalidDatabaseCursor()
		}
		if leftValue == rightValue {
			return 0, nil
		}
		if !leftValue {
			return -1, nil
		}
		return 1, nil
	case "integer":
		leftValue, leftOK := new(big.Int).SetString(left.Value, 10)
		rightValue, rightOK := new(big.Int).SetString(right.Value, 10)
		if !leftOK || !rightOK {
			return 0, invalidDatabaseCursor()
		}
		return leftValue.Cmp(rightValue), nil
	case "decimal":
		leftValue, leftOK := new(big.Rat).SetString(left.Value)
		rightValue, rightOK := new(big.Rat).SetString(right.Value)
		if !leftOK || !rightOK {
			return 0, invalidDatabaseCursor()
		}
		return leftValue.Cmp(rightValue), nil
	case "time":
		leftValue, leftErr := time.Parse(time.RFC3339Nano, left.Value)
		rightValue, rightErr := time.Parse(time.RFC3339Nano, right.Value)
		if leftErr != nil || rightErr != nil {
			return 0, invalidDatabaseCursor()
		}
		if leftValue.Before(rightValue) {
			return -1, nil
		}
		if leftValue.After(rightValue) {
			return 1, nil
		}
		return 0, nil
	case "string":
		return strings.Compare(left.Value, right.Value), nil
	default:
		return 0, invalidDatabaseCursor()
	}
}

func databaseCursorKey(cursor databaseCursor) string {
	digest := sha256.Sum256([]byte(cursor.Kind + "\x00" + cursor.Value))
	return hex.EncodeToString(digest[:])
}

func invalidDatabaseCursor() error {
	return &NodeError{
		Category: "VALIDATION",
		Code:     "DATABASE_INPUT_CURSOR_INVALID",
		Message:  "configuration.cursorColumn must contain non-null deterministic scalar values",
	}
}

func isIntegerDatabaseType(databaseType string) bool {
	switch databaseType {
	case "INT2", "INT4", "INT8", "SMALLINT", "INTEGER", "BIGINT", "SMALLSERIAL", "SERIAL", "BIGSERIAL":
		return true
	default:
		return false
	}
}

func isDecimalDatabaseType(databaseType string) bool {
	switch databaseType {
	case "NUMERIC", "DECIMAL", "FLOAT4", "FLOAT8", "REAL", "DOUBLE PRECISION":
		return true
	default:
		return false
	}
}
