package execution

import "fmt"

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "execution validation failed"
	}
	if e.Field == "" && e.Reason == "" {
		return "execution validation failed"
	}
	if e.Field == "" {
		return fmt.Sprintf("execution validation failed: %s", e.Reason)
	}
	if e.Reason == "" {
		return fmt.Sprintf("execution validation failed for %s",
			e.Field)
	}
	return fmt.Sprintf("%s: %s",
		e.Field, e.Reason)
}

type InvalidModeError struct {
	Value string
}

func (e *InvalidModeError) Error() string {
	if e == nil {
		return "invalid execution mode"
	}
	return fmt.Sprintf(
		"invalid execution mode %q: must be one of SYNC, ASYNC", e.Value)
}

type TransitionError struct {
	Entity        string
	CurrentStatus string
	TargetStatus  string
}

func (e *TransitionError) Error() string {
	if e == nil {
		return "execution transition failed"
	}
	return fmt.Sprintf("%s cannot transition from %s to %s",
		e.Entity, e.CurrentStatus, e.TargetStatus,
	)
}

type TimestampError struct {
	Field  string
	Reason string
}

func (e *TimestampError) Error() string {
	if e == nil {
		return "invalid execution timestamp"
	}
	if e.Field == "" {
		return fmt.Sprintf(
			"invalid execution timestamp: %s", e.Reason)
	}
	return fmt.Sprintf(
		"%s: %s", e.Field, e.Reason,
	)
}
func newValidationError(field string, reason string,
) error {
	return &ValidationError{Field: field,
		Reason: reason}
}
func newTimestampError(field string,
	reason string) error {
	return &TimestampError{
		Field: field, Reason: reason}
}
