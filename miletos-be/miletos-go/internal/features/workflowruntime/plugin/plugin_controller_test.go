package plugin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	plugin "miletos-go/internal/features/workflowruntime/plugin"
)

func TestPluginControllerReturnsDeterministicRegisteredNodes(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	handler := func(context.Context, map[string]any, any) (any, error) { return nil, nil }
	if err := registry.DefineNode("z.node", handler); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	if err := registry.DefineNode("a.node", handler); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	response := httptest.NewRecorder()

	plugin.NewController(registry).List(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil),
	)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	var document struct {
		Items []struct {
			Type        string `json:"type"`
			Version     string `json:"version"`
			DisplayName string `json:"displayName"`
		} `json:"items"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if document.Count != 2 || len(document.Items) != 2 {
		t.Fatalf("plugin response = %#v", document)
	}
	if document.Items[0].Type != "a.node" || document.Items[1].Type != "z.node" {
		t.Fatalf("plugin order = %#v", document.Items)
	}
	if document.Items[0].Version != "v1" || document.Items[0].DisplayName != "a.node" {
		t.Fatalf("plugin shape = %#v", document.Items[0])
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "handler") {
		t.Fatal("callback implementation was serialized")
	}
}
