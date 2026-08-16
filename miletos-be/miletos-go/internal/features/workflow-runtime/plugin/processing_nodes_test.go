package plugin

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordedPush struct {
	index   int
	payload any
}

type recordingAccess struct {
	edges  []EdgeData
	pushes []recordedPush
}

func (access *recordingAccess) GetOutputEdgeCount() int {
	return len(access.edges)
}

func (access *recordingAccess) GetEdgeData(index int) (EdgeData, bool) {
	if index < 0 || index >= len(access.edges) {
		return EdgeData{}, false
	}
	return access.edges[index], true
}

func (access *recordingAccess) PushEdge(index int, payload any) error {
	access.pushes = append(access.pushes, recordedPush{index: index, payload: payload})
	return nil
}

func TestDataProcessingNodeRegistrationSet(t *testing.T) {
	registry := NewNodeRegistry()
	if err := RegisterDataProcessingNodes(registry); err != nil {
		t.Fatalf("RegisterDataProcessingNodes() error = %v", err)
	}
	for _, pluginType := range []string{
		"core.merge", "core.map", "core.if", "core.filter", "core.subflow", "core.subflow-return",
	} {
		if _, exists := registry.Get(pluginType); !exists {
			t.Errorf("processing plugin %q is not registered", pluginType)
		}
	}
}

func TestMergeComposesGraphInputsBySourceNode(t *testing.T) {
	configuration := map[string]any{
		"primaryInputNodeId": "main",
		"mappings": []any{
			map[string]any{"key": "details", "sourceNodeId": "related"},
		},
	}
	payload := map[string]any{
		"inputs": []any{
			map[string]any{"sourceNodeId": "main", "value": map[string]any{"id": 42}},
			map[string]any{"sourceNodeId": "related", "value": []any{"a", "b"}},
		},
	}
	lifecycles := NewLifecycles()
	nodeContext := NewContext(ContextOptions{
		Runtime: context.Background(), Configuration: configuration,
		Payload: payload, Lifecycles: lifecycles,
	})
	if err := mergeRegistration().Handler(nodeContext); err != nil {
		t.Fatalf("Handler() error = %v", err)
	}
	output, err := lifecycles.InvokeRun()
	if err != nil {
		t.Fatalf("InvokeRun() error = %v", err)
	}
	want := map[string]any{"id": 42, "details": []any{"a", "b"}}
	if !reflect.DeepEqual(output, want) {
		t.Fatalf("Merge output = %#v, want %#v", output, want)
	}
}

func TestMergeRejectsLegacyAndCollidingConfiguration(t *testing.T) {
	if err := validateMerge(map[string]any{"dataType": "OBJECT"}); err == nil {
		t.Fatal("legacy Merge configuration error = nil")
	}
	configuration := map[string]any{
		"primaryInputNodeId": "main",
		"mappings": []any{
			map[string]any{"key": "id", "sourceNodeId": "related"},
		},
	}
	nodeContext := NewContext(ContextOptions{
		Runtime: context.Background(), Configuration: configuration,
		Payload: map[string]any{"inputs": []any{
			map[string]any{"sourceNodeId": "main", "value": map[string]any{"id": 42}},
			map[string]any{"sourceNodeId": "related", "value": "value"},
		}},
		Lifecycles: NewLifecycles(),
	})
	if _, err := mergeNode(nodeContext); err == nil {
		t.Fatal("colliding Merge mapping error = nil")
	}
}

func TestIfRoutesOriginalPayloadToSelectedPort(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		actual        string
		expectedIndex int
	}{
		{name: "yes", actual: "ready", expectedIndex: 0},
		{name: "no", actual: "blocked", expectedIndex: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload := map[string]any{"status": testCase.actual, "id": 42}
			access := &recordingAccess{edges: []EdgeData{
				{ID: "yes-edge", SourceOutputPort: "yes"},
				{ID: "no-edge", SourceOutputPort: "no"},
			}}
			output, err := ifNode(NewContext(ContextOptions{
				Configuration: map[string]any{
					"field": "status", "operator": conditionEquals, "value": "ready",
				},
				Payload: payload,
				Access:  access,
			}))
			if err != nil {
				t.Fatalf("ifNode() error = %v", err)
			}
			if !reflect.DeepEqual(output, payload) {
				t.Fatalf("ifNode() output = %#v, want %#v", output, payload)
			}
			if len(access.pushes) != 1 || access.pushes[0].index != testCase.expectedIndex ||
				!reflect.DeepEqual(access.pushes[0].payload, payload) {
				t.Fatalf("ifNode() pushes = %#v", access.pushes)
			}
		})
	}
}

func TestConditionFieldPathValidation(t *testing.T) {
	validPaths := []string{
		"balance",
		"legacy-field",
		"dbRecord.balance",
		"customer.address.city",
	}
	for _, field := range validPaths {
		t.Run("valid "+field, func(t *testing.T) {
			err := validateConditionConfiguration(map[string]any{
				"field": field, "operator": conditionEquals, "value": "expected",
			})
			if err != nil {
				t.Fatalf("validateConditionConfiguration() error = %v", err)
			}
		})
	}

	invalidPaths := []string{
		".dbRecord",
		"dbRecord.",
		"dbRecord..balance",
		"items[0].price",
		"$.dbRecord.balance",
		"dbRecord?.balance",
	}
	for _, field := range invalidPaths {
		t.Run("invalid "+field, func(t *testing.T) {
			err := validateConditionConfiguration(map[string]any{
				"field": field, "operator": conditionEquals, "value": "expected",
			})
			assertConditionNodeErrorCode(t, err, "CONDITION_FIELD_INVALID")
		})
	}
}

func TestEvaluateConfiguredConditionResolvesFieldPaths(t *testing.T) {
	payload := map[string]any{
		"balance": 1750,
		"dbRecord": map[string]any{
			"balance": 1750,
		},
		"customer": map[string]any{
			"address": map[string]any{
				"city": "Istanbul",
			},
		},
	}
	for _, testCase := range []struct {
		name     string
		field    string
		operator string
		value    any
		want     bool
	}{
		{name: "top-level numeric comparison", field: "balance", operator: conditionGreaterThanOrEqual, value: 1000, want: true},
		{name: "one-level nested numeric comparison", field: "dbRecord.balance", operator: conditionGreaterThanOrEqual, value: 1000, want: true},
		{name: "one-level nested numeric non-match", field: "dbRecord.balance", operator: conditionGreaterThanOrEqual, value: 2000, want: false},
		{name: "multi-level nested string comparison", field: "customer.address.city", operator: conditionEquals, value: "Istanbul", want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			matched, err := evaluateConfiguredCondition(map[string]any{
				"field": testCase.field, "operator": testCase.operator, "value": testCase.value,
			}, payload)
			if err != nil {
				t.Fatalf("evaluateConfiguredCondition() error = %v", err)
			}
			if matched != testCase.want {
				t.Fatalf("evaluateConfiguredCondition() = %t, want %t", matched, testCase.want)
			}
		})
	}
}

func TestEvaluateConfiguredConditionFieldPathErrors(t *testing.T) {
	configuration := func(field string) map[string]any {
		return map[string]any{"field": field, "operator": conditionEquals, "value": "expected"}
	}
	for _, testCase := range []struct {
		name     string
		field    string
		payload  map[string]any
		wantCode string
	}{
		{
			name: "missing nested member", field: "dbRecord.balance",
			payload: map[string]any{"dbRecord": map[string]any{}}, wantCode: "CONDITION_FIELD_MISSING",
		},
		{
			name: "null intermediate member", field: "dbRecord.balance",
			payload: map[string]any{"dbRecord": nil}, wantCode: "CONDITION_FIELD_NULL",
		},
		{
			name: "non-object intermediate member", field: "dbRecord.balance",
			payload: map[string]any{"dbRecord": "not-an-object"}, wantCode: "CONDITION_COMPARISON_INVALID",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := evaluateConfiguredCondition(configuration(testCase.field), testCase.payload)
			assertConditionNodeErrorCode(t, err, testCase.wantCode)
		})
	}
}

func assertConditionNodeErrorCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	var nodeError *NodeError
	if !errors.As(err, &nodeError) {
		t.Fatalf("error = %v, want *NodeError", err)
	}
	if nodeError.Code != wantCode {
		t.Fatalf("error code = %q, want %q", nodeError.Code, wantCode)
	}
}

func TestFilterForwardsOnlyMatchingOriginalPayload(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		actual         string
		expectedPushes int
	}{
		{name: "match", actual: "ready", expectedPushes: 1},
		{name: "no match", actual: "blocked", expectedPushes: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload := map[string]any{"status": testCase.actual, "id": 42}
			access := &recordingAccess{edges: []EdgeData{{ID: "output-edge", SourceOutputPort: "output"}}}
			output, err := filterNode(NewContext(ContextOptions{
				Configuration: map[string]any{
					"field": "status", "operator": conditionEquals, "value": "ready",
				},
				Payload: payload,
				Access:  access,
			}))
			if err != nil {
				t.Fatalf("filterNode() error = %v", err)
			}
			if !reflect.DeepEqual(output, payload) {
				t.Fatalf("filterNode() output = %#v, want %#v", output, payload)
			}
			if len(access.pushes) != testCase.expectedPushes {
				t.Fatalf("filterNode() pushes = %#v, want %d", access.pushes, testCase.expectedPushes)
			}
			if testCase.expectedPushes == 1 && !reflect.DeepEqual(access.pushes[0].payload, payload) {
				t.Fatalf("filterNode() payload = %#v, want %#v", access.pushes[0].payload, payload)
			}
		})
	}
}

func TestFilterResolvesNestedFieldWithoutChangingPayload(t *testing.T) {
	payload := map[string]any{
		"id": 42,
		"dbRecord": map[string]any{
			"balance": 1750,
		},
	}
	access := &recordingAccess{edges: []EdgeData{{ID: "output-edge", SourceOutputPort: "output"}}}
	output, err := filterNode(NewContext(ContextOptions{
		Configuration: map[string]any{
			"field": "dbRecord.balance", "operator": conditionGreaterThanOrEqual, "value": 1000,
		},
		Payload: payload,
		Access:  access,
	}))
	if err != nil {
		t.Fatalf("filterNode() error = %v", err)
	}
	if !reflect.DeepEqual(output, payload) {
		t.Fatalf("filterNode() output = %#v, want %#v", output, payload)
	}
	if len(access.pushes) != 1 || !reflect.DeepEqual(access.pushes[0].payload, payload) {
		t.Fatalf("filterNode() pushes = %#v, want original payload", access.pushes)
	}
}
