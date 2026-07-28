package repository

import (
	"bytes"
	"errors"
	"fmt"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strings"
	"time"
	"unicode/utf8"
)

func normalizeRequiredBoundedString(field string,
	value string, maximumCharacters int) (string, error) {
	if maximumCharacters <= 0 {
		return "", newValidationError(field,
			"maximum character count must be greater than zero")
	}
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field, "must not be empty")
	}
	if utf8.RuneCountInString(normalized) > maximumCharacters {
		return "", newValidationError(
			field, fmt.Sprintf("must not exceed %d characters",
				maximumCharacters))
	}
	return normalized, nil
}
func normalizeOptionalBoundedString(
	field string, value string, maximumCharacters int,
) (string, error) {
	if value == "" {
		return "", nil
	}
	return normalizeRequiredBoundedString(
		field, value, maximumCharacters,
	)
}

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "repository validation failed"
	}
	if e.Field == "" && e.Reason == "" {
		return "repository validation failed"
	}
	if e.Field == "" {
		return fmt.Sprintf(
			"repository validation failed: %s", e.Reason)
	}
	if e.Reason == "" {
		return fmt.Sprintf("repository validation failed for %s", e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Field,
		e.Reason)
}
func newValidationError(field string,
	reason string) error {
	return &ValidationError{
		Field: field, Reason: reason}
}

type DefinitionSnapshotID string
type ExecutionEventID string
type ExecutionLogID string
type ExecutionErrorID string

func NewDefinitionSnapshotID(value string) (DefinitionSnapshotID, error) {
	normalized, err := normalizeRequiredString("definitionSnapshotID", value)
	if err != nil {
		return "", err
	}
	return DefinitionSnapshotID(normalized), nil
}
func (id DefinitionSnapshotID) String() string {
	return string(id)
}
func NewExecutionEventID(value string) (ExecutionEventID, error) {
	normalized, err := normalizeRequiredString("executionEventID", value)
	if err != nil {
		return "", err
	}
	return ExecutionEventID(normalized), nil
}
func (id ExecutionEventID) String() string {
	return string(id)
}
func NewExecutionLogID(value string) (ExecutionLogID, error) {
	normalized, err := normalizeRequiredString("executionLogID", value)
	if err != nil {
		return "", err
	}
	return ExecutionLogID(normalized), nil
}
func (id ExecutionLogID) String() string {
	return string(id)
}
func NewExecutionErrorID(value string) (ExecutionErrorID, error) {
	normalized, err := normalizeRequiredString("executionErrorID", value)
	if err != nil {
		return "", err
	}
	return ExecutionErrorID(normalized), nil
}
func (id ExecutionErrorID) String() string {
	return string(id)
}
func normalizeRequiredString(field string, value string,
) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field, "must not be empty")
	}
	return normalized, nil
}

type JSONObject = workflow.JSONObject

func NewJSONObject(value []byte) (JSONObject, error) { return newJSONObject("jsonObject", value) }
func newJSONObject(field string, value []byte) (JSONObject, error) {
	object, err := workflow.NewJSONObject(value)
	if err == nil {
		return object, nil
	}
	var validationError *workflow.ValidationError
	if errors.As(err, &validationError) {
		return JSONObject{}, newValidationError(field, validationError.Reason)
	}
	return JSONObject{}, err
}

func normalizeOptionalNodeExecutionID(
	value execution.NodeExecutionID) (execution.NodeExecutionID, bool, error) {
	if value.String() == "" {
		return "", false, nil
	}
	normalized, err := execution.NewNodeExecutionID(value.String())
	if err != nil {
		return "", false, newValidationError("nodeExecutionID",
			err.Error())
	}
	return normalized, true, nil
}

const (
	DefaultPageLimit     = 50
	MaximumPageLimit     = 100
	maximumPageTokenSize = 2048
)

type PageToken string

func (token PageToken) String() string {
	return string(token)
}

type PageRequest struct {
	limit    int
	after    PageToken
	hasAfter bool
}

func NewPageRequest(limit int, after PageToken,
) (PageRequest, error) {
	normalizedLimit := limit
	if normalizedLimit == 0 {
		normalizedLimit = DefaultPageLimit
	}
	if normalizedLimit < 1 || normalizedLimit > MaximumPageLimit {
		return PageRequest{}, newValidationError("pageLimit", "must be between 1 and 100")
	}
	normalizedAfter, hasAfter, err := normalizeOptionalPageToken(after)
	if err != nil {
		return PageRequest{}, err
	}
	return PageRequest{limit: normalizedLimit, after: normalizedAfter,
		hasAfter: hasAfter}, nil
}
func (request PageRequest) Limit() int {
	return request.limit
}
func (request PageRequest) After() (
	PageToken, bool) {
	if !request.hasAfter {
		return "", false
	}
	return request.after, true
}
func (request PageRequest) IsValid() bool {
	_, err := NewPageRequest(
		request.limit, request.after)
	return err == nil
}

type Page[T any] struct {
	items   []T
	next    PageToken
	hasNext bool
}

func NewPage[T any](items []T,
	next PageToken) (Page[T], error) {
	normalizedNext, hasNext, err :=
		normalizeOptionalPageToken(next)
	if err != nil {
		return Page[T]{}, err
	}
	copiedItems := append(
		[]T(nil), items...)
	return Page[T]{items: copiedItems,
		next: normalizedNext, hasNext: hasNext}, nil
}
func (page Page[T]) Items() []T {
	return append([]T(nil), page.items...,
	)
}
func (page Page[T]) Next() (PageToken, bool,
) {
	if !page.hasNext {
		return "", false
	}
	return page.next, true
}
func (page Page[T]) HasNext() bool {
	return page.hasNext
}
func normalizeOptionalPageToken(value PageToken) (PageToken, bool, error) {
	raw := value.String()
	if raw == "" {
		return "", false, nil
	}
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", false, newValidationError(
			"pageToken", "must not be blank when provided")
	}
	if utf8.RuneCountInString(normalized) >
		maximumPageTokenSize {
		return "", false, newValidationError("pageToken",
			"must not exceed 2048 characters")
	}
	return PageToken(normalized), true, nil
}
func normalizeOptionalString(field string,
	value string) (string, error) {
	if value == "" {
		return "", nil
	}
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(
			field, "must not be blank when provided")
	}
	return normalized, nil
}
func normalizeRequiredRecordTime(
	field string, value time.Time) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, newValidationError(field,
			"must not be zero")
	}
	return value.UTC(), nil
}
func normalizeOptionalRecordTime(field string,
	value time.Time, createdAt time.Time) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, nil
	}
	normalized := value.UTC()
	if normalized.Before(createdAt) {
		return time.Time{}, newValidationError(field,
			"must not be before createdAt")
	}
	return normalized, nil
}
func normalizeOptionalJSONObject(field string,
	value []byte) (JSONObject, bool, error) {
	if len(bytes.TrimSpace(value)) == 0 {
		return JSONObject{}, false, nil
	}
	object, err := newJSONObject(field, value)
	if err != nil {
		return JSONObject{}, false, err
	}
	return object, true, nil
}
func validateUpdatedRecordTime(
	updatedAt time.Time, createdAt time.Time, transitionTimes ...time.Time,
) error {
	if updatedAt.Before(createdAt) {
		return newValidationError(
			"updatedAt", "must not be before createdAt")
	}
	for _, transitionTime := range transitionTimes {
		if transitionTime.IsZero() {
			continue
		}
		if updatedAt.Before(transitionTime) {
			return newValidationError(
				"updatedAt", "must not be before a recorded transition time")
		}
	}
	return nil
}

type SequenceNumber int64

func NewSequenceNumber(
	value int64) (SequenceNumber, error) {
	if value <= 0 {
		return 0, newValidationError("sequenceNumber", "must be greater than zero")
	}
	return SequenceNumber(value), nil
}
func (number SequenceNumber) Int64() int64 { return int64(number) }
func (number SequenceNumber) IsValid() bool {
	return number > 0
}

type StoreErrorKind string

const (
	StoreErrorKindNotFound   StoreErrorKind = "NOT_FOUND"
	StoreErrorKindConflict   StoreErrorKind = "CONFLICT"
	StoreErrorKindStaleWrite StoreErrorKind = "STALE_WRITE"
)

type StoreError struct {
	Kind      StoreErrorKind
	Operation string
	Resource  string
	Cause     error
}

func (e *StoreError) Error() string {
	if e == nil {
		return "repository operation failed"
	}
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "operate on"
	}
	resource := strings.TrimSpace(e.Resource)
	if resource == "" {
		resource = "record"
	}
	return fmt.Sprintf(
		"repository %s %s: %s", operation, resource,
		storeErrorKindMessage(e.Kind))
}
func (e *StoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
func NewNotFoundError(operation string, resource string,
	cause error) error {
	return &StoreError{
		Kind: StoreErrorKindNotFound, Operation: operation, Resource: resource,
		Cause: cause}
}
func NewConflictError(operation string,
	resource string, cause error) error {
	return &StoreError{Kind: StoreErrorKindConflict, Operation: operation,
		Resource: resource, Cause: cause}
}
func NewStaleWriteError(
	operation string, resource string, cause error,
) error {
	return &StoreError{Kind: StoreErrorKindStaleWrite,
		Operation: operation, Resource: resource, Cause: cause,
	}
}
func IsNotFound(err error) bool {
	return hasStoreErrorKind(err, StoreErrorKindNotFound)
}
func IsConflict(err error) bool {
	return hasStoreErrorKind(err, StoreErrorKindConflict)
}
func IsStaleWrite(err error) bool {
	return hasStoreErrorKind(err, StoreErrorKindStaleWrite)
}
func hasStoreErrorKind(err error, expected StoreErrorKind,
) bool {
	var storeError *StoreError
	if !errors.As(err, &storeError) {
		return false
	}
	return storeError.Kind == expected
}
func storeErrorKindMessage(kind StoreErrorKind,
) string {
	switch kind {
	case StoreErrorKindNotFound:
		return "not found"
	case StoreErrorKindConflict:
		return "conflict"
	case StoreErrorKindStaleWrite:
		return "stale write"
	default:
		return "operation failed"
	}
}

type TimelineEntryKind string

const (
	TimelineEntryKindEvent TimelineEntryKind = "EVENT"
	TimelineEntryKindLog   TimelineEntryKind = "LOG"
)

func (kind TimelineEntryKind) String() string {
	return string(kind)
}
func (kind TimelineEntryKind) IsValid() bool {
	switch kind {
	case TimelineEntryKindEvent,
		TimelineEntryKindLog:
		return true
	default:
		return false
	}
}

type TimelineEntry struct {
	kind  TimelineEntryKind
	event ExecutionEventDraft
	log   ExecutionLogDraft
}

func NewEventTimelineEntry(
	event ExecutionEventDraft) (TimelineEntry, error) {
	if !event.IsValid() {
		return TimelineEntry{}, newValidationError("eventDraft", "must be valid")
	}
	return TimelineEntry{kind: TimelineEntryKindEvent, event: event}, nil
}
func NewLogTimelineEntry(log ExecutionLogDraft) (TimelineEntry, error) {
	if !log.IsValid() {
		return TimelineEntry{}, newValidationError("logDraft",
			"must be valid")
	}
	return TimelineEntry{kind: TimelineEntryKindLog,
		log: log}, nil
}
func (entry TimelineEntry) Kind() TimelineEntryKind {
	return entry.kind
}
func (entry TimelineEntry) Event() (
	ExecutionEventDraft, bool) {
	if entry.kind != TimelineEntryKindEvent {
		return ExecutionEventDraft{}, false
	}
	return entry.event, true
}
func (entry TimelineEntry) Log() (ExecutionLogDraft,
	bool) {
	if entry.kind != TimelineEntryKindLog {
		return ExecutionLogDraft{}, false
	}
	return entry.log, true
}
func (entry TimelineEntry) IsValid() bool {
	switch entry.kind {
	case TimelineEntryKindEvent:
		return entry.event.IsValid() && !entry.log.IsValid()
	case TimelineEntryKindLog:
		return entry.log.IsValid() && !entry.event.IsValid()
	default:
		return false
	}
}
func optionalJSONObjectBytes(value JSONObject, exists bool) []byte {
	if !exists {
		return nil
	}
	return value.
		Bytes()
}
