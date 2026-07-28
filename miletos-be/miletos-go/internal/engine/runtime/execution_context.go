package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

const (
	ProtectedKeyCompanyID           = "companyID"
	ProtectedKeyWorkflowID          = "workflowID"
	ProtectedKeyWorkflowExecutionID = "workflowExecutionID"
	ProtectedKeyExecutionMode       = "executionMode"
	ProtectedKeyCorrelationID       = "correlationID"
)

type ExecutionContext struct {
	parent              context.Context
	workflowExecutionID execution.WorkflowExecutionID
	companyID           workflow.CompanyID
	workflowID          workflow.WorkflowID
	mode                execution.ExecutionMode
	correlationID       string
	startedAt           time.Time
	edgeRegistry        map[workflow.EdgeID]*EdgeRuntime
	edgeOrder           []workflow.EdgeID
	variablesMu         sync.RWMutex
	variables           map[string]RuntimeValue
}

func NewExecutionContext(parent context.Context, workflowExecution execution.WorkflowExecution,
	correlationID string, edges []*EdgeRuntime, initialVariables map[string]RuntimeValue,
) (*ExecutionContext, error) {
	if parent == nil {
		return nil, newValidationError(
			"context", "must not be nil")
	}
	normalizedWorkflowExecutionID, err :=
		execution.NewWorkflowExecutionID(workflowExecution.ID().String())
	if err != nil {
		return nil, newValidationError("workflowExecution.id",
			err.Error())
	}
	normalizedCompanyID, err := workflow.NewCompanyID(workflowExecution.CompanyID().String())
	if err != nil {
		return nil, newValidationError(
			"workflowExecution.companyID", err.Error())
	}
	normalizedWorkflowID, err := workflow.NewWorkflowID(
		workflowExecution.WorkflowID().String())
	if err != nil {
		return nil, newValidationError("workflowExecution.workflowID", err.Error())
	}
	mode := workflowExecution.Mode()
	if !mode.IsValid() {
		return nil, newValidationError(
			"workflowExecution.mode", "must be a valid execution mode")
	}
	if workflowExecution.Status() !=
		execution.WorkflowExecutionStatusRunning {
		return nil, newValidationError("workflowExecution.status",
			"must be RUNNING")
	}
	startedAt, started := workflowExecution.StartedAt()
	if !started || startedAt.IsZero() {
		return nil, newValidationError("workflowExecution.startedAt", "must exist for a running execution")
	}
	normalizedCorrelationID, err := normalizeRequiredString("correlationID", correlationID)
	if err != nil {
		return nil, err
	}
	edgeRegistry, edgeOrder, err :=
		buildExecutionEdgeRegistry(edges)
	if err != nil {
		return nil, err
	}
	normalizedVariables, err :=
		normalizeInitialVariables(initialVariables)
	if err != nil {
		return nil, err
	}
	return &ExecutionContext{
		parent: parent, workflowExecutionID: normalizedWorkflowExecutionID,
		companyID: normalizedCompanyID, workflowID: normalizedWorkflowID, mode: mode,
		correlationID: normalizedCorrelationID, startedAt: startedAt,
		edgeRegistry: edgeRegistry, edgeOrder: edgeOrder,
		variables: normalizedVariables}, nil
}
func (executionContext *ExecutionContext) Context() context.Context {
	if executionContext == nil {
		return nil
	}
	return executionContext.parent
}
func (executionContext *ExecutionContext) Done() <-chan struct{} {
	if executionContext == nil || executionContext.parent == nil {
		return nil
	}
	return executionContext.parent.Done()
}
func (executionContext *ExecutionContext) Err() error {
	if executionContext == nil || executionContext.parent == nil {
		return nil
	}
	return executionContext.parent.Err()
}
func (executionContext *ExecutionContext) Deadline() (time.Time, bool,
) {
	if executionContext == nil || executionContext.parent == nil {
		return time.Time{}, false
	}
	return executionContext.parent.Deadline()
}
func (executionContext *ExecutionContext) WorkflowExecutionID() execution.WorkflowExecutionID {
	if executionContext == nil {
		return ""
	}
	return executionContext.workflowExecutionID
}
func (executionContext *ExecutionContext) CompanyID() workflow.CompanyID {
	if executionContext == nil {
		return ""
	}
	return executionContext.companyID
}
func (executionContext *ExecutionContext) WorkflowID() workflow.WorkflowID {
	if executionContext == nil {
		return ""
	}
	return executionContext.workflowID
}
func (executionContext *ExecutionContext) Mode() execution.ExecutionMode {
	if executionContext == nil {
		return ""
	}
	return executionContext.mode
}
func (executionContext *ExecutionContext) CorrelationID() string {
	if executionContext == nil {
		return ""
	}
	return executionContext.correlationID
}
func (executionContext *ExecutionContext) StartedAt() time.Time {
	if executionContext == nil {
		return time.Time{}
	}
	return executionContext.startedAt
}
func (executionContext *ExecutionContext) ProtectedMetadata() map[string]string {
	if executionContext == nil {
		return nil
	}
	return map[string]string{
		ProtectedKeyCompanyID: executionContext.companyID.String(), ProtectedKeyWorkflowID: executionContext.workflowID.String(),
		ProtectedKeyWorkflowExecutionID: executionContext.workflowExecutionID.
			String(), ProtectedKeyExecutionMode: executionContext.mode.String(),
		ProtectedKeyCorrelationID: executionContext.correlationID}
}
func (executionContext *ExecutionContext) Edge(
	id workflow.EdgeID) (*EdgeRuntime, bool, error) {
	if executionContext == nil {
		return nil, false, newValidationError("executionContext", "must not be nil")
	}
	normalizedID, err := workflow.NewEdgeID(id.String())
	if err != nil {
		return nil, false, newValidationError("edgeID",
			err.Error())
	}
	edgeRuntime, exists := executionContext.edgeRegistry[normalizedID]
	if !exists {
		return nil, false, nil
	}
	return edgeRuntime, true, nil
}
func (executionContext *ExecutionContext) Edges() []*EdgeRuntime {
	if executionContext == nil || len(executionContext.edgeOrder) == 0 {
		return nil
	}
	edges := make(
		[]*EdgeRuntime, 0, len(executionContext.edgeOrder),
	)
	for _, edgeID := range executionContext.edgeOrder {
		edges = append(edges, executionContext.edgeRegistry[edgeID])
	}
	return edges
}
func (executionContext *ExecutionContext) SetVariable(key string, value RuntimeValue,
) error {
	if executionContext == nil {
		return newValidationError(
			"executionContext", "must not be nil")
	}
	normalizedKey, err := normalizeRuntimeVariableKey(
		"variable.key", key)
	if err != nil {
		return err
	}
	if !value.IsValid() {
		return newValidationError(
			"variable.value", "must contain valid JSON")
	}
	executionContext.variablesMu.Lock()
	defer executionContext.variablesMu.Unlock()
	executionContext.variables[normalizedKey] =
		cloneRuntimeValue(value)
	return nil
}
func (executionContext *ExecutionContext) Variable(
	key string) (RuntimeValue, bool, error) {
	if executionContext == nil {
		return RuntimeValue{}, false, newValidationError("executionContext", "must not be nil")
	}
	normalizedKey, err := normalizeRuntimeVariableKey("variable.key", key)
	if err != nil {
		return RuntimeValue{}, false, err
	}
	executionContext.variablesMu.RLock()
	defer executionContext.variablesMu.RUnlock()
	value, exists :=
		executionContext.variables[normalizedKey]
	if !exists {
		return RuntimeValue{}, false, nil
	}
	return cloneRuntimeValue(value), true, nil
}
func (executionContext *ExecutionContext) DeleteVariable(key string) (RuntimeValue, bool, error) {
	if executionContext == nil {
		return RuntimeValue{}, false, newValidationError("executionContext",
			"must not be nil")
	}
	normalizedKey, err := normalizeRuntimeVariableKey("variable.key",
		key)
	if err != nil {
		return RuntimeValue{}, false, err
	}
	executionContext.variablesMu.Lock()
	defer executionContext.variablesMu.Unlock()
	value, exists := executionContext.variables[normalizedKey]
	if !exists {
		return RuntimeValue{}, false, nil
	}
	delete(executionContext.variables,
		normalizedKey)
	return cloneRuntimeValue(value), true, nil
}
func (executionContext *ExecutionContext) VariablesSnapshot() map[string]RuntimeValue {
	if executionContext == nil {
		return nil
	}
	executionContext.variablesMu.RLock()
	defer executionContext.variablesMu.RUnlock()
	if len(executionContext.variables) == 0 {
		return nil
	}
	snapshot := make(map[string]RuntimeValue, len(executionContext.variables))
	for key, value := range executionContext.variables {
		snapshot[key] = cloneRuntimeValue(value)
	}
	return snapshot
}
func buildExecutionEdgeRegistry(edges []*EdgeRuntime) (
	map[workflow.EdgeID]*EdgeRuntime, []workflow.EdgeID, error,
) {
	if len(edges) == 0 {
		return map[workflow.EdgeID]*EdgeRuntime{},
			nil, nil
	}
	registry := make(map[workflow.EdgeID]*EdgeRuntime,
		len(edges))
	order := make([]workflow.EdgeID, 0,
		len(edges))
	for index, edgeRuntime := range edges {
		field := fmt.Sprintf("edges[%d]",
			index)
		if err := validateExecutionEdgeRuntime(field, edgeRuntime); err != nil {
			return nil, nil, err
		}
		normalizedEdgeID, err := workflow.NewEdgeID(edgeRuntime.ID().String())
		if err != nil {
			return nil, nil, newValidationError(
				field+".id", err.Error())
		}
		if _, exists := registry[normalizedEdgeID]; exists {
			return nil, nil, newValidationError("edges", fmt.Sprintf(
				"contains duplicate edge ID %q", normalizedEdgeID.String()),
			)
		}
		registry[normalizedEdgeID] = edgeRuntime
		order = append(order,
			normalizedEdgeID)
	}
	sort.Slice(order,
		func(left int, right int) bool {
			return order[left].String() < order[right].String()
		})
	return registry, order, nil
}
func validateExecutionEdgeRuntime(field string, edgeRuntime *EdgeRuntime,
) error {
	if edgeRuntime == nil {
		return newValidationError(
			field, "must not be nil")
	}
	if _, err := workflow.NewEdgeID(
		edgeRuntime.ID().String()); err != nil {
		return newValidationError(
			field+".id", err.Error())
	}
	if _, err := workflow.NewNodeID(
		edgeRuntime.SourceNodeID().String()); err != nil {
		return newValidationError(
			field+".sourceNodeID", err.Error())
	}
	if strings.TrimSpace(
		edgeRuntime.SourceOutputPort()) == "" {
		return newValidationError(
			field+".sourceOutputPort", "must not be empty")
	}
	if _, err := workflow.NewNodeID(
		edgeRuntime.TargetNodeID().String()); err != nil {
		return newValidationError(
			field+".targetNodeID", err.Error())
	}
	if strings.TrimSpace(
		edgeRuntime.TargetInputPort()) == "" {
		return newValidationError(
			field+".targetInputPort", "must not be empty")
	}
	if edgeRuntime.Queue() == nil {
		return newValidationError(field+".queue", "must not be nil")
	}
	if edgeRuntime.Cache() == nil {
		return newValidationError(field+".cache",
			"must not be nil")
	}
	return nil
}
func normalizeInitialVariables(initialVariables map[string]RuntimeValue,
) (map[string]RuntimeValue, error) {
	if len(initialVariables) == 0 {
		return map[string]RuntimeValue{}, nil
	}
	normalized := make(
		map[string]RuntimeValue, len(initialVariables))
	for key, value := range initialVariables {
		normalizedKey, err := normalizeRuntimeVariableKey(
			"initialVariables.key", key)
		if err != nil {
			return nil, err
		}
		if _, exists := normalized[normalizedKey]; exists {
			return nil, newValidationError(
				"initialVariables", fmt.Sprintf("contains duplicate key %q after normalization",
					normalizedKey))
		}
		if !value.IsValid() {
			return nil, newValidationError("initialVariables.value", fmt.Sprintf(
				"value for key %q must contain valid JSON", normalizedKey),
			)
		}
		normalized[normalizedKey] = cloneRuntimeValue(value)
	}
	return normalized, nil
}
func normalizeRuntimeVariableKey(field string,
	value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field,
			"must not be empty")
	}
	if isProtectedMetadataKey(normalized) {
		return "", newValidationError(
			field, "must not use a protected metadata key")
	}
	return normalized, nil
}
func isProtectedMetadataKey(key string) bool {
	return strings.EqualFold(key, ProtectedKeyCompanyID) ||
		strings.EqualFold(key, ProtectedKeyWorkflowID) ||
		strings.EqualFold(key, ProtectedKeyWorkflowExecutionID) ||
		strings.EqualFold(key, ProtectedKeyExecutionMode) ||
		strings.EqualFold(key, ProtectedKeyCorrelationID)
}
