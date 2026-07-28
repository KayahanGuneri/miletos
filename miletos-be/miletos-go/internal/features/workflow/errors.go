package workflow

import "fmt"

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "workflow validation failed"
	}
	if e.Field == "" && e.Reason == "" {
		return "workflow validation failed"
	}
	if e.Field == "" {
		return fmt.Sprintf(
			"workflow validation failed: %s", e.Reason)
	}
	if e.Reason == "" {
		return fmt.Sprintf("workflow validation failed for %s", e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Field,
		e.Reason)
}
func newValidationError(field string,
	reason string) error {
	return &ValidationError{
		Field: field, Reason: reason}
}
