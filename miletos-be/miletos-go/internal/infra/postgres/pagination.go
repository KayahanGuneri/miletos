package postgres

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	repository "miletos-go/internal/ports/persistence"
	"strings"
	"time"
)

const cursorVersion = 1

type cursorKind string

const (
	cursorKindWorkflowExecutions cursorKind = "workflow_executions"
	cursorKindNodeExecutions     cursorKind = "node_executions"
	cursorKindExecutionEvents    cursorKind = "execution_events"
	cursorKindExecutionLogs      cursorKind = "execution_logs"
	cursorKindExecutionErrors    cursorKind = "execution_errors"
)

func (kind cursorKind,
) IsValid() bool {
	switch kind {
	case cursorKindWorkflowExecutions,
		cursorKindNodeExecutions, cursorKindExecutionEvents, cursorKindExecutionLogs,
		cursorKindExecutionErrors:
		return true
	default:
		return false
	}
}

type cursorPayload struct {
	Version    int        `json:"v"`
	Kind       cursorKind `json:"k"`
	Timestamp  string     `json:"t,omitempty"`
	Identifier string     `json:"i,omitempty"`
	Sequence   int64      `json:"s,omitempty"`
}

func encodeTimestampCursor(kind cursorKind,
	timestamp time.Time, identifier string) (repository.PageToken, error) {
	if !kind.IsValid() {
		return "", fmt.Errorf("cursor kind is invalid")
	}
	if timestamp.IsZero() {
		return "", fmt.Errorf("cursor timestamp must not be zero")
	}
	normalizedIdentifier := strings.TrimSpace(identifier)
	if normalizedIdentifier == "" {
		return "", fmt.Errorf("cursor identifier must not be empty")
	}
	return encodeCursorPayload(
		cursorPayload{Version: cursorVersion, Kind: kind,
			Timestamp:  timestamp.UTC().Format(time.RFC3339Nano),
			Identifier: normalizedIdentifier})
}
func decodeTimestampCursor(
	expectedKind cursorKind, token repository.PageToken) (
	time.Time, string, error,
) {
	payload, err := decodeCursorPayload(
		expectedKind, token)
	if err != nil {
		return time.Time{}, "", err
	}
	if payload.Timestamp == "" {
		return time.Time{}, "", fmt.Errorf(
			"cursor timestamp is missing")
	}
	if payload.Identifier == "" {
		return time.Time{}, "", fmt.Errorf(
			"cursor identifier is missing")
	}
	if payload.Sequence != 0 {
		return time.Time{}, "", fmt.Errorf(
			"timestamp cursor must not contain a sequence")
	}
	timestamp, err := time.Parse(time.RFC3339Nano,
		payload.Timestamp)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse cursor timestamp: %w", err)
	}
	if timestamp.IsZero() {
		return time.Time{}, "", fmt.Errorf("cursor timestamp must not be zero")
	}
	identifier := strings.TrimSpace(payload.Identifier)
	if identifier == "" {
		return time.Time{}, "", fmt.Errorf(
			"cursor identifier must not be blank")
	}
	return timestamp.UTC(), identifier,
		nil
}
func encodeSequenceCursor(kind cursorKind, sequence repository.SequenceNumber,
) (repository.PageToken, error) {
	if !kind.IsValid() {
		return "", fmt.Errorf(
			"cursor kind is invalid")
	}
	if !sequence.IsValid() {
		return "", fmt.Errorf(
			"cursor sequence must be greater than zero")
	}
	return encodeCursorPayload(cursorPayload{
		Version: cursorVersion, Kind: kind, Sequence: sequence.Int64(),
	})
}
func decodeSequenceCursor(expectedKind cursorKind,
	token repository.PageToken) (repository.SequenceNumber,
	error) {
	payload, err :=
		decodeCursorPayload(expectedKind, token)
	if err != nil {
		return 0, err
	}
	if payload.Timestamp != "" ||
		payload.Identifier != "" {
		return 0, fmt.Errorf("sequence cursor must not contain timestamp fields")
	}
	sequence, err := repository.NewSequenceNumber(payload.Sequence)
	if err != nil {
		return 0, fmt.Errorf(
			"cursor sequence is invalid: %w", err)
	}
	return sequence, nil
}
func encodeCursorPayload(
	payload cursorPayload) (repository.PageToken, error) {
	content, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal cursor payload: %w",
			err)
	}
	encoded := base64.RawURLEncoding.
		EncodeToString(content)
	return repository.PageToken(encoded),
		nil
}
func decodeCursorPayload(expectedKind cursorKind, token repository.PageToken,
) (cursorPayload, error) {
	if !expectedKind.IsValid() {
		return cursorPayload{}, fmt.Errorf(
			"expected cursor kind is invalid")
	}
	rawToken := strings.TrimSpace(
		token.String())
	if rawToken == "" {
		return cursorPayload{}, fmt.Errorf("page token must not be empty")
	}
	content, err :=
		base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil {
		return cursorPayload{}, fmt.Errorf("decode page token: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return cursorPayload{}, fmt.Errorf("decode cursor payload: %w", err)
	}
	var trailing any
	if err := decoder.Decode(
		&trailing); err != io.EOF {
		if err == nil {
			return cursorPayload{}, fmt.Errorf("cursor payload contains trailing data")
		}
		return cursorPayload{}, fmt.Errorf(
			"decode cursor trailing data: %w", err)
	}
	if payload.Version !=
		cursorVersion {
		return cursorPayload{}, fmt.Errorf("unsupported cursor version %d",
			payload.Version)
	}
	if !payload.Kind.IsValid() {
		return cursorPayload{}, fmt.Errorf(
			"cursor kind is invalid")
	}
	if payload.Kind != expectedKind {
		return cursorPayload{}, fmt.Errorf("cursor kind %q cannot be used for %q", payload.Kind,
			expectedKind)
	}
	return payload, nil
}
