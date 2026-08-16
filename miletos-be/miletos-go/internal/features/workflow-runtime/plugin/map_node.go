package plugin

import (
	"encoding/json"
	"math"
	"strings"
)

func mapRegistration() NodeRegistration {
	maximumIncoming := uint(1)
	return NodeRegistration{
		Key:         "core.map",
		InputMode:   NodeInputSingle,
		InputPorts:  standardInputPorts(),
		OutputPorts: standardOutputPorts(),
		InputEdgeConstraint: EdgeConstraint{
			Maximum: &maximumIncoming,
		},
		OutputEdgeConstraint: EdgeConstraint{},
		RoutingMode:          OutputRoutingBroadcast,
		Validator:            validateMap,
		Handler:              onRunHandler(mapNode),
	}
}

type fieldMapping struct {
	SourceField string
	TargetField string
}

type literalAssignment struct {
	TargetField string
	Value       any
}

func validateMap(configuration map[string]any) error {
	mappings, err := mapFieldMappings(configuration)
	if err != nil {
		return err
	}
	assignments, err := mapLiteralAssignments(configuration)
	if err != nil {
		return err
	}
	if len(mappings) == 0 && len(assignments) == 0 {
		return validationNodeError(
			"MAP_OPERATIONS_REQUIRED",
			"configuration.mappings or configuration.assignments must contain at least one operation",
		)
	}
	sources := make(map[string]struct{}, len(mappings))
	targets := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping.SourceField == "" {
			return validationNodeError("MAP_SOURCE_FIELD_REQUIRED", "configuration.mappings.sourceField is required")
		}
		if mapping.TargetField == "" {
			return validationNodeError("MAP_TARGET_FIELD_REQUIRED", "configuration.mappings.targetField is required")
		}
		if _, exists := sources[mapping.SourceField]; exists {
			return validationNodeError(
				"MAP_SOURCE_FIELD_DUPLICATE", "configuration.mappings.sourceField must be unique",
			)
		}
		if _, exists := targets[mapping.TargetField]; exists {
			return validationNodeError(
				"MAP_TARGET_FIELD_DUPLICATE", "configuration.mappings.targetField must be unique",
			)
		}
		sources[mapping.SourceField] = struct{}{}
		targets[mapping.TargetField] = struct{}{}
	}
	for _, assignment := range assignments {
		if assignment.TargetField == "" {
			return validationNodeError(
				"MAP_ASSIGNMENT_TARGET_REQUIRED",
				"configuration.assignments.targetField is required",
			)
		}
		if _, exists := targets[assignment.TargetField]; exists {
			return validationNodeError(
				"MAP_TARGET_FIELD_DUPLICATE",
				"mapping and assignment target fields must be unique",
			)
		}
		targets[assignment.TargetField] = struct{}{}
	}
	return nil
}

func mapNode(nodeContext *Context) (any, error) {
	if err := validateMap(nodeContext.configuration); err != nil {
		return nil, err
	}
	row, err := objectPayload(
		nodeContext.Payload, "MAP_INPUT_INVALID", "Map requires a JSON object payload.",
	)
	if err != nil {
		return nil, err
	}
	mappings, err := mapFieldMappings(nodeContext.configuration)
	if err != nil {
		return nil, err
	}
	assignments, err := mapLiteralAssignments(nodeContext.configuration)
	if err != nil {
		return nil, err
	}
	mapped := applyFieldMappings(row, mappings)
	for _, assignment := range assignments {
		mapped[assignment.TargetField] = assignment.Value
	}
	return mapped, nil
}

func applyFieldMappings(row map[string]any, mappings []fieldMapping) map[string]any {
	original := cloneObject(row)
	mapped := cloneObject(row)
	targets := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		targets[mapping.TargetField] = struct{}{}
	}
	for _, mapping := range mappings {
		value, exists := original[mapping.SourceField]
		if !exists {
			continue
		}
		mapped[mapping.TargetField] = value
		if mapping.SourceField != mapping.TargetField {
			if _, targetCollision := targets[mapping.SourceField]; !targetCollision {
				delete(mapped, mapping.SourceField)
			}
		}
	}
	return mapped
}

func mapFieldMappings(configuration map[string]any) ([]fieldMapping, error) {
	raw, exists := configuration["mappings"]
	if !exists || raw == nil {
		return nil, nil
	}
	items, err := configurationObjectList(raw, "MAP_MAPPINGS_INVALID")
	if err != nil {
		return nil, err
	}
	mappings := make([]fieldMapping, 0, len(items))
	for _, item := range items {
		mappings = append(mappings, fieldMapping{
			SourceField: strings.TrimSpace(configString(item, "sourceField")),
			TargetField: strings.TrimSpace(configString(item, "targetField")),
		})
	}
	return mappings, nil
}

func mapLiteralAssignments(configuration map[string]any) ([]literalAssignment, error) {
	raw, exists := configuration["assignments"]
	if !exists || raw == nil {
		return nil, nil
	}
	items, err := configurationObjectList(raw, "MAP_ASSIGNMENTS_INVALID")
	if err != nil {
		return nil, err
	}
	assignments := make([]literalAssignment, 0, len(items))
	for _, item := range items {
		value, exists := item["value"]
		if !exists || !isJSONLiteral(value) {
			return nil, validationNodeError(
				"MAP_ASSIGNMENT_VALUE_INVALID",
				"configuration.assignments.value must be a string, finite number, boolean, or null",
			)
		}
		assignments = append(assignments, literalAssignment{
			TargetField: strings.TrimSpace(configString(item, "targetField")),
			Value:       value,
		})
	}
	return assignments, nil
}

func isJSONLiteral(value any) bool {
	switch value := value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	case json.Number:
		_, err := value.Float64()
		return err == nil
	case float32:
		return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	default:
		return false
	}
}
