package plugin

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	conditionEquals             = "EQUALS"
	conditionNotEquals          = "NOT_EQUALS"
	conditionGreaterThan        = "GREATER_THAN"
	conditionGreaterThanOrEqual = "GREATER_THAN_OR_EQUAL"
	conditionLessThan           = "LESS_THAN"
	conditionLessThanOrEqual    = "LESS_THAN_OR_EQUAL"
)

var conditionFieldSegmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type conditionConfiguration struct {
	Field    string
	Operator string
	Value    any
}

func ifRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                 "core.if",
		InputMode:           NodeInputSingle,
		InputPorts:          standardInputPorts(),
		OutputPorts:         []Port{{Name: "yes"}, {Name: "no"}},
		InputEdgeConstraint: fixedEdgeConstraint(1),
		RoutingMode:         OutputRoutingExplicit,
		Validator:           validateConditionConfiguration,
		Handler:             ifNodeHandler,
	}
}

func filterRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                 "core.filter",
		InputMode:           NodeInputSingle,
		InputPorts:          standardInputPorts(),
		OutputPorts:         standardOutputPorts(),
		InputEdgeConstraint: fixedEdgeConstraint(1),
		RoutingMode:         OutputRoutingExplicit,
		Validator:           validateConditionConfiguration,
		Handler:             filterNodeHandler,
	}
}

func ifNodeHandler(nodeContext *Context) error {
	nodeContext.Lifecycles.OnRun(func() (any, error) {
		return ifNode(nodeContext)
	})
	return nil
}

func filterNodeHandler(nodeContext *Context) error {
	nodeContext.Lifecycles.OnRun(func() (any, error) {
		return filterNode(nodeContext)
	})
	return nil
}

func validateConditionConfiguration(configuration map[string]any) error {
	_, err := parseConditionConfiguration(configuration)
	return err
}

func parseConditionConfiguration(configuration map[string]any) (conditionConfiguration, error) {
	field, ok := configuration["field"].(string)
	if !ok || strings.TrimSpace(field) == "" {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_FIELD_REQUIRED", "configuration.field is required",
		)
	}
	field = strings.TrimSpace(field)
	if !validConditionFieldPath(field) {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_FIELD_INVALID", "configuration.field must be a simple dot-separated payload field path",
		)
	}
	operator, ok := configuration["operator"].(string)
	if !ok {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_OPERATOR_REQUIRED", "configuration.operator is required",
		)
	}
	operator = strings.TrimSpace(operator)
	if operator == "" {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_OPERATOR_REQUIRED", "configuration.operator is required",
		)
	}
	if !supportedConditionOperator(operator) {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_OPERATOR_INVALID", "configuration.operator is not supported",
		)
	}
	value, exists := configuration["value"]
	if !exists || value == nil {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_VALUE_REQUIRED", "configuration.value is required",
		)
	}
	if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_VALUE_REQUIRED", "configuration.value is required",
		)
	}
	if !flatConditionValue(value) {
		return conditionConfiguration{}, validationNodeError(
			"CONDITION_VALUE_INVALID", "configuration.value must be a flat primitive value",
		)
	}
	return conditionConfiguration{Field: field, Operator: operator, Value: value}, nil
}

func validConditionFieldPath(field string) bool {
	for _, segment := range strings.Split(field, ".") {
		if !conditionFieldSegmentPattern.MatchString(segment) {
			return false
		}
	}
	return true
}

func supportedConditionOperator(operator string) bool {
	switch operator {
	case conditionEquals, conditionNotEquals, conditionGreaterThan,
		conditionGreaterThanOrEqual, conditionLessThan, conditionLessThanOrEqual:
		return true
	default:
		return false
	}
}

func flatConditionValue(value any) bool {
	switch value.(type) {
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	case json.Number, float32, float64:
		_, ok := conditionNumber(value)
		return ok
	default:
		return false
	}
}

func ifNode(nodeContext *Context) (any, error) {
	matched, err := evaluateConfiguredCondition(nodeContext.configuration, nodeContext.Payload)
	if err != nil {
		return nil, err
	}
	selectedPort := "no"
	if matched {
		selectedPort = "yes"
	}
	if err := routeConditionPayload(nodeContext, selectedPort); err != nil {
		return nil, err
	}
	return nodeContext.Payload, nil
}

func filterNode(nodeContext *Context) (any, error) {
	matched, err := evaluateConfiguredCondition(nodeContext.configuration, nodeContext.Payload)
	if err != nil {
		return nil, err
	}
	if matched {
		if err := routeConditionPayload(nodeContext, "output"); err != nil {
			return nil, err
		}
	}
	return nodeContext.Payload, nil
}

func routeConditionPayload(nodeContext *Context, selectedPort string) error {
	for index := 0; index < nodeContext.Access.GetOutputEdgeCount(); index++ {
		edge, exists := nodeContext.Access.GetEdgeData(index)
		if !exists {
			return &NodeError{
				Category: "INTERNAL",
				Code:     "CONDITION_ROUTE_INVALID",
				Message:  "The selected condition route could not be resolved.",
			}
		}
		if edge.SourceOutputPort != selectedPort {
			continue
		}
		if err := nodeContext.Access.PushEdge(index, nodeContext.Payload); err != nil {
			return &NodeError{
				Category: "EXECUTION",
				Code:     "CONDITION_ROUTE_FAILED",
				Message:  "The selected condition route could not be emitted.",
				Cause:    err,
			}
		}
	}
	return nil
}

func evaluateConfiguredCondition(configuration map[string]any, input any) (bool, error) {
	condition, err := parseConditionConfiguration(configuration)
	if err != nil {
		return false, err
	}
	payload, ok := input.(map[string]any)
	if !ok || payload == nil {
		return false, validationNodeError(
			"CONDITION_INPUT_INVALID", "Condition nodes require a JSON object payload.",
		)
	}
	actual, err := resolveConditionField(payload, condition.Field)
	if err != nil {
		return false, err
	}
	return compareConditionValue(actual, condition)
}

func resolveConditionField(payload map[string]any, fieldPath string) (any, error) {
	current := payload
	segments := strings.Split(fieldPath, ".")
	for index, segment := range segments {
		actual, exists := current[segment]
		if !exists {
			return nil, validationNodeError(
				"CONDITION_FIELD_MISSING", "The configured top-level field is missing from the payload.",
			)
		}
		if actual == nil {
			return nil, validationNodeError(
				"CONDITION_FIELD_NULL", "The configured top-level field must not be null.",
			)
		}
		if index == len(segments)-1 {
			return actual, nil
		}
		nested, ok := actual.(map[string]any)
		if !ok {
			return nil, conditionRuntimeError(
				"The configured field path must traverse JSON object values.",
			)
		}
		current = nested
	}
	return nil, validationNodeError(
		"CONDITION_FIELD_MISSING", "The configured top-level field is missing from the payload.",
	)
}

func compareConditionValue(actual any, condition conditionConfiguration) (bool, error) {
	switch typed := actual.(type) {
	case string:
		expected, ok := condition.Value.(string)
		if !ok {
			if _, numeric := conditionNumber(condition.Value); !numeric {
				return false, conditionRuntimeError("String fields require a configured string value.")
			}
			actualNumber, numeric := conditionStringNumber(typed)
			if !numeric {
				return false, conditionRuntimeError(
					"Numeric comparisons require a finite numeric field value.",
				)
			}
			return compareConditionValue(actualNumber, condition)
		}
		switch condition.Operator {
		case conditionEquals:
			return typed == expected, nil
		case conditionNotEquals:
			return typed != expected, nil
		case conditionGreaterThan, conditionGreaterThanOrEqual,
			conditionLessThan, conditionLessThanOrEqual:
			actualNumber, actualNumeric := conditionStringNumber(typed)
			expectedNumber, expectedNumeric := conditionStringNumber(expected)
			if !actualNumeric || !expectedNumeric {
				return false, conditionRuntimeError(
					"Relational comparisons require finite numeric field and configured values.",
				)
			}
			numericCondition := condition
			numericCondition.Value = expectedNumber
			return compareConditionValue(actualNumber, numericCondition)
		default:
			return false, conditionRuntimeError("The configured string operator is invalid.")
		}
	case bool:
		expected, ok := conditionBoolean(condition.Value)
		if !ok {
			return false, conditionRuntimeError("Boolean fields require a configured true or false value.")
		}
		switch condition.Operator {
		case conditionEquals:
			return typed == expected, nil
		case conditionNotEquals:
			return typed != expected, nil
		default:
			return false, conditionRuntimeError("Boolean fields support only EQUALS and NOT_EQUALS.")
		}
	default:
		actualNumber, ok := conditionNumber(actual)
		if !ok {
			return false, conditionRuntimeError(
				"The configured field must contain a string, number, or boolean value.",
			)
		}
		expectedNumber, ok := configuredConditionNumber(condition.Value)
		if !ok {
			return false, conditionRuntimeError("Numeric fields require a finite configured number.")
		}
		switch condition.Operator {
		case conditionEquals:
			return actualNumber == expectedNumber, nil
		case conditionNotEquals:
			return actualNumber != expectedNumber, nil
		case conditionGreaterThan:
			return actualNumber > expectedNumber, nil
		case conditionGreaterThanOrEqual:
			return actualNumber >= expectedNumber, nil
		case conditionLessThan:
			return actualNumber < expectedNumber, nil
		case conditionLessThanOrEqual:
			return actualNumber <= expectedNumber, nil
		default:
			return false, conditionRuntimeError("The configured numeric operator is invalid.")
		}
	}
}

func configuredConditionNumber(value any) (float64, bool) {
	if text, ok := value.(string); ok {
		return conditionStringNumber(text)
	}
	return conditionNumber(value)
}

func conditionStringNumber(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func conditionNumber(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int8:
		number = float64(typed)
	case int16:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint8:
		number = float64(typed)
	case uint16:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func conditionBoolean(value any) (bool, bool) {
	if typed, ok := value.(bool); ok {
		return typed, true
	}
	text, ok := value.(string)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func conditionRuntimeError(message string) *NodeError {
	return validationNodeError("CONDITION_COMPARISON_INVALID", message)
}
