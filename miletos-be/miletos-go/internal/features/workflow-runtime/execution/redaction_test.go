package execution

import "testing"

func TestRedactNodeDataRecursivelyPreservesShape(t *testing.T) {
	input := map[string]any{
		"Authorization": "Bearer private",
		"nested": map[string]any{
			"accessToken": "private",
			"items":       []any{map[string]any{"PASSWORD": "private", "visible": "ok"}},
		},
	}
	redacted := RedactNodeData(input).(map[string]any)
	if redacted["Authorization"] != redactedValue {
		t.Fatalf("authorization was not redacted: %#v", redacted)
	}
	nested := redacted["nested"].(map[string]any)
	if nested["accessToken"] != redactedValue {
		t.Fatalf("nested token was not redacted: %#v", redacted)
	}
	item := nested["items"].([]any)[0].(map[string]any)
	if item["PASSWORD"] != redactedValue || item["visible"] != "ok" {
		t.Fatalf("array shape or values changed: %#v", redacted)
	}
}
