package plugin

func objectPayload(input any, code string, message string) (map[string]any, error) {
	payload, ok := input.(map[string]any)
	if !ok || payload == nil {
		return nil, validationNodeError(code, message)
	}
	return payload, nil
}

func multiInputEntries(input any, code string) ([]map[string]any, error) {
	payload, ok := input.(map[string]any)
	if !ok {
		return nil, validationNodeError(code, "Node requires a deterministic multi-input payload.")
	}
	rawInputs, exists := payload["inputs"]
	if !exists {
		return nil, validationNodeError(code, "Multi-input payload is missing inputs.")
	}
	switch typed := rawInputs.(type) {
	case []map[string]any:
		return typed, nil
	case []any:
		entries := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, validationNodeError(code, "Multi-input entries must be JSON objects.")
			}
			entries = append(entries, entry)
		}
		return entries, nil
	default:
		return nil, validationNodeError(code, "Multi-input payload inputs must be an array.")
	}
}

func cloneObject(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func configurationObjectList(raw any, code string) ([]map[string]any, error) {
	switch typed := raw.(type) {
	case []map[string]any:
		return typed, nil
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, validationNodeError(code, "configuration value must be a list of objects")
			}
			items = append(items, object)
		}
		return items, nil
	default:
		return nil, validationNodeError(code, "configuration value must be a list")
	}
}

func validationNodeError(code string, message string) *NodeError {
	return &NodeError{Category: "VALIDATION", Code: code, Message: message}
}

func executionNodeError(code string, message string, retryable bool) *NodeError {
	return &NodeError{
		Category: "EXECUTION", Code: code, Message: message, CanRetry: retryable,
	}
}
