package runtime

import (
	"fmt"
	"sort"
)

func (executionContext *ExecutionContext) ApplyContextChanges(
	changes ContextChanges) error {
	if executionContext == nil {
		return newValidationError("executionContext", "must not be nil")
	}
	if err := changes.validate(); err != nil {
		return newValidationError("contextChanges",
			err.Error())
	}
	if changes.IsEmpty() {
		return nil
	}
	setKeys := changes.SetKeys()
	setValues := changes.SetValues()
	deleteKeys := changes.DeleteKeys()
	executionContext.variablesMu.Lock()
	defer executionContext.variablesMu.Unlock()
	for _, key := range deleteKeys {
		delete(executionContext.variables,
			key)
	}
	for _, key := range setKeys {
		value := setValues[key]
		executionContext.variables[key] = cloneRuntimeValue(value)
	}
	return nil
}

type ContextChanges struct {
	setValues   map[string]RuntimeValue
	setOrder    []string
	deleteKeys  map[string]struct{}
	deleteOrder []string
}

func NewContextChanges(setValues map[string]RuntimeValue, deleteKeys []string,
) (ContextChanges, error) {
	normalizedSetValues, setOrder, err := normalizeContextChangeSetValues(setValues)
	if err != nil {
		return ContextChanges{}, err
	}
	normalizedDeleteKeys, deleteOrder, err := normalizeContextChangeDeleteKeys(deleteKeys)
	if err != nil {
		return ContextChanges{}, err
	}
	for key := range normalizedSetValues {
		if _, exists := normalizedDeleteKeys[key]; exists {
			return ContextChanges{}, newValidationError("contextChanges", fmt.Sprintf(
				"key %q cannot be both set and deleted", key),
			)
		}
	}
	return ContextChanges{setValues: normalizedSetValues,
		setOrder: setOrder, deleteKeys: normalizedDeleteKeys, deleteOrder: deleteOrder,
	}, nil
}
func (changes ContextChanges) IsEmpty() bool {
	return len(changes.setValues) == 0 && len(changes.deleteKeys) == 0
}
func (changes ContextChanges) SetKeys() []string {
	if len(changes.setOrder) == 0 {
		return nil
	}
	return append([]string(nil),
		changes.setOrder...)
}
func (changes ContextChanges) SetValues() map[string]RuntimeValue {
	if len(changes.setValues) == 0 {
		return nil
	}
	values := make(map[string]RuntimeValue, len(changes.setValues))
	for key, value := range changes.setValues {
		values[key] = cloneRuntimeValue(value)
	}
	return values
}
func (changes ContextChanges) SetValue(key string) (RuntimeValue, bool, error) {
	normalizedKey, err := normalizeRuntimeVariableKey("contextChanges.key", key)
	if err != nil {
		return RuntimeValue{}, false, err
	}
	value, exists := changes.setValues[normalizedKey]
	if !exists {
		return RuntimeValue{}, false, nil
	}
	return cloneRuntimeValue(value), true, nil
}
func (changes ContextChanges) DeleteKeys() []string {
	if len(changes.deleteOrder) == 0 {
		return nil
	}
	return append([]string(nil), changes.deleteOrder...,
	)
}
func (changes ContextChanges) Deletes(key string) (bool, error) {
	normalizedKey, err := normalizeRuntimeVariableKey("contextChanges.key", key)
	if err != nil {
		return false, err
	}
	_, exists := changes.deleteKeys[normalizedKey]
	return exists, nil
}
func (changes ContextChanges) IsValid() bool {
	return changes.validate() == nil
}
func (changes ContextChanges) validate() error {
	normalized, err := NewContextChanges(changes.setValues, changes.deleteOrder)
	if err != nil {
		return err
	}
	if len(normalized.setValues) !=
		len(changes.setValues) {
		return newValidationError("contextChanges",
			"set values are not normalized")
	}
	if len(normalized.deleteKeys) != len(changes.deleteKeys) {
		return newValidationError("contextChanges", "delete keys are not normalized")
	}
	return nil
}
func cloneContextChanges(changes ContextChanges) ContextChanges {
	setValues := make(map[string]RuntimeValue, len(changes.setValues))
	for key, value := range changes.setValues {
		setValues[key] = cloneRuntimeValue(value)
	}
	deleteKeys := make(map[string]struct{}, len(changes.deleteKeys))
	for key := range changes.deleteKeys {
		deleteKeys[key] = struct{}{}
	}
	return ContextChanges{setValues: setValues, setOrder: append(
		[]string(nil), changes.setOrder...),
		deleteKeys: deleteKeys, deleteOrder: append([]string(nil),
			changes.deleteOrder...)}
}
func normalizeContextChangeSetValues(
	values map[string]RuntimeValue) (map[string]RuntimeValue,
	[]string, error) {
	if len(values) == 0 {
		return map[string]RuntimeValue{}, nil,
			nil
	}
	normalized := make(map[string]RuntimeValue, len(values))
	order := make(
		[]string, 0, len(values),
	)
	for key, value := range values {
		normalizedKey, err := normalizeRuntimeVariableKey("contextChanges.set.key",
			key)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := normalized[normalizedKey]; exists {
			return nil, nil, newValidationError("contextChanges.set",
				fmt.Sprintf("contains duplicate key %q after normalization", normalizedKey))
		}
		if !value.IsValid() {
			return nil, nil, newValidationError(
				"contextChanges.set.value", fmt.Sprintf("value for key %q must contain valid JSON",
					normalizedKey))
		}
		normalized[normalizedKey] =
			cloneRuntimeValue(value)
		order = append(
			order, normalizedKey)
	}
	sort.Strings(order)
	return normalized, order, nil
}
func normalizeContextChangeDeleteKeys(keys []string,
) (map[string]struct{}, []string,
	error) {
	if len(keys) == 0 {
		return map[string]struct{}{}, nil, nil
	}
	normalized := make(
		map[string]struct{}, len(keys))
	order := make([]string,
		0, len(keys))
	for _, key := range keys {
		normalizedKey, err :=
			normalizeRuntimeVariableKey("contextChanges.delete.key", key)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := normalized[normalizedKey]; exists {
			return nil, nil, newValidationError("contextChanges.delete", fmt.Sprintf(
				"contains duplicate key %q after normalization", normalizedKey),
			)
		}
		normalized[normalizedKey] = struct{}{}
		order = append(
			order, normalizedKey)
	}
	sort.Strings(order)
	return normalized, order, nil
}
