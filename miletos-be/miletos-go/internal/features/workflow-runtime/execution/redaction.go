package execution

import "strings"

const redactedValue = "[REDACTED]"

var sensitiveKeys = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"password":            {},
	"secret":              {},
	"token":               {},
	"accesstoken":         {},
	"refreshtoken":        {},
	"apikey":              {},
	"clientsecret":        {},
}

func RedactNodeData(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, child := range typed {
			if _, sensitive := sensitiveKeys[strings.ToLower(key)]; sensitive {
				redacted[key] = redactedValue
				continue
			}
			redacted[key] = RedactNodeData(child)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for index, child := range typed {
			redacted[index] = RedactNodeData(child)
		}
		return redacted
	default:
		return typed
	}
}

func redactObject(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return RedactNodeData(value).(map[string]any)
}
