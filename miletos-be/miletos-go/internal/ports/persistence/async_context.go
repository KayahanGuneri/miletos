package repository

import (
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"time"
	"unicode/utf8"
)

const (
	MaximumAsyncContextValueBytes    = 1024 * 1024
	maximumAsyncContextKeyCharacters = 256
)

type AsyncContextVariableParams struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	Key                 string
	Value               runtime.RuntimeValue
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Version             int64
}
type AsyncContextVariable struct{ params AsyncContextVariableParams }

func NewAsyncContextVariable(params AsyncContextVariableParams) (AsyncContextVariable, error) {
	companyID, workflowExecutionID, err := normalizeAsyncExecutionScope(params.CompanyID, params.WorkflowExecutionID)
	if err != nil {
		return AsyncContextVariable{}, err
	}
	key, value, err := normalizeAsyncContextEntry(params.Key, params.Value)
	if err != nil {
		return AsyncContextVariable{}, err
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return AsyncContextVariable{}, err
	}
	updatedAt, err := normalizeRequiredRecordTime("updatedAt", params.UpdatedAt)
	if err != nil {
		return AsyncContextVariable{}, err
	}
	if updatedAt.Before(createdAt) {
		return AsyncContextVariable{}, newValidationError("updatedAt", "must not be before createdAt")
	}
	if params.Version <= 0 {
		return AsyncContextVariable{}, newValidationError("version", "must be greater than zero")
	}
	return AsyncContextVariable{params: AsyncContextVariableParams{CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, Key: key, Value: value, CreatedAt: createdAt, UpdatedAt: updatedAt, Version: params.Version}}, nil
}
func normalizeAsyncContextEntry(key string, value runtime.RuntimeValue) (string, runtime.RuntimeValue, error) {
	if !value.IsValid() {
		return "", runtime.RuntimeValue{}, newValidationError("value", "must contain valid JSON")
	}
	if len(value.Bytes()) > MaximumAsyncContextValueBytes {
		return "", runtime.RuntimeValue{}, newValidationError("value", "exceeds the maximum async context value size")
	}
	changes, err := runtime.NewContextChanges(map[string]runtime.RuntimeValue{key: value}, nil)
	if err != nil {
		return "", runtime.RuntimeValue{}, newValidationError("key", err.Error())
	}
	keys := changes.SetKeys()
	if len(keys) != 1 {
		return "", runtime.RuntimeValue{}, newValidationError("key", "must identify exactly one context variable")
	}
	if utf8.RuneCountInString(keys[0]) > maximumAsyncContextKeyCharacters {
		return "", runtime.RuntimeValue{}, newValidationError("key", "must not exceed 256 characters")
	}
	normalizedValue, exists, err := changes.SetValue(keys[0])
	if err != nil || !exists {
		return "", runtime.RuntimeValue{}, newValidationError("value", "must be available after validation")
	}
	return keys[0], normalizedValue, nil
}
func (variable AsyncContextVariable) CompanyID() workflow.CompanyID {
	return variable.params.CompanyID
}
func (variable AsyncContextVariable) WorkflowExecutionID() execution.WorkflowExecutionID {
	return variable.params.WorkflowExecutionID
}
func (variable AsyncContextVariable) Key() string {
	return variable.params.Key
}
func (variable AsyncContextVariable) Value() runtime.RuntimeValue {
	value, _ := runtime.NewRuntimeValue(variable.params.Value.Bytes())
	return value
}
func (variable AsyncContextVariable) CreatedAt() time.Time {
	return variable.params.CreatedAt
}
func (variable AsyncContextVariable) UpdatedAt() time.Time {
	return variable.params.UpdatedAt
}
func (variable AsyncContextVariable) Version() int64 {
	return variable.params.Version
}
func (variable AsyncContextVariable) IdentityKey() string {
	return deterministicAsyncIdentity("context", variable.params.CompanyID.String(), variable.params.WorkflowExecutionID.String(), variable.params.Key)
}
func (variable AsyncContextVariable) IsValid() bool {
	_, err := NewAsyncContextVariable(variable.params)
	return err == nil
}

type AsyncContextWrite struct {
	variable        AsyncContextVariable
	expectedVersion int64
}

func NewAsyncContextWrite(variable AsyncContextVariable, expectedVersion int64) (AsyncContextWrite, error) {
	if !variable.IsValid() {
		return AsyncContextWrite{}, newValidationError("variable", "must be valid")
	}
	if expectedVersion < 0 {
		return AsyncContextWrite{}, newValidationError("expectedVersion", "must not be negative")
	}
	expectedNewVersion := int64(1)
	if expectedVersion > 0 {
		expectedNewVersion = expectedVersion + 1
	}
	if variable.Version() != expectedNewVersion {
		return AsyncContextWrite{}, newValidationError("version", "must be 1 for insert or exactly expectedVersion + 1 for update")
	}
	return AsyncContextWrite{variable: variable, expectedVersion: expectedVersion}, nil
}
func (write AsyncContextWrite) Variable() AsyncContextVariable {
	return write.variable
}
func (write AsyncContextWrite) ExpectedVersion() int64 {
	return write.expectedVersion
}
func (write AsyncContextWrite) IsInsert() bool {
	return write.expectedVersion == 0
}
func (write AsyncContextWrite) IsValid() bool {
	_, err := NewAsyncContextWrite(write.variable, write.expectedVersion)
	return err == nil
}

type AsyncContextDelete struct {
	companyID           workflow.CompanyID
	workflowExecutionID execution.WorkflowExecutionID
	key                 string
	expectedVersion     int64
	deletedAt           time.Time
}

func NewAsyncContextDelete(companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, key string, expectedVersion int64, deletedAt time.Time) (AsyncContextDelete, error) {
	normalizedCompanyID, normalizedExecutionID, err := normalizeAsyncExecutionScope(companyID, workflowExecutionID)
	if err != nil {
		return AsyncContextDelete{}, err
	}
	placeholder, err := runtime.NewRuntimeValue([]byte("null"))
	if err != nil {
		return AsyncContextDelete{}, err
	}
	normalizedKey, _, err := normalizeAsyncContextEntry(key, placeholder)
	if err != nil {
		return AsyncContextDelete{}, err
	}
	if expectedVersion <= 0 {
		return AsyncContextDelete{}, newValidationError("expectedVersion", "must be greater than zero for delete")
	}
	normalizedDeletedAt, err := normalizeRequiredRecordTime("deletedAt", deletedAt)
	if err != nil {
		return AsyncContextDelete{}, err
	}
	return AsyncContextDelete{companyID: normalizedCompanyID, workflowExecutionID: normalizedExecutionID, key: normalizedKey, expectedVersion: expectedVersion, deletedAt: normalizedDeletedAt}, nil
}
func (deletion AsyncContextDelete) CompanyID() workflow.CompanyID {
	return deletion.companyID
}
func (deletion AsyncContextDelete) WorkflowExecutionID() execution.WorkflowExecutionID {
	return deletion.workflowExecutionID
}
func (deletion AsyncContextDelete) Key() string {
	return deletion.key
}
func (deletion AsyncContextDelete) ExpectedVersion() int64 {
	return deletion.expectedVersion
}
func (deletion AsyncContextDelete) DeletedAt() time.Time {
	return deletion.deletedAt
}
func (deletion AsyncContextDelete) IsValid() bool {
	_, err := NewAsyncContextDelete(deletion.companyID, deletion.workflowExecutionID, deletion.key, deletion.expectedVersion, deletion.deletedAt)
	return err == nil
}
