package dataarrival

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/plugin"
	pluginstate "miletos-go/internal/features/workflow-runtime/plugin-state"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

const (
	cursorStateKey    = "data-arrival-cursor"
	sourcePollTimeout = 30 * time.Second
)

type Service struct {
	bindings             *Repository
	workflows            *workflow.WorkflowRepository
	validator            *workflow.WorkflowService
	executions           *execution.ExecutionService
	registry             *plugin.NodeRegistry
	state                *pluginstate.Repository
	wake                 chan struct{}
	incompatibleMutex    sync.RWMutex
	incompatibleBindings map[incompatibleBindingKey]string
}

type incompatibleBindingKey struct {
	CompanyID        string
	WorkflowID       string
	WorkflowRevision uint64
	SnapshotID       string
	NodeID           string
	PluginType       string
}

func NewService(
	bindings *Repository,
	workflows *workflow.WorkflowRepository,
	validator *workflow.WorkflowService,
	executions *execution.ExecutionService,
	registry *plugin.NodeRegistry,
	state *pluginstate.Repository,
) *Service {
	return &Service{
		bindings: bindings, workflows: workflows, validator: validator,
		executions: executions, registry: registry, state: state,
		wake: make(chan struct{}, 1), incompatibleBindings: make(map[incompatibleBindingKey]string),
	}
}

func (service *Service) Activate(ctx context.Context, definition workflow.Workflow) error {
	if err := service.validator.Validate(definition); err != nil {
		return err
	}
	bindings := make([]Binding, 0)
	for _, node := range workflow.Roots(definition) {
		registration, exists := service.registry.Get(node.Type)
		if !exists || registration.DataArrivalSource == nil {
			continue
		}
		source := registration.DataArrivalSource
		if source.Enabled != nil && !source.Enabled(node.Configuration) {
			continue
		}
		if source.Validate != nil {
			if err := source.Validate(node.Configuration); err != nil {
				return err
			}
		}
		bindings = append(bindings, Binding{
			CompanyID: definition.CompanyID, WorkflowID: definition.ID,
			WorkflowRevision: definition.Revision, NodeID: node.ID, PluginType: node.Type,
		})
	}
	if len(bindings) > 0 && !service.executions.AsyncAvailable() {
		return execution.ErrAsyncUnavailable
	}
	snapshot := workflow.WorkflowSnapshot{
		ID: deterministicSnapshotID(definition), Workflow: definition, CreatedAt: time.Now().UTC(),
	}
	for index := range bindings {
		bindings[index].SnapshotID = snapshot.ID
	}
	if err := service.bindings.Activate(ctx, snapshot, bindings); err != nil {
		return err
	}
	service.clearWorkflowIncompatibilities(definition.CompanyID, definition.ID)
	select {
	case service.wake <- struct{}{}:
	default:
	}
	return nil
}

func (service *Service) DisableWorkflow(
	ctx context.Context,
	companyID string,
	workflowID string,
) (int, error) {
	disabled, err := service.bindings.DisableWorkflow(ctx, companyID, workflowID)
	if err != nil {
		return 0, err
	}
	service.clearWorkflowIncompatibilities(companyID, workflowID)
	return disabled, nil
}

func (service *Service) Poll(ctx context.Context) error {
	bindings, err := service.bindings.ListActive(ctx)
	if err != nil {
		return err
	}
	var pollErrors error
	for _, binding := range bindings {
		if err := ctx.Err(); err != nil {
			return err
		}
		if service.bindingIncompatibility(binding) != "" {
			continue
		}
		pollContext, cancel := context.WithTimeout(ctx, sourcePollTimeout)
		err := service.pollBinding(pollContext, binding)
		cancel()
		if err != nil {
			if code, incompatible := plugin.SFTPIncompatibilityCode(err); incompatible &&
				service.rememberBindingIncompatibility(binding, code) {
				slog.Warn(
					"suppress incompatible data-arrival binding",
					"companyId", binding.CompanyID,
					"workflowId", binding.WorkflowID,
					"workflowRevision", binding.WorkflowRevision,
					"snapshotId", binding.SnapshotID,
					"nodeId", binding.NodeID,
					"pluginType", binding.PluginType,
					"incompatibilityCode", code,
				)
			}
			pollErrors = errors.Join(pollErrors, fmt.Errorf(
				"poll data-arrival source %s/%s/%d/%s: %w",
				binding.CompanyID, binding.WorkflowID, binding.WorkflowRevision, binding.NodeID, err,
			))
		}
	}
	return pollErrors
}

func (service *Service) bindingIncompatibility(binding Binding) string {
	service.incompatibleMutex.RLock()
	defer service.incompatibleMutex.RUnlock()
	return service.incompatibleBindings[incompatibleKey(binding)]
}

func (service *Service) rememberBindingIncompatibility(binding Binding, code string) bool {
	service.incompatibleMutex.Lock()
	defer service.incompatibleMutex.Unlock()
	key := incompatibleKey(binding)
	if _, exists := service.incompatibleBindings[key]; exists {
		return false
	}
	service.incompatibleBindings[key] = code
	return true
}

func (service *Service) clearWorkflowIncompatibilities(companyID, workflowID string) {
	service.incompatibleMutex.Lock()
	defer service.incompatibleMutex.Unlock()
	for key := range service.incompatibleBindings {
		if key.CompanyID == companyID && key.WorkflowID == workflowID {
			delete(service.incompatibleBindings, key)
		}
	}
}

func incompatibleKey(binding Binding) incompatibleBindingKey {
	return incompatibleBindingKey{
		CompanyID: binding.CompanyID, WorkflowID: binding.WorkflowID,
		WorkflowRevision: binding.WorkflowRevision, SnapshotID: binding.SnapshotID,
		NodeID: binding.NodeID, PluginType: binding.PluginType,
	}
}

func (service *Service) pollBinding(ctx context.Context, binding Binding) error {
	snapshot, err := service.workflows.FindBySnapshotID(ctx, binding.CompanyID, binding.SnapshotID)
	if err != nil {
		return err
	}
	if snapshot.Workflow.CompanyID != binding.CompanyID ||
		snapshot.Workflow.ID != binding.WorkflowID ||
		snapshot.Workflow.Revision != binding.WorkflowRevision {
		return fmt.Errorf("data-arrival binding snapshot does not match its workflow identity")
	}
	node, exists := findNode(snapshot.Workflow, binding.NodeID)
	if !exists || node.Type != binding.PluginType {
		return fmt.Errorf("data-arrival binding node does not match its workflow snapshot")
	}
	registration, exists := service.registry.Get(node.Type)
	if !exists || registration.DataArrivalSource == nil || registration.DataArrivalSource.Poll == nil {
		return fmt.Errorf("data-arrival source capability is unavailable")
	}
	storage := pluginstate.NewStorage(ctx, service.state, pluginstate.Scope{
		CompanyID: binding.CompanyID, WorkflowID: binding.WorkflowID,
		WorkflowRevision: binding.WorkflowRevision, NodeID: binding.NodeID,
	})
	cursor, found, err := storage.Get(cursorStateKey)
	if err != nil {
		return err
	}
	if !found {
		cursor = nil
	}
	events, err := registration.DataArrivalSource.Poll(
		ctx, binding.CompanyID, node.Configuration, cursor,
	)
	if err != nil {
		return err
	}
	for _, event := range events {
		active, err := service.bindings.IsActive(ctx, binding)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}
		if event.CheckpointOnly {
			if err := storage.Set(cursorStateKey, event.Cursor); err != nil {
				return err
			}
			continue
		}
		if strings.TrimSpace(event.Key) == "" {
			return fmt.Errorf("data-arrival event key is required")
		}
		idempotencyKey := dataArrivalExecutionKey(binding, event.Key)
		fingerprint := execution.Fingerprint(map[string]any{
			"snapshotId": binding.SnapshotID, "nodeId": binding.NodeID,
			"eventKey": event.Key,
		})
		if _, err := service.executions.ExecuteDataArrivalFromSnapshot(
			ctx, snapshot, binding.NodeID, event.Payload,
			idempotencyKey, idempotencyKey, fingerprint,
		); err != nil {
			return err
		}
		if err := storage.Set(cursorStateKey, event.Cursor); err != nil {
			return err
		}
	}
	return nil
}

func deterministicSnapshotID(definition workflow.Workflow) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%d", definition.CompanyID, definition.ID, definition.Revision,
	)))
	return "arrival_snapshot_" + hex.EncodeToString(digest[:])
}

func dataArrivalExecutionKey(binding Binding, eventKey string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		binding.CompanyID, binding.WorkflowID,
		fmt.Sprintf("%d", binding.WorkflowRevision), binding.NodeID, eventKey,
	}, "\x00")))
	return "arrival:" + hex.EncodeToString(digest[:])
}

func findNode(definition workflow.Workflow, nodeID string) (workflow.WorkflowNode, bool) {
	for _, node := range definition.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return workflow.WorkflowNode{}, false
}
