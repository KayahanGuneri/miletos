package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

const (
	mergeStateKey            = "merge-latest-input-state-v1"
	mergeEmissionAnySource   = "ANY_SOURCE"
	mergeEmissionPrimaryOnly = "PRIMARY_ONLY"
)

type mergePersistedState struct {
	Sources map[string]any `json:"sources"`
}

type mergeMapping struct {
	Key          string
	SourceNodeID string
	Correlation  *mergeCorrelation
}

type mergeCorrelation struct {
	PrimaryField string
	SourceField  string
}

type correlationKey struct {
	Kind  string
	Value string
}

func mergeRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                  "core.merge",
		InputMode:            NodeInputMulti,
		TriggerInputMode:     TriggerInputAvailable,
		InputPorts:           standardInputPorts(),
		OutputPorts:          standardOutputPorts(),
		InputEdgeConstraint:  EdgeConstraint{Minimum: 2},
		OutputEdgeConstraint: EdgeConstraint{},
		RoutingMode:          OutputRoutingExplicit,
		Validator:            validateMerge,
		RecordSourceRelation: mergeRecordSourceRelation,
		Handler:              onRunHandler(mergeNode),
	}
}

func mergeRecordSourceRelation(
	configuration map[string]any,
	sourceNodeID string,
) RecordSourceRelation {
	if strings.TrimSpace(configString(configuration, "primaryInputNodeId")) == sourceNodeID {
		return RecordSourceRelationDriving
	}
	mappings, err := mergeMappings(configuration)
	if err != nil {
		return RecordSourceRelationUnsupported
	}
	for _, mapping := range mappings {
		if mapping.SourceNodeID == sourceNodeID && mapping.Correlation != nil {
			return RecordSourceRelationPassive
		}
	}
	return RecordSourceRelationUnsupported
}

func validateMerge(configuration map[string]any) error {
	primaryInputNodeID := strings.TrimSpace(configString(configuration, "primaryInputNodeId"))
	if primaryInputNodeID == "" {
		return validationNodeError(
			"MERGE_PRIMARY_INPUT_REQUIRED", "configuration.primaryInputNodeId is required",
		)
	}
	if _, err := mergeEmissionMode(configuration); err != nil {
		return err
	}
	mappings, err := mergeMappings(configuration)
	if err != nil {
		return err
	}
	if len(mappings) == 0 {
		return validationNodeError(
			"MERGE_MAPPINGS_REQUIRED", "configuration.mappings must contain at least one mapping",
		)
	}
	keys := make(map[string]struct{}, len(mappings))
	sources := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping.Key == "" {
			return validationNodeError("MERGE_MAPPING_KEY_REQUIRED", "configuration.mappings.key is required")
		}
		if mapping.SourceNodeID == "" {
			return validationNodeError(
				"MERGE_MAPPING_SOURCE_REQUIRED", "configuration.mappings.sourceNodeId is required",
			)
		}
		if mapping.SourceNodeID == primaryInputNodeID {
			return validationNodeError(
				"MERGE_MAPPING_SOURCE_PRIMARY", "configuration.mappings.sourceNodeId cannot be the primary input",
			)
		}
		if _, exists := keys[mapping.Key]; exists {
			return validationNodeError(
				"MERGE_MAPPING_KEY_DUPLICATE", "configuration.mappings.key must be unique",
			)
		}
		if _, exists := sources[mapping.SourceNodeID]; exists {
			return validationNodeError(
				"MERGE_MAPPING_SOURCE_DUPLICATE", "configuration.mappings.sourceNodeId must be unique",
			)
		}
		if mapping.Correlation != nil {
			if mapping.Correlation.PrimaryField == "" {
				return validationNodeError(
					"MERGE_CORRELATION_PRIMARY_FIELD_REQUIRED",
					"configuration.mappings.correlation.primaryField is required",
				)
			}
			if strings.Contains(mapping.Correlation.PrimaryField, ".") {
				return validationNodeError(
					"MERGE_CORRELATION_PRIMARY_FIELD_INVALID",
					"configuration.mappings.correlation.primaryField must name one top-level field",
				)
			}
			if !databaseCursorColumn.MatchString(mapping.Correlation.SourceField) {
				return validationNodeError(
					"MERGE_CORRELATION_SOURCE_FIELD_INVALID",
					"configuration.mappings.correlation.sourceField must be a simple returned column name",
				)
			}
		}
		keys[mapping.Key] = struct{}{}
		sources[mapping.SourceNodeID] = struct{}{}
	}
	return nil
}

func mergeNode(nodeContext *Context) (any, error) {
	if err := validateMerge(nodeContext.configuration); err != nil {
		return nil, err
	}
	primaryInputNodeID := strings.TrimSpace(configString(nodeContext.configuration, "primaryInputNodeId"))
	emissionMode, err := mergeEmissionMode(nodeContext.configuration)
	if err != nil {
		return nil, err
	}
	mappings, err := mergeMappings(nodeContext.configuration)
	if err != nil {
		return nil, err
	}
	requiredSources := mergeRequiredSources(primaryInputNodeID, mappings)
	availableSources, err := mergeInputsBySource(nodeContext.Payload, requiredSources)
	if err != nil {
		return nil, err
	}
	if len(availableSources) == 0 {
		return nil, nil
	}
	_, primaryArrivedNow := availableSources[primaryInputNodeID]
	updated, err := nodeContext.Storage.Update(
		mergeStateKey,
		func(current any, exists bool) (any, error) {
			state := mergePersistedState{Sources: make(map[string]any)}
			if exists {
				decoded, decodeErr := decodeMergeState(current)
				if decodeErr != nil {
					return nil, decodeErr
				}
				state = decoded
			}
			nextSources := make(map[string]any, len(state.Sources)+len(availableSources))
			for sourceNodeID, value := range state.Sources {
				nextSources[sourceNodeID] = value
			}
			for sourceNodeID, value := range availableSources {
				nextSources[sourceNodeID] = value
			}
			return mergePersistedState{Sources: nextSources}, nil
		},
	)
	if err != nil {
		var nodeError *NodeError
		if errors.As(err, &nodeError) {
			return nil, err
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     "MERGE_STATE_UPDATE_FAILED",
			Message:  "Merge input state could not be updated.",
			CanRetry: true,
			Cause:    err,
		}
	}
	state, ok := updated.(mergePersistedState)
	if ok && state.Sources == nil {
		return nil, invalidMergeState(nil)
	}
	if !ok {
		state, err = decodeMergeState(updated)
		if err != nil {
			return nil, err
		}
	}
	if emissionMode == mergeEmissionPrimaryOnly && !primaryArrivedNow {
		return nil, nil
	}
	if !mergeStateComplete(state, requiredSources) {
		return nil, nil
	}
	resolvedSources, err := resolveMergeCorrelations(nodeContext, primaryInputNodeID, mappings, state.Sources)
	if err != nil {
		return nil, err
	}
	output, err := composeMergeOutput(primaryInputNodeID, mappings, resolvedSources)
	if err != nil {
		return nil, err
	}
	if err := routeMergeOutput(nodeContext, output); err != nil {
		return nil, err
	}
	return output, nil
}

func mergeEmissionMode(configuration map[string]any) (string, error) {
	raw, exists := configuration["emissionMode"]
	if !exists || raw == nil {
		return mergeEmissionAnySource, nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", validationNodeError(
			"MERGE_EMISSION_MODE_INVALID",
			"configuration.emissionMode must be ANY_SOURCE or PRIMARY_ONLY",
		)
	}
	mode := strings.ToUpper(strings.TrimSpace(value))
	if mode == "" {
		return mergeEmissionAnySource, nil
	}
	switch mode {
	case mergeEmissionAnySource, mergeEmissionPrimaryOnly:
		return mode, nil
	default:
		return "", validationNodeError(
			"MERGE_EMISSION_MODE_INVALID",
			"configuration.emissionMode must be ANY_SOURCE or PRIMARY_ONLY",
		)
	}
}

func mergeRequiredSources(primaryInputNodeID string, mappings []mergeMapping) map[string]struct{} {
	sources := make(map[string]struct{}, len(mappings)+1)
	sources[primaryInputNodeID] = struct{}{}
	for _, mapping := range mappings {
		if mapping.Correlation == nil {
			sources[mapping.SourceNodeID] = struct{}{}
		}
	}
	return sources
}

func decodeMergeState(value any) (mergePersistedState, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return mergePersistedState{}, invalidMergeState(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var state mergePersistedState
	if err := decoder.Decode(&state); err != nil || state.Sources == nil {
		return mergePersistedState{}, invalidMergeState(err)
	}
	return state, nil
}

func invalidMergeState(cause error) error {
	return &NodeError{
		Category: "INTERNAL",
		Code:     "MERGE_STATE_INVALID",
		Message:  "The persisted Merge input state is invalid.",
		Cause:    cause,
	}
}

func mergeStateComplete(state mergePersistedState, requiredSources map[string]struct{}) bool {
	for sourceNodeID := range requiredSources {
		if _, present := state.Sources[sourceNodeID]; !present {
			return false
		}
	}
	return true
}

func composeMergeOutput(
	primaryInputNodeID string,
	mappings []mergeMapping,
	sources map[string]any,
) (map[string]any, error) {
	primary, exists := sources[primaryInputNodeID]
	if !exists {
		return nil, validationNodeError(
			"MERGE_PRIMARY_INPUT_MISSING", "Merge primary input is not present in the available inputs.",
		)
	}
	primaryObject, ok := primary.(map[string]any)
	if !ok {
		return nil, validationNodeError(
			"MERGE_PRIMARY_INPUT_INVALID", "Merge primary input must be a JSON object.",
		)
	}
	result := cloneObject(primaryObject)
	for _, mapping := range mappings {
		if _, collision := result[mapping.Key]; collision {
			return nil, validationNodeError(
				"MERGE_MAPPING_KEY_COLLISION", "A Merge mapping key collides with a primary input field.",
			)
		}
		value, exists := sources[mapping.SourceNodeID]
		if !exists {
			return nil, validationNodeError(
				"MERGE_MAPPING_SOURCE_MISSING", "A configured Merge source is not present in the available inputs.",
			)
		}
		result[mapping.Key] = value
	}
	return result, nil
}

func resolveMergeCorrelations(
	nodeContext *Context,
	primaryInputNodeID string,
	mappings []mergeMapping,
	sources map[string]any,
) (map[string]any, error) {
	resolved := make(map[string]any, len(sources))
	for sourceNodeID, value := range sources {
		resolved[sourceNodeID] = value
	}
	primary, exists := resolved[primaryInputNodeID]
	if !exists {
		return nil, validationNodeError(
			"MERGE_PRIMARY_INPUT_MISSING",
			"Merge primary input is not present in the available inputs.",
		)
	}
	primaryObject, ok := primary.(map[string]any)
	if !ok {
		return nil, validationNodeError(
			"MERGE_PRIMARY_INPUT_INVALID",
			"Merge primary input must be a JSON object.",
		)
	}
	accessor, ok := nodeContext.Access.(InputNodeAccessor)
	if !ok {
		return nil, &NodeError{
			Category: "INTERNAL",
			Code:     "MERGE_CORRELATION_SOURCE_UNAVAILABLE",
			Message:  "Merge correlation source metadata is unavailable.",
		}
	}
	for _, mapping := range mappings {
		if mapping.Correlation == nil {
			continue
		}
		primaryValue, exists := primaryObject[mapping.Correlation.PrimaryField]
		if !exists {
			return nil, validationNodeError(
				"MERGE_CORRELATION_PRIMARY_FIELD_MISSING",
				"The configured Merge primary correlation field is missing.",
			)
		}
		if primaryValue == nil {
			return nil, validationNodeError(
				"MERGE_CORRELATION_PRIMARY_FIELD_NULL",
				"The configured Merge primary correlation field must not be null.",
			)
		}
		source, exists := accessor.GetInputNode(mapping.SourceNodeID)
		if !exists || source.PluginType != "core.database-input" {
			return nil, validationNodeError(
				"MERGE_CORRELATION_SOURCE_INVALID",
				"Merge correlation requires a connected Database Input source.",
			)
		}
		value, err := lookupDatabaseCorrelation(
			nodeContext.runtime,
			nodeContext.Infra.Database,
			nodeContext.Infra.Secrets,
			source.Configuration,
			mapping.Correlation.SourceField,
			primaryValue,
		)
		if err != nil {
			return nil, err
		}
		resolved[mapping.SourceNodeID] = value
	}
	return resolved, nil
}

func lookupDatabaseCorrelation(
	ctx context.Context,
	database DatabaseInfrastructure,
	secrets SecretDecryptor,
	configuration map[string]any,
	sourceField string,
	primaryValue any,
) (map[string]any, error) {
	primaryKey, argument, err := canonicalCorrelationValue(primaryValue, "")
	if err != nil {
		return nil, validationNodeError(
			"MERGE_CORRELATION_PRIMARY_FIELD_INVALID",
			"The configured Merge primary correlation field must contain a deterministic scalar value.",
		)
	}
	if err := validateDatabaseInput(configuration); err != nil {
		return nil, err
	}
	connection, err := openDatabaseConnection(
		ctx, database, secrets, configuration, "MERGE_CORRELATION",
	)
	if err != nil {
		return nil, mergeCorrelationDatabaseError(err)
	}
	defer connection.Close()
	query := fmt.Sprintf(
		`SELECT * FROM (%s) AS miletos_correlation WHERE %s = $1 LIMIT 2`,
		configString(configuration, "query"),
		quotePostgreSQLIdentifier(sourceField),
	)
	rows, err := connection.Query(ctx, query, argument)
	if err != nil {
		return nil, mergeCorrelationDatabaseError(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, mergeCorrelationDatabaseError(err)
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, mergeCorrelationDatabaseError(err)
	}
	sourceIndex := -1
	for index, column := range columns {
		if column == sourceField {
			sourceIndex = index
			break
		}
	}
	if sourceIndex < 0 {
		return nil, validationNodeError(
			"MERGE_CORRELATION_SOURCE_FIELD_MISSING",
			"The configured Merge source correlation field is missing from the Database Input result.",
		)
	}
	var matched map[string]any
	matchCount := 0
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, mergeCorrelationDatabaseError(err)
		}
		sourceKey, _, err := canonicalCorrelationValue(
			values[sourceIndex], columnTypes[sourceIndex].DatabaseTypeName(),
		)
		if err != nil {
			return nil, validationNodeError(
				"MERGE_CORRELATION_SOURCE_FIELD_INVALID",
				"The configured Merge source correlation field must contain deterministic scalar values.",
			)
		}
		if sourceKey != primaryKey {
			continue
		}
		matchCount++
		if matchCount > 1 {
			return nil, validationNodeError(
				"MERGE_CORRELATION_SOURCE_DUPLICATE",
				"The configured Merge source correlation field must contain unique values.",
			)
		}
		matched = make(map[string]any, len(columns))
		for index, column := range columns {
			matched[column] = normalizeSQLValue(values[index])
		}
	}
	if err := rows.Err(); err != nil {
		return nil, mergeCorrelationDatabaseError(err)
	}
	if matched == nil {
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     "MERGE_CORRELATION_NOT_FOUND",
			Message:  "No Database Input row matched the configured Merge correlation key.",
		}
	}
	return matched, nil
}

func canonicalCorrelationValue(value any, databaseType string) (correlationKey, any, error) {
	databaseType = strings.ToUpper(strings.TrimSpace(databaseType))
	if value == nil || databaseCorrelationTypeUnsupported(databaseType) {
		return correlationKey{}, nil, errors.New("correlation key is not scalar")
	}
	switch value := value.(type) {
	case bool:
		return correlationKey{Kind: "boolean", Value: strconv.FormatBool(value)}, value, nil
	case string:
		if isIntegerDatabaseType(databaseType) || isDecimalDatabaseType(databaseType) {
			return canonicalNumericCorrelation(value, value)
		}
		return correlationKey{Kind: "string", Value: value}, value, nil
	case []byte:
		text := string(value)
		if isIntegerDatabaseType(databaseType) || isDecimalDatabaseType(databaseType) {
			return canonicalNumericCorrelation(text, text)
		}
		return correlationKey{Kind: "string", Value: text}, text, nil
	case json.Number:
		return canonicalNumericCorrelation(value.String(), correlationQueryNumber(value))
	case int:
		return canonicalNumericCorrelation(strconv.Itoa(value), value)
	case int8:
		return canonicalNumericCorrelation(strconv.FormatInt(int64(value), 10), value)
	case int16:
		return canonicalNumericCorrelation(strconv.FormatInt(int64(value), 10), value)
	case int32:
		return canonicalNumericCorrelation(strconv.FormatInt(int64(value), 10), value)
	case int64:
		return canonicalNumericCorrelation(strconv.FormatInt(value, 10), value)
	case uint:
		return canonicalNumericCorrelation(strconv.FormatUint(uint64(value), 10), value)
	case uint8:
		return canonicalNumericCorrelation(strconv.FormatUint(uint64(value), 10), value)
	case uint16:
		return canonicalNumericCorrelation(strconv.FormatUint(uint64(value), 10), value)
	case uint32:
		return canonicalNumericCorrelation(strconv.FormatUint(uint64(value), 10), value)
	case uint64:
		argument := any(value)
		if value > math.MaxInt64 {
			argument = strconv.FormatUint(value, 10)
		}
		return canonicalNumericCorrelation(strconv.FormatUint(value, 10), argument)
	case float32:
		return canonicalFloatCorrelation(float64(value), 32)
	case float64:
		return canonicalFloatCorrelation(value, 64)
	case time.Time:
		normalized := value.UTC().Format(time.RFC3339Nano)
		return correlationKey{Kind: "time", Value: normalized}, value, nil
	default:
		return correlationKey{}, nil, errors.New("correlation key is not scalar")
	}
}

func canonicalFloatCorrelation(value float64, bitSize int) (correlationKey, any, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return correlationKey{}, nil, errors.New("correlation key is not finite")
	}
	return canonicalNumericCorrelation(strconv.FormatFloat(value, 'g', -1, bitSize), value)
}

func canonicalNumericCorrelation(value string, argument any) (correlationKey, any, error) {
	number, ok := new(big.Rat).SetString(value)
	if !ok {
		return correlationKey{}, nil, errors.New("correlation key is not numeric")
	}
	return correlationKey{Kind: "number", Value: number.RatString()}, argument, nil
}

func correlationQueryNumber(value json.Number) any {
	if integer, err := value.Int64(); err == nil {
		return integer
	}
	return value.String()
}

func databaseCorrelationTypeUnsupported(databaseType string) bool {
	return databaseType == "JSON" || databaseType == "JSONB" ||
		databaseType == "BYTEA" || strings.HasPrefix(databaseType, "_") ||
		strings.HasSuffix(databaseType, "[]")
}

func mergeCorrelationDatabaseError(err error) error {
	var nodeError *NodeError
	if errors.As(err, &nodeError) {
		return err
	}
	var postgresError interface{ SQLState() string }
	if errors.As(err, &postgresError) {
		switch postgresError.SQLState() {
		case "42703":
			return validationNodeError(
				"MERGE_CORRELATION_SOURCE_FIELD_MISSING",
				"The configured Merge source correlation field is missing from the Database Input result.",
			)
		case "22P02", "42804", "42883":
			return validationNodeError(
				"MERGE_CORRELATION_KEY_TYPE_INVALID",
				"The configured Merge primary and source correlation fields have incompatible scalar types.",
			)
		}
	}
	return &NodeError{
		Category: "EXECUTION",
		Code:     "MERGE_CORRELATION_QUERY_FAILED",
		Message:  "The Merge correlation query failed.",
		CanRetry: isRetryablePostgreSQLConnectionError(err),
		Cause:    err,
	}
}

func routeMergeOutput(nodeContext *Context, output map[string]any) error {
	for index := 0; index < nodeContext.Access.GetOutputEdgeCount(); index++ {
		if err := nodeContext.Access.PushEdge(index, output); err != nil {
			return &NodeError{
				Category: "EXECUTION",
				Code:     "MERGE_ROUTE_FAILED",
				Message:  "The merged output could not be emitted.",
				Cause:    err,
			}
		}
	}
	return nil
}

func mergeMappings(configuration map[string]any) ([]mergeMapping, error) {
	raw, exists := configuration["mappings"]
	if !exists || raw == nil {
		return nil, validationNodeError("MERGE_MAPPINGS_REQUIRED", "configuration.mappings is required")
	}
	items, err := configurationObjectList(raw, "MERGE_MAPPINGS_INVALID")
	if err != nil {
		return nil, err
	}
	mappings := make([]mergeMapping, 0, len(items))
	for _, item := range items {
		correlation, err := mergeMappingCorrelation(item)
		if err != nil {
			return nil, err
		}
		mappings = append(mappings, mergeMapping{
			Key:          strings.TrimSpace(configString(item, "key")),
			SourceNodeID: strings.TrimSpace(configString(item, "sourceNodeId")),
			Correlation:  correlation,
		})
	}
	return mappings, nil
}

func mergeMappingCorrelation(item map[string]any) (*mergeCorrelation, error) {
	raw, exists := item["correlation"]
	if !exists || raw == nil {
		return nil, nil
	}
	configuration, ok := raw.(map[string]any)
	if !ok {
		return nil, validationNodeError(
			"MERGE_CORRELATION_INVALID",
			"configuration.mappings.correlation must be an object",
		)
	}
	return &mergeCorrelation{
		PrimaryField: strings.TrimSpace(configString(configuration, "primaryField")),
		SourceField:  strings.TrimSpace(configString(configuration, "sourceField")),
	}, nil
}

func mergeInputsBySource(
	payload any,
	configuredSources map[string]struct{},
) (map[string]any, error) {
	entries, err := multiInputEntries(payload, "MERGE_INPUT_INVALID")
	if err != nil {
		return nil, err
	}
	inputs := make(map[string]any, len(entries))
	for _, entry := range entries {
		sourceNodeID, ok := entry["sourceNodeId"].(string)
		value, valueExists := entry["value"]
		sourceNodeID = strings.TrimSpace(sourceNodeID)
		if !ok || sourceNodeID == "" || !valueExists {
			return nil, validationNodeError(
				"MERGE_INPUT_INVALID", "Merge input metadata must identify a source node and value.",
			)
		}
		if _, configured := configuredSources[sourceNodeID]; !configured {
			continue
		}
		if _, duplicate := inputs[sourceNodeID]; duplicate {
			return nil, validationNodeError(
				"MERGE_INPUT_DUPLICATE", "Merge inputs must have unique source node identifiers.",
			)
		}
		inputs[sourceNodeID] = value
	}
	return inputs, nil
}
