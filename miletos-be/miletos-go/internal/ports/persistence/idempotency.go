package repository

import (
	"context"
	"encoding/hex"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strings"
	"time"
)

const (
	MaximumHTTPIdempotencyKeyCharacters  = 255
	HTTPIdempotencyFingerprintCharacters = 64
)

type HTTPIdempotencyState string

const (
	HTTPIdempotencyStateReserved HTTPIdempotencyState = "RESERVED"
	HTTPIdempotencyStateAccepted HTTPIdempotencyState = "ACCEPTED"
)

func (state HTTPIdempotencyState) String() string {
	return string(state)
}
func (state HTTPIdempotencyState) IsValid() bool {
	switch state {
	case HTTPIdempotencyStateReserved, HTTPIdempotencyStateAccepted:
		return true
	default:
		return false
	}
}

type HTTPIdempotencyRecordParams struct {
	CompanyID           workflow.CompanyID
	IdempotencyKey      string
	RequestFingerprint  string
	WorkflowExecutionID execution.WorkflowExecutionID
	State               HTTPIdempotencyState
	CreatedAt           time.Time
	AcceptedAt          time.Time
}
type HTTPIdempotencyRecord struct {
	params HTTPIdempotencyRecordParams
}

func NewHTTPIdempotencyRecord(params HTTPIdempotencyRecordParams) (HTTPIdempotencyRecord, error) {
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return HTTPIdempotencyRecord{}, newValidationError(
			"companyID", err.Error())
	}
	idempotencyKey, err :=
		normalizeRequiredBoundedString("idempotencyKey", params.IdempotencyKey,
			MaximumHTTPIdempotencyKeyCharacters)
	if err != nil {
		return HTTPIdempotencyRecord{}, err
	}
	requestFingerprint, err := normalizeHTTPIdempotencyFingerprint(params.RequestFingerprint)
	if err != nil {
		return HTTPIdempotencyRecord{}, err
	}
	workflowExecutionID, err :=
		execution.NewWorkflowExecutionID(params.WorkflowExecutionID.String())
	if err != nil {
		return HTTPIdempotencyRecord{}, newValidationError(
			"workflowExecutionID", err.Error())
	}
	state, err :=
		normalizeHTTPIdempotencyState(params.State)
	if err != nil {
		return HTTPIdempotencyRecord{}, err
	}
	createdAt, err := normalizeRequiredRecordTime(
		"createdAt", params.CreatedAt)
	if err != nil {
		return HTTPIdempotencyRecord{}, err
	}
	acceptedAt := params.AcceptedAt
	switch state {
	case HTTPIdempotencyStateReserved:
		if !acceptedAt.IsZero() {
			return HTTPIdempotencyRecord{}, newValidationError("acceptedAt",
				"must be empty while idempotency state is RESERVED")
		}
	case HTTPIdempotencyStateAccepted:
		acceptedAt, err =
			normalizeRequiredRecordTime("acceptedAt", acceptedAt)
		if err != nil {
			return HTTPIdempotencyRecord{}, err
		}
		if acceptedAt.Before(createdAt) {
			return HTTPIdempotencyRecord{}, newValidationError("acceptedAt",
				"must not be before createdAt")
		}
	}
	return HTTPIdempotencyRecord{params: HTTPIdempotencyRecordParams{CompanyID: companyID, IdempotencyKey: idempotencyKey, RequestFingerprint: requestFingerprint, WorkflowExecutionID: workflowExecutionID, State: state, CreatedAt: createdAt, AcceptedAt: acceptedAt}}, nil
}
func (record HTTPIdempotencyRecord) CompanyID() workflow.CompanyID {
	return record.params.CompanyID
}
func (record HTTPIdempotencyRecord) IdempotencyKey() string {
	return record.params.IdempotencyKey
}
func (record HTTPIdempotencyRecord) RequestFingerprint() string {
	return record.params.RequestFingerprint
}
func (record HTTPIdempotencyRecord,
) WorkflowExecutionID() execution.WorkflowExecutionID {
	return record.params.WorkflowExecutionID
}
func (record HTTPIdempotencyRecord) State() HTTPIdempotencyState {
	return record.params.State
}
func (record HTTPIdempotencyRecord) CreatedAt() time.Time {
	return record.params.CreatedAt
}
func (record HTTPIdempotencyRecord) AcceptedAt() (time.Time, bool) {
	if record.params.AcceptedAt.IsZero() {
		return time.Time{}, false
	}
	return record.params.AcceptedAt, true
}
func (record HTTPIdempotencyRecord) IsValid() bool {
	_, err := NewHTTPIdempotencyRecord(record.params)
	return err == nil

}

type HTTPIdempotencyReservationParams struct {
	CompanyID           workflow.CompanyID
	IdempotencyKey      string
	RequestFingerprint  string
	WorkflowExecutionID execution.WorkflowExecutionID
	CreatedAt           time.Time
}
type HTTPIdempotencyReservation struct {
	record HTTPIdempotencyRecord
}

func NewHTTPIdempotencyReservation(
	params HTTPIdempotencyReservationParams) (HTTPIdempotencyReservation, error) {
	record, err := NewHTTPIdempotencyRecord(
		HTTPIdempotencyRecordParams{CompanyID: params.CompanyID, IdempotencyKey: params.IdempotencyKey,
			RequestFingerprint: params.RequestFingerprint, WorkflowExecutionID: params.WorkflowExecutionID, State: HTTPIdempotencyStateReserved,
			CreatedAt: params.CreatedAt})
	if err != nil {
		return HTTPIdempotencyReservation{}, err
	}
	return HTTPIdempotencyReservation{record: record}, nil
}
func (reservation HTTPIdempotencyReservation) Record() HTTPIdempotencyRecord {
	return reservation.record
}
func (reservation HTTPIdempotencyReservation) IsValid() bool {
	return reservation.record.IsValid() && reservation.record.State() == HTTPIdempotencyStateReserved
}

type HTTPIdempotencyAcceptanceParams struct {
	CompanyID           workflow.CompanyID
	IdempotencyKey      string
	RequestFingerprint  string
	WorkflowExecutionID execution.WorkflowExecutionID
	AcceptedAt          time.Time
}
type HTTPIdempotencyAcceptance struct {
	params HTTPIdempotencyAcceptanceParams
}

func NewHTTPIdempotencyAcceptance(params HTTPIdempotencyAcceptanceParams) (HTTPIdempotencyAcceptance, error) {
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return HTTPIdempotencyAcceptance{}, newValidationError(
			"companyID", err.Error())
	}
	idempotencyKey, err :=
		normalizeRequiredBoundedString("idempotencyKey", params.IdempotencyKey,
			MaximumHTTPIdempotencyKeyCharacters)
	if err != nil {
		return HTTPIdempotencyAcceptance{}, err
	}
	requestFingerprint, err := normalizeHTTPIdempotencyFingerprint(params.RequestFingerprint)
	if err != nil {
		return HTTPIdempotencyAcceptance{}, err
	}
	workflowExecutionID, err :=
		execution.NewWorkflowExecutionID(params.WorkflowExecutionID.String())
	if err != nil {
		return HTTPIdempotencyAcceptance{}, newValidationError(
			"workflowExecutionID", err.Error())
	}
	acceptedAt, err :=
		normalizeRequiredRecordTime("acceptedAt", params.AcceptedAt)
	if err != nil {
		return HTTPIdempotencyAcceptance{}, err
	}
	return HTTPIdempotencyAcceptance{params: HTTPIdempotencyAcceptanceParams{CompanyID: companyID, IdempotencyKey: idempotencyKey, RequestFingerprint: requestFingerprint, WorkflowExecutionID: workflowExecutionID, AcceptedAt: acceptedAt}}, nil
}
func (
	acceptance HTTPIdempotencyAcceptance) CompanyID() workflow.CompanyID {
	return acceptance.params.CompanyID
}
func (
	acceptance HTTPIdempotencyAcceptance) IdempotencyKey() string {
	return acceptance.params.IdempotencyKey
}
func (
	acceptance HTTPIdempotencyAcceptance) RequestFingerprint() string {
	return acceptance.params.RequestFingerprint
}
func (
	acceptance HTTPIdempotencyAcceptance) WorkflowExecutionID() execution.WorkflowExecutionID {
	return acceptance.params.WorkflowExecutionID
}
func (
	acceptance HTTPIdempotencyAcceptance) AcceptedAt() time.Time {
	return acceptance.params.AcceptedAt
}
func (
	acceptance HTTPIdempotencyAcceptance) IsValid() bool {
	_, err := NewHTTPIdempotencyAcceptance(acceptance.params)
	return err == nil

}

type HTTPIdempotencyStore interface {
	ReserveHTTPIdempotency(ctx context.Context,
		reservation HTTPIdempotencyReservation) (HTTPIdempotencyRecord,
		bool, error)
	MarkHTTPIdempotencyAccepted(ctx context.Context,
		acceptance HTTPIdempotencyAcceptance) (HTTPIdempotencyRecord,
		error)
}
type HTTPIdempotencyAcceptanceEvidenceStore interface {
	HasHTTPIdempotencyAcceptanceEvidence(
		ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	) (bool, error,
	)
}

func normalizeHTTPIdempotencyFingerprint(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(
		value))
	if len(normalized) != HTTPIdempotencyFingerprintCharacters {
		return "", newValidationError("requestFingerprint",
			"must contain exactly 64 lowercase hexadecimal characters")
	}
	decoded, err := hex.DecodeString(
		normalized)
	if err != nil ||
		len(decoded) != 32 {
		return "", newValidationError(
			"requestFingerprint", "must contain exactly 64 lowercase hexadecimal characters")
	}
	return normalized, nil
}
func normalizeHTTPIdempotencyState(
	value HTTPIdempotencyState) (HTTPIdempotencyState, error) {
	normalized :=
		HTTPIdempotencyState(strings.ToUpper(strings.TrimSpace(
			value.String())),
		)
	if !normalized.IsValid() {
		return "", newValidationError("state",
			"must be RESERVED or ACCEPTED")
	}
	return normalized, nil
}
