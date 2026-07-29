package engine_test

import (
	"context"
	"errors"
	"testing"

	"miletos-go/internal/engine"
)

func builtinHandler(t *testing.T, name string) engine.NodeHandler {
	t.Helper()
	registry := engine.NewNodeRegistry()
	if err := engine.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	handler, exists := registry.Get(name)
	if !exists {
		t.Fatalf("built-in %q is not registered", name)
	}
	return handler
}

func TestStaticInputNode(t *testing.T) {
	handler := builtinHandler(t, "core.static-input")

	output, err := handler(context.Background(), map[string]any{"value": "configured"}, nil)
	if err != nil || output != "configured" {
		t.Fatalf("handler returned (%#v, %v)", output, err)
	}
	if _, err := handler(context.Background(), map[string]any{}, nil); err == nil {
		t.Fatal("missing value error = nil")
	}
	if _, err := handler(context.Background(), map[string]any{"value": "x"}, "input"); err == nil {
		t.Fatal("unexpected input error = nil")
	}
}

func TestPassThroughAndTerminalNodes(t *testing.T) {
	for _, name := range []string{"core.pass-through", "core.terminal"} {
		t.Run(name, func(t *testing.T) {
			handler := builtinHandler(t, name)
			output, err := handler(context.Background(), nil, "payload")
			if err != nil || output != "payload" {
				t.Fatalf("handler returned (%#v, %v)", output, err)
			}
			if _, err := handler(context.Background(), nil, nil); err == nil {
				t.Fatal("nil input error = nil")
			}
		})
	}
}

func TestDelayNodeHonorsCancellationAndValidatesConfiguration(t *testing.T) {
	handler := builtinHandler(t, "core.delay")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := handler(ctx, map[string]any{"delay": "1h"}, "payload")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("handler error = %v, want context.Canceled", err)
	}
	for _, configuration := range []map[string]any{
		nil,
		{"delay": "invalid"},
		{"delay": "0s"},
		{"delay": "25h"},
	} {
		if _, err := handler(context.Background(), configuration, "payload"); err == nil {
			t.Fatalf("configuration %#v error = nil", configuration)
		}
	}
}
