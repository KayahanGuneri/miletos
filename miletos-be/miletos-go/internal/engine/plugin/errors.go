package plugin

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "plugin validation failed"
	}
	if e.Field == "" && e.Reason == "" {
		return "plugin validation failed"
	}
	if e.Field == "" {
		return fmt.Sprintf("plugin validation failed: %s", e.Reason)
	}
	if e.Reason == "" {
		return fmt.Sprintf("plugin validation failed for %s",
			e.Field)
	}
	return fmt.Sprintf("%s: %s",
		e.Field, e.Reason)
}
func newValidationError(
	field string, reason string) error {
	return &ValidationError{Field: field, Reason: reason}
}
func normalizeRequiredString(field string, value string,
) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field, "must not be empty")
	}
	return normalized, nil
}
func normalizeOptionalString(value string) string {
	return strings.TrimSpace(value)
}
