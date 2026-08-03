package httptrigger

import (
	"net/http"
	"reflect"
	"testing"
)

func TestStableFingerprintPayloadExcludesServerOwnedIDs(t *testing.T) {
	first := map[string]any{
		"requestId":     "request-one",
		"correlationId": "correlation-one",
		"method":        "POST",
		"body":          map[string]any{"value": "stable"},
	}
	second := map[string]any{
		"requestId":     "request-two",
		"correlationId": "correlation-two",
		"method":        "POST",
		"body":          map[string]any{"value": "stable"},
	}

	if !reflect.DeepEqual(stableFingerprintPayload(first), stableFingerprintPayload(second)) {
		t.Fatal("volatile server-owned IDs changed the stable fingerprint payload")
	}
	if _, present := stableFingerprintPayload(first)["requestId"]; present {
		t.Fatal("requestId was retained in fingerprint payload")
	}
	if _, present := stableFingerprintPayload(first)["correlationId"]; present {
		t.Fatal("correlationId was retained in fingerprint payload")
	}
}

func TestSafeHeadersFiltersCredentialsAndControlHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization":        {"redacted"},
		"Cookie":               {"redacted"},
		"X-Api-Key":            {"redacted"},
		"X-Miletos-Company-Id": {"untrusted"},
		"X-Miletos-Custom":     {"control"},
		"X-Correlation-Id":     {"caller-owned"},
		"X-Request-Id":         {"caller-owned"},
		"X-Safe-Custom-Header": {"visible"},
		"Content-Type":         {"application/json"},
		"User-Agent":           {"runtime-test"},
	}

	safe := safeHeaders(headers)
	for _, name := range []string{
		"Authorization", "Cookie", "X-Api-Key", "X-Miletos-Company-Id",
		"X-Miletos-Custom", "X-Correlation-Id", "X-Request-Id",
	} {
		if _, present := safe[name]; present {
			t.Fatalf("sensitive header %q was retained", name)
		}
	}
	if got := safe["X-Safe-Custom-Header"]; !reflect.DeepEqual(got, []string{"visible"}) {
		t.Fatalf("safe custom header = %v", got)
	}
	if got := safe["Content-Type"]; !reflect.DeepEqual(got, []string{"application/json"}) {
		t.Fatalf("content type = %v", got)
	}
}
