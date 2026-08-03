package plugin_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	plugin "miletos-go/internal/features/workflow-runtime/plugin"
)

func TestNodeRegistryRegistersAndExecutesHandler(t *testing.T) {
	type contextKey string
	const key contextKey = "request"
	registry := plugin.NewNodeRegistry()
	wantError := errors.New("handler failed")
	wantNodeContext := plugin.NodeExecutionContext{
		CompanyID: "company-1", WorkflowID: "workflow-1", ExecutionID: "execution-1",
		NodeID: "node-1", CorrelationID: "correlation-1", Source: "MANUAL_DIRECT",
	}
	handler := func(ctx context.Context, nodeContext plugin.NodeExecutionContext, configuration map[string]any, input any) (any, error) {
		if ctx.Value(key) != "context-value" {
			t.Errorf("context value = %v", ctx.Value(key))
		}
		if configuration["configured"] != true {
			t.Errorf("configuration = %#v", configuration)
		}
		if input != "input-value" {
			t.Errorf("input = %#v", input)
		}
		if nodeContext != wantNodeContext {
			t.Errorf("node execution context = %#v", nodeContext)
		}
		return "output-value", wantError
	}

	if err := registry.DefineNode(" custom.node ", handler); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	registered, exists := registry.Get("custom.node")
	if !exists {
		t.Fatal("Get() did not find registered node")
	}
	output, err := registered(
		context.WithValue(context.Background(), key, "context-value"),
		wantNodeContext,
		map[string]any{"configured": true},
		"input-value",
	)
	if output != "output-value" || !errors.Is(err, wantError) {
		t.Fatalf("handler returned (%#v, %v)", output, err)
	}
}

func TestNodeRegistryRejectsInvalidRegistration(t *testing.T) {
	handler := func(context.Context, plugin.NodeExecutionContext, map[string]any, any) (any, error) { return nil, nil }
	tests := []struct {
		name    string
		node    string
		handler plugin.NodeHandler
	}{
		{name: "empty name", node: "", handler: handler},
		{name: "whitespace name", node: " \t ", handler: handler},
		{name: "nil handler", node: "custom.node", handler: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := plugin.NewNodeRegistry()
			if err := registry.DefineNode(test.node, test.handler); err == nil {
				t.Fatal("DefineNode() error = nil")
			}
		})
	}
}

func TestNodeRegistryRejectsDuplicateAndUnknownLookup(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	handler := func(context.Context, plugin.NodeExecutionContext, map[string]any, any) (any, error) { return nil, nil }
	if err := registry.DefineNode("custom.node", handler); err != nil {
		t.Fatalf("first DefineNode() error = %v", err)
	}
	if err := registry.DefineNode("custom.node", handler); err == nil {
		t.Fatal("duplicate DefineNode() error = nil")
	}
	if _, exists := registry.Get("unknown.node"); exists {
		t.Fatal("Get() found unknown node")
	}
}

func TestNodeRegistriesAreIndependent(t *testing.T) {
	first := plugin.NewNodeRegistry()
	second := plugin.NewNodeRegistry()
	handler := func(context.Context, plugin.NodeExecutionContext, map[string]any, any) (any, error) { return nil, nil }
	if err := first.DefineNode("custom.node", handler); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	if _, exists := second.Get("custom.node"); exists {
		t.Fatal("second registry contains first registry handler")
	}
}

func TestNodeRegistrySupportsConcurrentReads(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	handler := func(context.Context, plugin.NodeExecutionContext, map[string]any, any) (any, error) { return nil, nil }
	if err := registry.DefineNode("custom.node", handler); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}

	var missing atomic.Int32
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 100 {
				if _, exists := registry.Get("custom.node"); !exists {
					missing.Add(1)
				}
			}
		}()
	}
	wait.Wait()

	if missing.Load() != 0 {
		t.Fatalf("missing reads = %d", missing.Load())
	}
}

func TestCanReceiveEntryInputIsDerivedFromPortsAndEdgeConstraints(t *testing.T) {
	zero := uint(0)
	one := uint(1)
	tests := []struct {
		name       string
		definition plugin.NodeDefinition
		want       bool
	}{
		{name: "no input port", definition: plugin.NodeDefinition{}},
		{
			name: "incoming edge required",
			definition: plugin.NodeDefinition{
				InputPorts:          []plugin.Port{{Name: "input"}},
				InputEdgeConstraint: plugin.EdgeConstraint{Minimum: 1},
			},
		},
		{
			name: "nil maximum is unbounded",
			definition: plugin.NodeDefinition{
				InputPorts: []plugin.Port{{Name: "input"}},
			},
			want: true,
		},
		{
			name: "zero maximum rejects input",
			definition: plugin.NodeDefinition{
				InputPorts:          []plugin.Port{{Name: "input"}},
				InputEdgeConstraint: plugin.EdgeConstraint{Maximum: &zero},
			},
		},
		{
			name: "positive maximum accepts input",
			definition: plugin.NodeDefinition{
				InputPorts:          []plugin.Port{{Name: "input"}},
				InputEdgeConstraint: plugin.EdgeConstraint{Maximum: &one},
			},
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := plugin.CanReceiveEntryInput(test.definition); got != test.want {
				t.Fatalf("CanReceiveEntryInput() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestNodeRegistryDistinguishesGenericAndDeclaredExecutionSources(t *testing.T) {
	handler := func(context.Context, plugin.NodeExecutionContext, map[string]any, any) (any, error) {
		return nil, nil
	}
	registry := plugin.NewNodeRegistry()
	for _, registration := range []plugin.NodeRegistration{
		{
			Definition: plugin.NodeDefinition{Type: "unrestricted", Version: "v1"},
			Handler:    handler,
		},
		{
			Definition:              plugin.NodeDefinition{Type: "webhook", Version: "v1"},
			Handler:                 handler,
			AllowedExecutionSources: []string{"HTTP_WEBHOOK"},
		},
		{
			Definition:              plugin.NodeDefinition{Type: "manual", Version: "v1"},
			Handler:                 handler,
			AllowedExecutionSources: []string{"MANUAL_DIRECT"},
		},
	} {
		if err := registry.RegisterNode(registration); err != nil {
			t.Fatalf("RegisterNode(%s) error = %v", registration.Definition.Type, err)
		}
	}

	if !registry.CanStartFrom("unrestricted", "v1", "HTTP_WEBHOOK") {
		t.Fatal("unrestricted registration cannot start from HTTP_WEBHOOK")
	}
	if registry.DeclaresExecutionSource("unrestricted", "v1", "HTTP_WEBHOOK") {
		t.Fatal("empty source list was treated as an explicit HTTP_WEBHOOK declaration")
	}
	if !registry.CanStartFrom("webhook", "v1", "HTTP_WEBHOOK") ||
		!registry.DeclaresExecutionSource("webhook", "v1", "HTTP_WEBHOOK") {
		t.Fatal("explicit HTTP_WEBHOOK registration was not recognized")
	}
	if registry.CanStartFrom("manual", "v1", "HTTP_WEBHOOK") ||
		registry.DeclaresExecutionSource("manual", "v1", "HTTP_WEBHOOK") {
		t.Fatal("MANUAL_DIRECT registration was treated as HTTP_WEBHOOK eligible")
	}
	if registry.CanStartFrom("missing", "v1", "HTTP_WEBHOOK") ||
		registry.DeclaresExecutionSource("missing", "v1", "HTTP_WEBHOOK") {
		t.Fatal("missing registration was treated as eligible")
	}
}
