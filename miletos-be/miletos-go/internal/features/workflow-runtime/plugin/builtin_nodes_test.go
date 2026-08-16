package plugin_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	plugin "miletos-go/internal/features/workflow-runtime/plugin"
)

func builtinRegistration(t *testing.T, name string) plugin.NodeRegistration {
	t.Helper()
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	registration, exists := registry.Get(name)
	if !exists {
		t.Fatalf("built-in %q is not registered", name)
	}
	return registration
}

func runBuiltin(
	t *testing.T,
	registration plugin.NodeRegistration,
	runtime context.Context,
	configuration map[string]any,
	payload any,
) (any, error) {
	t.Helper()
	lifecycles := plugin.NewLifecycles()
	nodeContext := plugin.NewContext(plugin.ContextOptions{
		Runtime: runtime, Configuration: configuration, Payload: payload, Lifecycles: lifecycles,
	})
	if err := registration.Handler(nodeContext); err != nil {
		return nil, err
	}
	return lifecycles.InvokeRun()
}

func TestStaticInputNode(t *testing.T) {
	registration := builtinRegistration(t, "core.static-input")

	output, err := runBuiltin(
		t, registration, context.Background(), map[string]any{"value": "configured"}, nil,
	)
	if err != nil || output != "configured" {
		t.Fatalf("handler returned (%#v, %v)", output, err)
	}
	output, err = runBuiltin(
		t, registration, context.Background(),
		map[string]any{"valueType": "JSON", "value": `[{"id":1}]`}, nil,
	)
	if err != nil || !reflect.DeepEqual(output, []any{map[string]any{"id": float64(1)}}) {
		t.Fatalf("JSON array handler returned (%#v, %v)", output, err)
	}
	if _, err := runBuiltin(t, registration, context.Background(), map[string]any{}, nil); err == nil {
		t.Fatal("missing value error = nil")
	}
	if _, err := runBuiltin(
		t, registration, context.Background(), map[string]any{"value": "x"}, "input",
	); err == nil {
		t.Fatal("unexpected input error = nil")
	}
}

func TestPassThroughAndTerminalNodes(t *testing.T) {
	for _, name := range []string{"core.pass-through", "core.terminal"} {
		t.Run(name, func(t *testing.T) {
			registration := builtinRegistration(t, name)
			output, err := runBuiltin(t, registration, context.Background(), nil, "payload")
			if err != nil || output != "payload" {
				t.Fatalf("handler returned (%#v, %v)", output, err)
			}
			if _, err := runBuiltin(t, registration, context.Background(), nil, nil); err == nil {
				t.Fatal("nil input error = nil")
			}
		})
	}
}

func TestDelayNodeHonorsCancellationAndValidatesConfiguration(t *testing.T) {
	registration := builtinRegistration(t, "core.delay")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runBuiltin(t, registration, ctx, map[string]any{"delay": "1h"}, "payload")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("handler error = %v, want context.Canceled", err)
	}
	for _, configuration := range []map[string]any{
		nil,
		{"delay": "invalid"},
		{"delay": "0s"},
		{"delay": "25h"},
	} {
		if _, err := runBuiltin(t, registration, context.Background(), configuration, "payload"); err == nil {
			t.Fatalf("configuration %#v error = nil", configuration)
		}
	}
}
