package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	pluginstate "miletos-go/internal/features/workflow-runtime/plugin-state"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

var ErrAmbiguousRecordSources = errors.New("workflow has multiple independent record sources")

type ManualExecutionBatch struct {
	Executions          []model.Execution
	WorkflowID          string
	WorkflowRevision    uint64
	SnapshotID          string
	Mode                string
	Status              string
	CorrelationID       string
	ScheduledEntryNodes int
	Replayed            bool
}

type manualRecordManifest struct {
	RequestFingerprint string           `json:"requestFingerprint"`
	SnapshotID         string           `json:"snapshotId"`
	Records            []map[string]any `json:"records"`
}

func (service *ExecutionService) ExecuteManual(
	ctx context.Context,
	definition workflow.Workflow,
	startInput map[string]any,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
	requestedMode string,
) (ManualExecutionBatch, error) {
	if err := service.workflows.Validate(definition); err != nil {
		return ManualExecutionBatch{}, err
	}
	sourceNode, registration, found, err := service.manualRecordSource(definition)
	if err != nil {
		return ManualExecutionBatch{}, err
	}
	if !found {
		var outcome ExecutionOutcome
		if strings.EqualFold(requestedMode, "ASYNC") {
			outcome, err = service.ExecuteAsync(
				ctx, definition, startInput, correlationID, idempotencyKey, fingerprint,
			)
		} else {
			outcome, err = service.ExecuteSync(
				ctx, definition, startInput, correlationID, idempotencyKey, fingerprint,
			)
		}
		if err != nil {
			return ManualExecutionBatch{}, err
		}
		return manualBatchFromOutcome(outcome), nil
	}
	if err := service.validateExecutionStart(
		definition, startInput, model.ExecutionOriginManualDirect,
	); err != nil {
		return ManualExecutionBatch{}, err
	}
	if !service.asyncEnabled {
		return ManualExecutionBatch{}, ErrAsyncUnavailable
	}
	manifest, manifestReplayed, err := service.loadOrCreateManualRecordManifest(
		ctx, definition, sourceNode, registration, idempotencyKey, fingerprint,
	)
	if err != nil {
		return ManualExecutionBatch{}, err
	}
	records := manifest.Records
	batch := ManualExecutionBatch{
		WorkflowID: definition.ID, WorkflowRevision: definition.Revision,
		Mode: "ASYNC", Status: "NO_RECORDS", CorrelationID: correlationID,
		Executions: make([]model.Execution, 0, len(records)),
		SnapshotID: manifest.SnapshotID, Replayed: manifestReplayed,
	}
	if len(records) == 0 {
		return batch, nil
	}
	snapshot, err := service.workflows.FindSnapshot(
		ctx, definition.CompanyID, manifest.SnapshotID,
	)
	if err != nil {
		return ManualExecutionBatch{}, err
	}
	batch.Status = "DISPATCHED"
	commonSnapshotID := ""
	mixedSnapshots := false
	for index, record := range records {
		if record == nil {
			return ManualExecutionBatch{}, fmt.Errorf("record source %s materialized a nil object", sourceNode.ID)
		}
		recordKey := sourceRecordIdempotencyKey(idempotencyKey, sourceNode.ID, index)
		recordFingerprint := sourceRecordFingerprint(
			fingerprint, sourceNode.ID, index, record,
		)
		outcome, dispatchErr := service.ExecuteDispatchedFromSnapshot(
			ctx,
			snapshot,
			sourceNode.ID,
			SourceDispatch{Output: record},
			model.ExecutionOriginManualDirect,
			correlationID,
			recordKey,
			recordFingerprint,
		)
		if dispatchErr != nil {
			return ManualExecutionBatch{}, dispatchErr
		}
		batch.Executions = append(batch.Executions, outcome.Execution)
		if commonSnapshotID == "" {
			commonSnapshotID = outcome.Execution.SnapshotID
		} else if commonSnapshotID != outcome.Execution.SnapshotID {
			mixedSnapshots = true
		}
		batch.ScheduledEntryNodes += outcome.ScheduledEntryNodes
		batch.Replayed = batch.Replayed && outcome.Replayed
	}
	if !mixedSnapshots {
		batch.SnapshotID = commonSnapshotID
	}
	return batch, nil
}

func (service *ExecutionService) loadOrCreateManualRecordManifest(
	ctx context.Context,
	definition workflow.Workflow,
	sourceNode workflow.WorkflowNode,
	registration plugin.NodeRegistration,
	idempotencyKey string,
	requestFingerprint string,
) (manualRecordManifest, bool, error) {
	if service.recordSourceState == nil {
		return manualRecordManifest{}, false, fmt.Errorf(
			"record source manifest persistence is unavailable",
		)
	}
	storage := pluginstate.NewStorage(ctx, service.recordSourceState, pluginstate.Scope{
		CompanyID: definition.CompanyID, WorkflowID: definition.ID,
		WorkflowRevision: definition.Revision, NodeID: sourceNode.ID,
	})
	stateKey := "manual-record-manifest:" + Fingerprint(map[string]any{
		"idempotencyKey": idempotencyKey,
	})
	stored, found, err := storage.Get(stateKey)
	if err != nil {
		return manualRecordManifest{}, false, err
	}
	if found {
		manifest, err := decodeManualRecordManifest(stored, requestFingerprint)
		return manifest, true, err
	}

	records, err := registration.RecordSource.Materialize(
		ctx, definition.CompanyID, sourceNode.Configuration,
	)
	if err != nil {
		return manualRecordManifest{}, false, err
	}
	for _, record := range records {
		if record == nil {
			return manualRecordManifest{}, false, fmt.Errorf(
				"record source %s materialized a nil object", sourceNode.ID,
			)
		}
	}
	snapshot, err := service.workflows.CreateWorkflow(ctx, definition)
	if err != nil {
		return manualRecordManifest{}, false, err
	}
	candidate := manualRecordManifest{
		RequestFingerprint: requestFingerprint,
		SnapshotID:         snapshot.ID,
		Records:            records,
	}
	persisted, err := storage.Update(stateKey, func(current any, exists bool) (any, error) {
		if !exists {
			return candidate, nil
		}
		manifest, decodeErr := decodeManualRecordManifest(current, requestFingerprint)
		if decodeErr != nil {
			return nil, decodeErr
		}
		return manifest, nil
	})
	if err != nil {
		return manualRecordManifest{}, false, err
	}
	manifest, err := decodeManualRecordManifest(persisted, requestFingerprint)
	if err != nil {
		return manualRecordManifest{}, false, err
	}
	return manifest, manifest.SnapshotID != candidate.SnapshotID, nil
}

func decodeManualRecordManifest(
	value any,
	requestFingerprint string,
) (manualRecordManifest, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return manualRecordManifest{}, fmt.Errorf("encode record source manifest: %w", err)
	}
	var manifest manualRecordManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return manualRecordManifest{}, fmt.Errorf("decode record source manifest: %w", err)
	}
	if manifest.RequestFingerprint != requestFingerprint {
		return manualRecordManifest{}, repository.ErrIdempotencyConflict
	}
	if strings.TrimSpace(manifest.SnapshotID) == "" || manifest.Records == nil {
		return manualRecordManifest{}, fmt.Errorf("record source manifest is incomplete")
	}
	return manifest, nil
}

func sourceRecordFingerprint(
	requestFingerprint string,
	sourceNodeID string,
	index int,
	record map[string]any,
) string {
	return Fingerprint(map[string]any{
		"requestFingerprint": requestFingerprint,
		"sourceNodeId":       sourceNodeID,
		"recordIndex":        index,
		"record":             record,
	})
}

func manualBatchFromOutcome(outcome ExecutionOutcome) ManualExecutionBatch {
	return ManualExecutionBatch{
		Executions:          []model.Execution{outcome.Execution},
		WorkflowID:          outcome.Execution.WorkflowID,
		WorkflowRevision:    outcome.Execution.WorkflowRevision,
		SnapshotID:          outcome.Execution.SnapshotID,
		Mode:                outcome.Execution.Mode,
		Status:              string(outcome.Execution.Status),
		CorrelationID:       outcome.Execution.CorrelationID,
		ScheduledEntryNodes: outcome.ScheduledEntryNodes,
		Replayed:            outcome.Replayed,
	}
}

func sourceRecordIdempotencyKey(requestKey string, sourceNodeID string, index int) string {
	return "source:" + Fingerprint(map[string]any{
		"requestKey": requestKey, "sourceNodeId": sourceNodeID, "recordIndex": index,
	})
}

func (service *ExecutionService) manualRecordSource(
	definition workflow.Workflow,
) (workflow.WorkflowNode, plugin.NodeRegistration, bool, error) {
	var sourceNode workflow.WorkflowNode
	var sourceRegistration plugin.NodeRegistration
	count := 0
	for _, root := range workflow.Roots(definition) {
		registration, exists := service.scheduler.registry.Get(root.Type)
		if !exists || registration.RecordSource == nil || registration.RecordSource.Materialize == nil {
			continue
		}
		if service.isPassiveRecordSource(definition, root.ID) {
			continue
		}
		if !service.recordSourceHasDefinedRouting(definition, root.ID) {
			return workflow.WorkflowNode{}, plugin.NodeRegistration{}, false, ErrAmbiguousRecordSources
		}
		count++
		sourceNode = root
		sourceRegistration = registration
	}
	if count > 1 {
		return workflow.WorkflowNode{}, plugin.NodeRegistration{}, false, ErrAmbiguousRecordSources
	}
	return sourceNode, sourceRegistration, count == 1, nil
}

func (service *ExecutionService) recordSourceHasDefinedRouting(
	definition workflow.Workflow,
	sourceNodeID string,
) bool {
	for _, edge := range definition.Edges {
		if edge.SourceNodeID != sourceNodeID {
			continue
		}
		if service.recordSourceRelation(definition, edge, sourceNodeID) !=
			plugin.RecordSourceRelationDriving {
			return false
		}
	}
	return true
}

func (service *ExecutionService) isPassiveRecordSource(
	definition workflow.Workflow,
	sourceNodeID string,
) bool {
	hasOutgoing := false
	for _, edge := range definition.Edges {
		if edge.SourceNodeID != sourceNodeID {
			continue
		}
		hasOutgoing = true
		if service.recordSourceRelation(definition, edge, sourceNodeID) !=
			plugin.RecordSourceRelationPassive {
			return false
		}
	}
	return hasOutgoing
}

func (service *ExecutionService) recordSourceRelation(
	definition workflow.Workflow,
	edge workflow.Edge,
	sourceNodeID string,
) plugin.RecordSourceRelation {
	target, exists := findWorkflowNode(definition, edge.TargetNodeID)
	if !exists {
		return plugin.RecordSourceRelationDriving
	}
	registration, exists := service.scheduler.registry.Get(target.Type)
	if !exists || registration.RecordSourceRelation == nil {
		return plugin.RecordSourceRelationDriving
	}
	relation := registration.RecordSourceRelation(target.Configuration, sourceNodeID)
	switch relation {
	case plugin.RecordSourceRelationDriving, plugin.RecordSourceRelationPassive:
		return relation
	default:
		return plugin.RecordSourceRelationUnsupported
	}
}
