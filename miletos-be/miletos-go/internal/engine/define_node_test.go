package engine_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"miletos-go/internal/engine"
)

func TestNodeRegistryRegistersAndExecutesHandler(t *testing.T) {
	type contextKey string
	const key contextKey = "request"
	registry := engine.NewNodeRegistry()
	wantError := errors.New("handler failed")
	handler := func(ctx context.Context, configuration map[string]any, input any) (any, error) {
		if ctx.Value(key) != "context-value" {
			t.Errorf("context value = %v", ctx.Value(key))
		}
		if configuration["configured"] != true {
			t.Errorf("configuration = %#v", configuration)
		}
		if input != "input-value" {
			t.Errorf("input = %#v", input)
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
		map[string]any{"configured": true},
		"input-value",
	)
	if output != "output-value" || !errors.Is(err, wantError) {
		t.Fatalf("handler returned (%#v, %v)", output, err)
	}
}

func TestNodeRegistryRejectsInvalidRegistration(t *testing.T) {
	handler := func(context.Context, map[string]any, any) (any, error) { return nil, nil }
	tests := []struct {
		name    string
		node    string
		handler engine.NodeHandler
	}{
		{name: "empty name", node: "", handler: handler},
		{name: "whitespace name", node: " \t ", handler: handler},
		{name: "nil handler", node: "custom.node", handler: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := engine.NewNodeRegistry()
			if err := registry.DefineNode(test.node, test.handler); err == nil {
				t.Fatal("DefineNode() error = nil")
			}
		})
	}
}

func TestNodeRegistryRejectsDuplicateAndUnknownLookup(t *testing.T) {
	registry := engine.NewNodeRegistry()
	handler := func(context.Context, map[string]any, any) (any, error) { return nil, nil }
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
	first := engine.NewNodeRegistry()
	second := engine.NewNodeRegistry()
	handler := func(context.Context, map[string]any, any) (any, error) { return nil, nil }
	if err := first.DefineNode("custom.node", handler); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	if _, exists := second.Get("custom.node"); exists {
		t.Fatal("second registry contains first registry handler")
	}
}

func TestNodeRegistrySupportsConcurrentReads(t *testing.T) {
	registry := engine.NewNodeRegistry()
	handler := func(context.Context, map[string]any, any) (any, error) { return nil, nil }
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
