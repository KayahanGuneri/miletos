package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"miletos-go/internal/shared/database"
)

var (
	ErrNotFound            = database.ErrNotFound
	ErrIdempotencyConflict = errors.New("idempotency key was used for another request")
	ErrRecoveryConflict    = errors.New("a recovery already exists for the source execution")
	ErrStateTransition     = errors.New("invalid persisted state transition")
)

func newID(prefix string) string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err == nil {
		return prefix + hex.EncodeToString(value)
	}
	return fmt.Sprintf("%s%x", prefix, time.Now().UTC().UnixNano())
}

func encodeJSON(value any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode JSON: %w", err)
	}
	return encoded, nil
}

func decodeObject(encoded []byte) map[string]any {
	if len(encoded) == 0 {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(encoded, &value) != nil {
		return nil
	}
	return value
}

func mapNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

type rowScanner interface {
	Scan(...any) error
}

type sqlExecutor interface {
	Exec(context.Context, string, ...any) (database.Result, error)
	QueryRow(context.Context, string, ...any) *sql.Row
}

func optionalTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
