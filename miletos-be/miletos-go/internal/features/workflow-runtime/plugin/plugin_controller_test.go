package plugin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/emptypb"

	plugin "miletos-go/internal/features/workflow-runtime/plugin"
)

func TestPluginControllerReturnsDeterministicRegisteredNodes(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	handler := func(context.Context, plugin.NodeExecutionContext, map[string]any, any) (any, error) { return nil, nil }
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

func TestPluginTransportsDeriveBuiltinCompatibilityFields(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}

	response := httptest.NewRecorder()
	plugin.NewController(registry).List(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d; body=%s", response.Code, response.Body.String())
	}
	var document struct {
		Items []struct {
			Type                    string   `json:"type"`
			AcceptsInitialVariables bool     `json:"acceptsInitialVariables"`
			AllowedRootOrigins      []string `json:"allowedRootOrigins"`
			ContextProvider         string   `json:"contextProvider"`
			InputEdgeConstraint     struct {
				Maximum   *uint `json:"maximum"`
				Unlimited bool  `json:"unlimited"`
			} `json:"inputEdgeConstraint"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	httpItems := make(map[string]struct {
		accepts   bool
		provider  string
		origins   []string
		maximum   *uint
		unlimited bool
	}, len(document.Items))
	for _, item := range document.Items {
		httpItems[item.Type] = struct {
			accepts   bool
			provider  string
			origins   []string
			maximum   *uint
			unlimited bool
		}{
			accepts:   item.AcceptsInitialVariables,
			provider:  item.ContextProvider,
			origins:   item.AllowedRootOrigins,
			maximum:   item.InputEdgeConstraint.Maximum,
			unlimited: item.InputEdgeConstraint.Unlimited,
		}
	}
	for _, nodeType := range []string{
		"core.static-input", "core.pass-through", "core.delay", "core.terminal", "core.join",
	} {
		item := httpItems[nodeType]
		if item.provider != "" {
			t.Errorf("HTTP %s contextProvider = %q", nodeType, item.provider)
		}
	}
	if httpItems["core.static-input"].accepts || httpItems["core.pass-through"].accepts ||
		httpItems["core.join"].accepts {
		t.Fatalf("HTTP entry-input compatibility = %#v", httpItems)
	}
	httpTrigger := httpItems["core.http-trigger"]
	if !httpTrigger.accepts || httpTrigger.provider != "http-trigger" ||
		len(httpTrigger.origins) != 1 || httpTrigger.origins[0] != "HTTP_WEBHOOK" {
		t.Fatalf("HTTP trigger compatibility = %#v", httpTrigger)
	}
	join := httpItems["core.join"]
	if join.maximum != nil || !join.unlimited {
		t.Fatalf("HTTP join input constraint = %#v", join)
	}

	grpcResponse, err := plugin.NewGRPCService(registry).ListPlugins(
		context.Background(), &emptypb.Empty{},
	)
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	for _, item := range grpcResponse.GetItems() {
		switch item.GetType() {
		case "core.join":
			if item.GetContextProvider() != "" || item.GetAcceptsInitialVariables() ||
				!item.GetInputEdgeConstraint().GetUnlimited() {
				t.Errorf("gRPC join compatibility = %#v", item)
			}
		case "core.http-trigger":
			if item.GetContextProvider() != "http-trigger" ||
				!item.GetAcceptsInitialVariables() ||
				len(item.GetAllowedRootOrigins()) != 1 ||
				item.GetAllowedRootOrigins()[0] != "HTTP_WEBHOOK" {
				t.Errorf("gRPC HTTP trigger compatibility = %#v", item)
			}
		}
	}
}
