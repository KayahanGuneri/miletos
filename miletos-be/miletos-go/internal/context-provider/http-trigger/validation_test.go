package httptrigger

import (
	"context"
	"errors"
	"testing"

	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

func triggerRegistry(t *testing.T, allowedSources []string) *plugin.NodeRegistry {
	t.Helper()
	registry := plugin.NewNodeRegistry()
	err := registry.RegisterNode(plugin.NodeRegistration{
		Definition: plugin.NodeDefinition{Type: "test.trigger", Version: "v1"},
		Handler: func(context.Context, plugin.NodeExecutionContext, map[string]any, any) (any, error) {
			return nil, nil
		},
		AllowedExecutionSources: allowedSources,
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func triggerWorkflow(configuration map[string]any) workflow.Workflow {
	return workflow.Workflow{Nodes: []workflow.WorkflowNode{{
		ID: "root", Type: "test.trigger", Version: "v1", Configuration: configuration,
	}}}
}

func TestValidateTriggerRootRequiresExplicitHTTPWebhookSource(t *testing.T) {
	err := validateTriggerRoot(
		triggerWorkflow(map[string]any{"method": "post"}),
		"root", "POST", triggerRegistry(t, []string{"HTTP_WEBHOOK"}),
	)
	if err != nil {
		t.Fatalf("valid provider contract rejected: %v", err)
	}
}

func TestValidateTriggerRootRejectsInvalidOrIncompatibleBindings(t *testing.T) {
	tests := []struct {
		name           string
		definition     workflow.Workflow
		nodeID         string
		method         string
		allowedSources []string
	}{
		{"wrong node", triggerWorkflow(map[string]any{"method": "POST"}), "other", "POST", []string{"HTTP_WEBHOOK"}},
		{"method mismatch", triggerWorkflow(map[string]any{"method": "GET"}), "root", "POST", []string{"HTTP_WEBHOOK"}},
		{"missing method", triggerWorkflow(nil), "root", "POST", []string{"HTTP_WEBHOOK"}},
		{"wrong source", triggerWorkflow(map[string]any{"method": "POST"}), "root", "POST", []string{"MANUAL_DIRECT"}},
		{"unrestricted is not explicit", triggerWorkflow(map[string]any{"method": "POST"}), "root", "POST", nil},
		{"multiple roots", workflow.Workflow{Nodes: []workflow.WorkflowNode{
			{ID: "root", Type: "test.trigger", Version: "v1", Configuration: map[string]any{"method": "POST"}},
			{ID: "second", Type: "test.trigger", Version: "v1", Configuration: map[string]any{"method": "POST"}},
		}}, "root", "POST", []string{"HTTP_WEBHOOK"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTriggerRoot(test.definition, test.nodeID, test.method, triggerRegistry(t, test.allowedSources))
			if !errors.Is(err, ErrInvalidTrigger) {
				t.Fatalf("error = %v, want ErrInvalidTrigger", err)
			}
		})
	}
}
