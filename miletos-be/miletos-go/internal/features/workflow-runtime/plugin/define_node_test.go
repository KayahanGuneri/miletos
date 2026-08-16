package plugin_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	plugin "miletos-go/internal/features/workflow-runtime/plugin"
)

func TestNodeRegistryRegistersAndExecutesLifecycleHandler(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	wantError := errors.New("handler failed")
	handler := func(ctx *plugin.Context) error {
		ctx.Lifecycles.OnRun(func() (any, error) {
			return ctx.Payload, wantError
		})
		return nil
	}
	if err := registry.RegisterNode(plugin.NodeRegistration{
		Key: " custom.node ", Handler: handler,
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	registered, exists := registry.Get("custom.node")
	if !exists {
		t.Fatal("Get() did not find registered node")
	}
	lifecycles := plugin.NewLifecycles()
	nodeContext := plugin.NewContext(plugin.ContextOptions{
		Lifecycles: lifecycles, Payload: "input-value",
	})
	if err := registered.Handler(nodeContext); err != nil {
		t.Fatalf("handler registration error = %v", err)
	}
	output, err := lifecycles.InvokeRun()
	if output != "input-value" || !errors.Is(err, wantError) {
		t.Fatalf("handler returned (%#v, %v)", output, err)
	}
}

func TestNodeRegistryRejectsInvalidRegistration(t *testing.T) {
	handler := func(*plugin.Context) error { return nil }
	tests := []plugin.NodeRegistration{
		{Key: "", Handler: handler},
		{Key: " \t ", Handler: handler},
		{Key: "custom.node"},
	}
	for _, registration := range tests {
		registry := plugin.NewNodeRegistry()
		if err := registry.RegisterNode(registration); err == nil {
			t.Fatalf("RegisterNode(%q) error = nil", registration.Key)
		}
	}
}

func TestNodeRegistryRejectsDuplicateAndUnknownLookup(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	registration := plugin.NodeRegistration{
		Key: "custom.node", Handler: func(*plugin.Context) error { return nil },
	}
	if err := registry.RegisterNode(registration); err != nil {
		t.Fatalf("first RegisterNode() error = %v", err)
	}
	if err := registry.RegisterNode(registration); err == nil {
		t.Fatal("duplicate RegisterNode() error = nil")
	}
	if _, exists := registry.Get("unknown.node"); exists {
		t.Fatal("Get() found unknown node")
	}
}

func TestNodeRegistriesAreIndependent(t *testing.T) {
	first := plugin.NewNodeRegistry()
	second := plugin.NewNodeRegistry()
	if err := first.RegisterNode(plugin.NodeRegistration{
		Key: "custom.node", Handler: func(*plugin.Context) error { return nil },
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	if _, exists := second.Get("custom.node"); exists {
		t.Fatal("second registry contains first registry registration")
	}
}

func TestNodeRegistrySupportsConcurrentReads(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := registry.RegisterNode(plugin.NodeRegistration{
		Key: "custom.node", Handler: func(*plugin.Context) error { return nil },
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
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
		name         string
		registration plugin.NodeRegistration
		want         bool
	}{
		{name: "no input port"},
		{
			name: "incoming edge required",
			registration: plugin.NodeRegistration{
				InputPorts:          []plugin.Port{{Name: "input"}},
				InputEdgeConstraint: plugin.EdgeConstraint{Minimum: 1},
			},
		},
		{
			name:         "nil maximum is unbounded",
			registration: plugin.NodeRegistration{InputPorts: []plugin.Port{{Name: "input"}}},
			want:         true,
		},
		{
			name: "zero maximum rejects input",
			registration: plugin.NodeRegistration{
				InputPorts:          []plugin.Port{{Name: "input"}},
				InputEdgeConstraint: plugin.EdgeConstraint{Maximum: &zero},
			},
		},
		{
			name: "positive maximum accepts input",
			registration: plugin.NodeRegistration{
				InputPorts:          []plugin.Port{{Name: "input"}},
				InputEdgeConstraint: plugin.EdgeConstraint{Maximum: &one},
			},
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := plugin.CanReceiveEntryInput(test.registration); got != test.want {
				t.Fatalf("CanReceiveEntryInput() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestNodeRegistryDistinguishesGenericAndDeclaredExecutionSources(t *testing.T) {
	handler := func(*plugin.Context) error { return nil }
	registry := plugin.NewNodeRegistry()
	for _, registration := range []plugin.NodeRegistration{
		{Key: "unrestricted", Handler: handler},
		{Key: "webhook", Handler: handler, AllowedExecutionSources: []string{"HTTP_WEBHOOK"}},
		{Key: "manual", Handler: handler, AllowedExecutionSources: []string{"MANUAL_DIRECT"}},
	} {
		if err := registry.RegisterNode(registration); err != nil {
			t.Fatalf("RegisterNode(%s) error = %v", registration.Key, err)
		}
	}
	if !registry.CanStartFrom("unrestricted", "HTTP_WEBHOOK") {
		t.Fatal("unrestricted registration cannot start from HTTP_WEBHOOK")
	}
	if registry.DeclaresExecutionSource("unrestricted", "HTTP_WEBHOOK") {
		t.Fatal("empty source list was treated as an explicit HTTP_WEBHOOK declaration")
	}
	if !registry.CanStartFrom("webhook", "HTTP_WEBHOOK") ||
		!registry.DeclaresExecutionSource("webhook", "HTTP_WEBHOOK") {
		t.Fatal("explicit HTTP_WEBHOOK registration was not recognized")
	}
	if registry.CanStartFrom("manual", "HTTP_WEBHOOK") ||
		registry.DeclaresExecutionSource("manual", "HTTP_WEBHOOK") {
		t.Fatal("MANUAL_DIRECT registration was treated as HTTP_WEBHOOK eligible")
	}
	if registry.CanStartFrom("missing", "HTTP_WEBHOOK") ||
		registry.DeclaresExecutionSource("missing", "HTTP_WEBHOOK") {
		t.Fatal("missing registration was treated as eligible")
	}
}
