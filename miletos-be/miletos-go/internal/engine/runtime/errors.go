package runtime

import "fmt"

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "runtime validation failed"
	}
	if e.Field == "" && e.Reason == "" {
		return "runtime validation failed"
	}
	if e.Field == "" {
		return fmt.Sprintf(
			"runtime validation failed: %s", e.Reason)
	}
	if e.Reason == "" {
		return fmt.Sprintf("runtime validation failed for %s", e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Field,
		e.Reason)
}

type DecodeError struct {
	Source string
	Reason string
	Err    error
}

func (e *DecodeError) Error() string {
	if e == nil {
		return "runtime value decode failed"
	}
	if e.Source == "" && e.Reason == "" {
		return "runtime value decode failed"
	}
	if e.Source == "" {
		return fmt.Sprintf(
			"runtime value decode failed: %s", e.Reason)
	}
	if e.Reason == "" {
		return fmt.Sprintf("%s decode failed", e.Source)
	}
	return fmt.Sprintf("%s decode failed: %s", e.Source,
		e.Reason)
}
func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type AccessError struct {
	Resource   string
	Identifier string
	Reason     string
}

func (e *AccessError) Error() string {
	if e == nil {
		return "runtime access denied"
	}
	if e.Resource == "" &&
		e.Identifier == "" && e.Reason == "" {
		return "runtime access denied"
	}
	if e.Resource == "" {
		return fmt.Sprintf("runtime access denied: %s", e.Reason)
	}
	if e.Identifier == "" {
		if e.Reason == "" {
			return fmt.Sprintf(
				"runtime access denied for %s", e.Resource)
		}
		return fmt.Sprintf(
			"runtime access denied for %s: %s", e.Resource, e.Reason,
		)
	}
	if e.Reason == "" {
		return fmt.Sprintf("runtime access denied for %s %q",
			e.Resource, e.Identifier)
	}
	return fmt.Sprintf(
		"runtime access denied for %s %q: %s", e.Resource, e.Identifier,
		e.Reason)
}
func newValidationError(field string,
	reason string) error {
	return &ValidationError{
		Field: field, Reason: reason}
}
func newDecodeError(
	source string, reason string, err error,
) error {
	return &DecodeError{Source: source,
		Reason: reason, Err: err}
}
func newAccessError(
	resource string, identifier string, reason string,
) error {
	return &AccessError{Resource: resource,
		Identifier: identifier, Reason: reason}
}
