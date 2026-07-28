package repository

import (
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strings"
	"time"
)

const (
	maximumExecutionErrorCodeCharacters            = 128
	maximumExecutionErrorSafeMessageCharacters     = 2000
	maximumExecutionErrorTechnicalDetailCharacters = 8000
)

type ExecutionErrorRecordParams struct {
	ID                  ExecutionErrorID
	WorkflowExecutionID execution.WorkflowExecutionID
	CompanyID           workflow.CompanyID
	NodeExecutionID     execution.NodeExecutionID
	RelatedEventID      ExecutionEventID
	Category            runtime.FailureCategory
	Code                string
	SafeMessage         string
	TechnicalDetail     string
	Retryable           bool
	Details             []byte
	CreatedAt           time.Time
}
type ExecutionErrorRecord struct {
	params             ExecutionErrorRecordParams
	hasNodeExecutionID bool
	hasRelatedEventID  bool
	details            JSONObject
}

func NewExecutionErrorRecord(params ExecutionErrorRecordParams) (ExecutionErrorRecord, error) {
	normalizedID, err := NewExecutionErrorID(params.ID.String())
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	normalizedWorkflowExecutionID, err := execution.NewWorkflowExecutionID(params.WorkflowExecutionID.String())
	if err != nil {
		return ExecutionErrorRecord{}, newValidationError("workflowExecutionID", err.Error())
	}
	normalizedCompanyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return ExecutionErrorRecord{}, newValidationError("companyID", err.Error())
	}
	normalizedNodeExecutionID, hasNodeExecutionID, err := normalizeOptionalNodeExecutionID(params.NodeExecutionID)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	normalizedRelatedEventID, hasRelatedEventID, err := normalizeOptionalExecutionEventID(params.RelatedEventID)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	normalizedCategory, err := normalizeExecutionErrorCategory(params.Category)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	normalizedCode, err := normalizeRequiredBoundedString("code", params.Code, maximumExecutionErrorCodeCharacters)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	normalizedSafeMessage, err := normalizeRequiredBoundedString("safeMessage", params.SafeMessage, maximumExecutionErrorSafeMessageCharacters)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	normalizedTechnicalDetail, err := normalizeOptionalBoundedString("technicalDetail", params.TechnicalDetail, maximumExecutionErrorTechnicalDetailCharacters)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	details, err := newJSONObject("details", params.Details)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return ExecutionErrorRecord{}, err
	}
	return ExecutionErrorRecord{params: ExecutionErrorRecordParams{ID: normalizedID, WorkflowExecutionID: normalizedWorkflowExecutionID, CompanyID: normalizedCompanyID, NodeExecutionID: normalizedNodeExecutionID, RelatedEventID: normalizedRelatedEventID, Category: normalizedCategory, Code: normalizedCode, SafeMessage: normalizedSafeMessage, TechnicalDetail: normalizedTechnicalDetail, Retryable: params.Retryable, Details: details.Bytes(), CreatedAt: createdAt}, hasNodeExecutionID: hasNodeExecutionID, hasRelatedEventID: hasRelatedEventID, details: details}, nil
}
func normalizeExecutionErrorCategory(category runtime.FailureCategory) (runtime.FailureCategory, error) {
	normalized := runtime.FailureCategory(strings.ToUpper(strings.TrimSpace(category.String())))
	if !normalized.IsValid() {
		return "", newValidationError("category", "must contain a supported failure category")
	}
	return normalized, nil
}
func (record ExecutionErrorRecord) ID() ExecutionErrorID {
	return record.params.ID
}
func (record ExecutionErrorRecord) WorkflowExecutionID() execution.WorkflowExecutionID {
	return record.params.WorkflowExecutionID
}
func (record ExecutionErrorRecord) CompanyID() workflow.CompanyID {
	return record.params.CompanyID
}
func (record ExecutionErrorRecord) NodeExecutionID() (execution.NodeExecutionID, bool) {
	if !record.hasNodeExecutionID {
		return "", false
	}
	return record.params.NodeExecutionID, true
}
func (record ExecutionErrorRecord) RelatedEventID() (ExecutionEventID, bool) {
	if !record.hasRelatedEventID {
		return "", false
	}
	return record.params.RelatedEventID, true
}
func (record ExecutionErrorRecord) Category() runtime.FailureCategory {
	return record.params.Category
}
func (record ExecutionErrorRecord) Code() string {
	return record.params.Code
}
func (record ExecutionErrorRecord) SafeMessage() string {
	return record.params.SafeMessage
}
func (record ExecutionErrorRecord) TechnicalDetail() (string, bool) {
	if record.params.TechnicalDetail == "" {
		return "", false
	}
	return record.params.TechnicalDetail, true
}
func (record ExecutionErrorRecord) Retryable() bool {
	return record.params.Retryable
}
func (record ExecutionErrorRecord) Details() JSONObject {
	return record.details
}
func (record ExecutionErrorRecord) CreatedAt() time.Time {
	return record.params.CreatedAt
}
func (record ExecutionErrorRecord) IsValid() bool {
	_, err := NewExecutionErrorRecord(record.params)
	return err == nil
}
