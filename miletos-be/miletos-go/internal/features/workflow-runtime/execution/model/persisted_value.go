package model

const PersistedValueFormat = "miletos.workflow.persisted-value.v1"

func EncodePersistedValue(value any) map[string]any {
	return map[string]any{
		"format": PersistedValueFormat,
		"value":  value,
	}
}

func IsPersistedValue(stored map[string]any) bool {
	if stored == nil {
		return false
	}
	format, _ := stored["format"].(string)
	if format != PersistedValueFormat {
		return false
	}
	_, hasValue := stored["value"]
	return hasValue && len(stored) == 2
}

func DecodePersistedValue(stored map[string]any) any {
	if stored == nil {
		return nil
	}
	if IsPersistedValue(stored) {
		return stored["value"]
	}
	return decodeLegacySummary(stored)
}

func decodeLegacySummary(stored map[string]any) any {

	if value, exists := stored["value"]; exists && len(stored) == 1 {
		return value
	}
	return stored
}

func PublicSummary(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if object, ok := value.(map[string]any); ok {
		if object == nil {
			return map[string]any{}
		}
		return object
	}
	return map[string]any{"value": value}
}

func EncodePersistedEdgePayloads(payloads map[string]any) map[string]any {
	if payloads == nil {
		return nil
	}
	encoded := make(map[string]any, len(payloads))
	for edgeID, payload := range payloads {
		encoded[edgeID] = EncodePersistedValue(payload)
	}
	return encoded
}

func DecodePersistedEdgePayload(value any) any {
	envelope, ok := value.(map[string]any)
	if !ok || !IsPersistedValue(envelope) {
		return value
	}
	return envelope["value"]
}

func DecodePersistedEdgePayloads(payloads map[string]any) map[string]any {
	if payloads == nil {
		return nil
	}
	decoded := make(map[string]any, len(payloads))
	for edgeID, payload := range payloads {
		decoded[edgeID] = DecodePersistedEdgePayload(payload)
	}
	return decoded
}
