package httptrigger

import (
	"errors"
	"testing"

	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

func triggerRegistry(t *testing.T, allowedSources []string) *plugin.NodeRegistry {
	t.Helper()
	registry := plugin.NewNodeRegistry()
	err := registry.RegisterNode(plugin.NodeRegistration{
		Key:                     "test.trigger",
		Handler:                 func(*plugin.Context) error { return nil },
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
		"root", triggerRegistry(t, []string{"HTTP_WEBHOOK"}),
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
		allowedSources []string
	}{
		{"wrong node", triggerWorkflow(map[string]any{"method": "POST"}), "other", []string{"HTTP_WEBHOOK"}},
		{"wrong source", triggerWorkflow(map[string]any{"method": "POST"}), "root", []string{"MANUAL_DIRECT"}},
		{"unrestricted is not explicit", triggerWorkflow(map[string]any{"method": "POST"}), "root", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTriggerRoot(test.definition, test.nodeID, triggerRegistry(t, test.allowedSources))
			if !errors.Is(err, ErrInvalidTrigger) {
				t.Fatalf("error = %v, want ErrInvalidTrigger", err)
			}
		})
	}
}
