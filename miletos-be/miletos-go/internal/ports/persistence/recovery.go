package repository

import (
	"context"
	"time"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

const MaximumPartialRecoveryPlanBytes = 1 << 20

type RecoveryNodeDisposition string

const (
	RecoveryNodePreserved RecoveryNodeDisposition = "PRESERVED"
	RecoveryNodeScheduled RecoveryNodeDisposition = "SCHEDULED"
	RecoveryNodeReset     RecoveryNodeDisposition = "RESET"
)

func (disposition RecoveryNodeDisposition) IsValid() bool {
	switch disposition {
	case RecoveryNodePreserved, RecoveryNodeScheduled, RecoveryNodeReset:
		return true
	default:
		return false
	}
}

type RecoveryNodePlan struct {
	sourceNodeExecutionID execution.NodeExecutionID
	nodeExecution         NodeExecutionRecord
	sourceStatus          execution.NodeExecutionStatus
	sourceAttempt         int16
	disposition           RecoveryNodeDisposition
}

func NewRecoveryNodePlan(
	sourceNodeExecutionID execution.NodeExecutionID,
	nodeExecution NodeExecutionRecord,
	sourceStatus execution.NodeExecutionStatus,
	sourceAttempt int16,
	disposition RecoveryNodeDisposition,
) (RecoveryNodePlan, error) {
	normalizedSourceID, err := execution.NewNodeExecutionID(sourceNodeExecutionID.String())
	if err != nil {
		return RecoveryNodePlan{}, newValidationError("sourceNodeExecutionID", err.Error())
	}
	if !nodeExecution.IsValid() {
		return RecoveryNodePlan{}, newValidationError("nodeExecution", "must be valid")
	}
	if !sourceStatus.IsTerminal() {
		return RecoveryNodePlan{}, newValidationError("sourceStatus", "must be terminal")
	}
	if sourceAttempt <= 0 {
		return RecoveryNodePlan{}, newValidationError("sourceAttempt", "must be greater than zero")
	}
	if !disposition.IsValid() {
		return RecoveryNodePlan{}, newValidationError("disposition", "must be valid")
	}
	switch disposition {
	case RecoveryNodePreserved:
		if sourceStatus != execution.NodeExecutionStatusSucceeded ||
			nodeExecution.Status() != execution.NodeExecutionStatusSucceeded {
			return RecoveryNodePlan{}, newValidationError(
				"disposition", "PRESERVED requires succeeded source and recovery nodes")
		}
	case RecoveryNodeScheduled:
		if sourceStatus != execution.NodeExecutionStatusFailed &&
			sourceStatus != execution.NodeExecutionStatusTimedOut {
			return RecoveryNodePlan{}, newValidationError(
				"disposition", "SCHEDULED requires a failed or timed out source node")
		}
		if nodeExecution.Status() != execution.NodeExecutionStatusQueued {
			return RecoveryNodePlan{}, newValidationError(
				"nodeExecution.status", "SCHEDULED recovery node must be QUEUED")
		}
	case RecoveryNodeReset:
		if sourceStatus != execution.NodeExecutionStatusSkipped ||
			nodeExecution.Status() != execution.NodeExecutionStatusPending {
			return RecoveryNodePlan{}, newValidationError(
				"disposition", "RESET requires a skipped source and pending recovery node")
		}
	}
	return RecoveryNodePlan{
		sourceNodeExecutionID: normalizedSourceID,
		nodeExecution:         nodeExecution,
		sourceStatus:          sourceStatus,
		sourceAttempt:         sourceAttempt,
		disposition:           disposition,
	}, nil
}

func (plan RecoveryNodePlan) SourceNodeExecutionID() execution.NodeExecutionID {
	return plan.sourceNodeExecutionID
}

func (plan RecoveryNodePlan) NodeExecution() NodeExecutionRecord {
	return plan.nodeExecution
}

func (plan RecoveryNodePlan) SourceStatus() execution.NodeExecutionStatus {
	return plan.sourceStatus
}

func (plan RecoveryNodePlan) SourceAttempt() int16 {
	return plan.sourceAttempt
}

func (plan RecoveryNodePlan) Disposition() RecoveryNodeDisposition {
	return plan.disposition
}

func (plan RecoveryNodePlan) IsValid() bool {
	_, err := NewRecoveryNodePlan(
		plan.sourceNodeExecutionID,
		plan.nodeExecution,
		plan.sourceStatus,
		plan.sourceAttempt,
		plan.disposition,
	)
	return err == nil
}

type PartialRecoveryCommandParams struct {
	CompanyID                 workflow.CompanyID
	IdempotencyKey            string
	RequestFingerprint        string
	SourceWorkflowExecutionID execution.WorkflowExecutionID
	ExpectedSourceStatus      execution.WorkflowExecutionStatus
	ExpectedSourceLockVersion int64
	RecoveryWorkflowExecution WorkflowExecutionRecord
	NodePlans                 []RecoveryNodePlan
	ContextVariables          []AsyncContextVariable
	NodeInputs                []AsyncNodeInput
	OutboxMessages            []OutboxMessage
	Timeline                  []TimelineEntry
	RecoveryPlan              []byte
	CreatedAt                 time.Time
}

type PartialRecoveryCommand struct {
	params       PartialRecoveryCommandParams
	recoveryPlan JSONObject
}

func NewPartialRecoveryCommand(params PartialRecoveryCommandParams) (PartialRecoveryCommand, error) {
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return PartialRecoveryCommand{}, newValidationError("companyID", err.Error())
	}
	idempotencyKey, err := normalizeRequiredBoundedString(
		"idempotencyKey", params.IdempotencyKey, MaximumHTTPIdempotencyKeyCharacters)
	if err != nil {
		return PartialRecoveryCommand{}, err
	}
	requestFingerprint, err := normalizeHTTPIdempotencyFingerprint(params.RequestFingerprint)
	if err != nil {
		return PartialRecoveryCommand{}, err
	}
	sourceExecutionID, err := execution.NewWorkflowExecutionID(
		params.SourceWorkflowExecutionID.String())
	if err != nil {
		return PartialRecoveryCommand{}, newValidationError("sourceWorkflowExecutionID", err.Error())
	}
	if params.ExpectedSourceStatus != execution.WorkflowExecutionStatusFailed &&
		params.ExpectedSourceStatus != execution.WorkflowExecutionStatusTimedOut {
		return PartialRecoveryCommand{}, newValidationError(
			"expectedSourceStatus", "must be FAILED or TIMED_OUT")
	}
	if params.ExpectedSourceLockVersion < 0 {
		return PartialRecoveryCommand{}, newValidationError(
			"expectedSourceLockVersion", "must not be negative")
	}
	recoveryExecution := params.RecoveryWorkflowExecution
	if !recoveryExecution.IsValid() ||
		recoveryExecution.CompanyID() != companyID ||
		recoveryExecution.ID() == sourceExecutionID ||
		recoveryExecution.Mode() != execution.ExecutionModeAsync ||
		recoveryExecution.Status() != execution.WorkflowExecutionStatusRunning {
		return PartialRecoveryCommand{}, newValidationError(
			"recoveryWorkflowExecution", "must be a distinct, valid, running async execution in the same company")
	}
	if len(params.NodePlans) == 0 {
		return PartialRecoveryCommand{}, newValidationError("nodePlans", "must not be empty")
	}
	nodePlans := append([]RecoveryNodePlan(nil), params.NodePlans...)
	nodeIDs := make(map[workflow.NodeID]struct{}, len(nodePlans))
	scheduled := make(map[execution.NodeExecutionID]struct{})
	plansByExecutionID := make(map[execution.NodeExecutionID]RecoveryNodePlan, len(nodePlans))
	for _, plan := range nodePlans {
		if !plan.IsValid() ||
			plan.NodeExecution().CompanyID() != companyID ||
			plan.NodeExecution().WorkflowExecutionID() != recoveryExecution.ID() {
			return PartialRecoveryCommand{}, newValidationError(
				"nodePlans", "must contain valid recovery-scoped node plans")
		}
		nodeID := plan.NodeExecution().NodeID()
		if _, exists := nodeIDs[nodeID]; exists {
			return PartialRecoveryCommand{}, newValidationError(
				"nodePlans", "must contain each node exactly once")
		}
		nodeIDs[nodeID] = struct{}{}
		plansByExecutionID[plan.NodeExecution().ID()] = plan
		if plan.Disposition() == RecoveryNodeScheduled {
			scheduled[plan.NodeExecution().ID()] = struct{}{}
		}
	}
	contextVariables := append([]AsyncContextVariable(nil), params.ContextVariables...)
	contextKeys := make(map[string]struct{}, len(contextVariables))
	for _, variable := range contextVariables {
		if !variable.IsValid() || variable.CompanyID() != companyID ||
			variable.WorkflowExecutionID() != recoveryExecution.ID() {
			return PartialRecoveryCommand{}, newValidationError(
				"contextVariables", "must contain valid recovery-scoped values")
		}
		if _, exists := contextKeys[variable.Key()]; exists {
			return PartialRecoveryCommand{}, newValidationError(
				"contextVariables", "must contain each context key exactly once")
		}
		contextKeys[variable.Key()] = struct{}{}
	}
	nodeInputs := append([]AsyncNodeInput(nil), params.NodeInputs...)
	inputIdentities := make(map[string]struct{}, len(nodeInputs))
	for _, input := range nodeInputs {
		if !input.IsValid() || input.CompanyID() != companyID ||
			input.WorkflowExecutionID() != recoveryExecution.ID() {
			return PartialRecoveryCommand{}, newValidationError(
				"nodeInputs", "must contain valid recovery-scoped inputs")
		}
		sourcePlan, sourceExists := plansByExecutionID[input.SourceNodeExecutionID()]
		targetPlan, targetExists := plansByExecutionID[input.TargetNodeExecutionID()]
		if !sourceExists || !targetExists ||
			sourcePlan.Disposition() != RecoveryNodePreserved ||
			targetPlan.Disposition() == RecoveryNodePreserved ||
			sourcePlan.NodeExecution().NodeID() != input.SourceNodeID() ||
			targetPlan.NodeExecution().NodeID() != input.TargetNodeID() ||
			sourcePlan.NodeExecution().Attempt() != input.SourceAttempt() {
			return PartialRecoveryCommand{}, newValidationError(
				"nodeInputs", "must flow from a preserved node to a recoverable node")
		}
		if _, exists := inputIdentities[input.IdentityKey()]; exists {
			return PartialRecoveryCommand{}, newValidationError(
				"nodeInputs", "must contain each logical input exactly once")
		}
		inputIdentities[input.IdentityKey()] = struct{}{}
	}
	outboxMessages := append([]OutboxMessage(nil), params.OutboxMessages...)
	if len(outboxMessages) != len(scheduled) {
		return PartialRecoveryCommand{}, newValidationError(
			"outboxMessages", "must contain exactly one command for every scheduled recovery node")
	}
	seenOutboxNodes := make(map[execution.NodeExecutionID]struct{}, len(outboxMessages))
	for _, message := range outboxMessages {
		nodeExecutionID, hasNode := message.NodeExecutionID()
		if !message.IsValid() || !hasNode ||
			message.CompanyID() != companyID ||
			message.WorkflowExecutionID() != recoveryExecution.ID() ||
			message.OperationKind() != OutboxOperationNodeCommand ||
			message.PublicationState() != OutboxPublicationPending ||
			!message.AvailableAt().Equal(message.CreatedAt()) {
			return PartialRecoveryCommand{}, newValidationError(
				"outboxMessages", "must contain immediate pending recovery node commands")
		}
		if _, exists := scheduled[nodeExecutionID]; !exists {
			return PartialRecoveryCommand{}, newValidationError(
				"outboxMessages", "must target a scheduled recovery node")
		}
		scheduledPlan := plansByExecutionID[nodeExecutionID]
		messageNodeID, _ := message.NodeID()
		messageAttempt, _ := message.Attempt()
		if messageNodeID != scheduledPlan.NodeExecution().NodeID() ||
			messageAttempt != scheduledPlan.NodeExecution().Attempt() {
			return PartialRecoveryCommand{}, newValidationError(
				"outboxMessages", "must match the scheduled recovery node identity and attempt")
		}
		if _, exists := seenOutboxNodes[nodeExecutionID]; exists {
			return PartialRecoveryCommand{}, newValidationError(
				"outboxMessages", "must target each scheduled recovery node once")
		}
		seenOutboxNodes[nodeExecutionID] = struct{}{}
	}
	timeline := append([]TimelineEntry(nil), params.Timeline...)
	if len(timeline) == 0 {
		return PartialRecoveryCommand{}, newValidationError("timeline", "must not be empty")
	}
	for _, entry := range timeline {
		if !entry.IsValid() {
			return PartialRecoveryCommand{}, newValidationError("timeline", "must contain valid entries")
		}
		switch entry.Kind() {
		case TimelineEntryKindEvent:
			event, _ := entry.Event()
			if event.CompanyID() != companyID ||
				event.WorkflowExecutionID() != recoveryExecution.ID() {
				return PartialRecoveryCommand{}, newValidationError(
					"timeline", "must contain recovery-scoped entries")
			}
		case TimelineEntryKindLog:
			log, _ := entry.Log()
			if log.CompanyID() != companyID ||
				log.WorkflowExecutionID() != recoveryExecution.ID() {
				return PartialRecoveryCommand{}, newValidationError(
					"timeline", "must contain recovery-scoped entries")
			}
		}
	}
	recoveryPlan, err := newJSONObject("recoveryPlan", params.RecoveryPlan)
	if err != nil {
		return PartialRecoveryCommand{}, err
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return PartialRecoveryCommand{}, err
	}
	if !recoveryExecution.CreatedAt().Equal(createdAt) {
		return PartialRecoveryCommand{}, newValidationError(
			"createdAt", "must equal the recovery workflow creation time")
	}
	for _, variable := range contextVariables {
		if !variable.CreatedAt().Equal(createdAt) || !variable.UpdatedAt().Equal(createdAt) {
			return PartialRecoveryCommand{}, newValidationError(
				"contextVariables", "must be created with the recovery request")
		}
	}
	for _, input := range nodeInputs {
		if !input.CreatedAt().Equal(createdAt) {
			return PartialRecoveryCommand{}, newValidationError(
				"nodeInputs", "must be created with the recovery request")
		}
	}
	for _, message := range outboxMessages {
		if !message.CreatedAt().Equal(createdAt) {
			return PartialRecoveryCommand{}, newValidationError(
				"outboxMessages", "must be created with the recovery request")
		}
	}
	if len(recoveryPlan.Bytes()) > MaximumPartialRecoveryPlanBytes {
		return PartialRecoveryCommand{}, newValidationError(
			"recoveryPlan", "exceeds the maximum partial recovery plan size")
	}
	return PartialRecoveryCommand{
		params: PartialRecoveryCommandParams{
			CompanyID: companyID, IdempotencyKey: idempotencyKey,
			RequestFingerprint:        requestFingerprint,
			SourceWorkflowExecutionID: sourceExecutionID,
			ExpectedSourceStatus:      params.ExpectedSourceStatus,
			ExpectedSourceLockVersion: params.ExpectedSourceLockVersion,
			RecoveryWorkflowExecution: recoveryExecution,
			NodePlans:                 nodePlans, ContextVariables: contextVariables,
			NodeInputs: nodeInputs, OutboxMessages: outboxMessages,
			Timeline: timeline, RecoveryPlan: recoveryPlan.Bytes(),
			CreatedAt: createdAt,
		},
		recoveryPlan: recoveryPlan,
	}, nil
}

func (command PartialRecoveryCommand) CompanyID() workflow.CompanyID {
	return command.params.CompanyID
}
func (command PartialRecoveryCommand) IdempotencyKey() string {
	return command.params.IdempotencyKey
}
func (command PartialRecoveryCommand) RequestFingerprint() string {
	return command.params.RequestFingerprint
}
func (command PartialRecoveryCommand) SourceWorkflowExecutionID() execution.WorkflowExecutionID {
	return command.params.SourceWorkflowExecutionID
}
func (command PartialRecoveryCommand) ExpectedSourceStatus() execution.WorkflowExecutionStatus {
	return command.params.ExpectedSourceStatus
}
func (command PartialRecoveryCommand) ExpectedSourceLockVersion() int64 {
	return command.params.ExpectedSourceLockVersion
}
func (command PartialRecoveryCommand) RecoveryWorkflowExecution() WorkflowExecutionRecord {
	return command.params.RecoveryWorkflowExecution
}
func (command PartialRecoveryCommand) NodePlans() []RecoveryNodePlan {
	return append([]RecoveryNodePlan(nil), command.params.NodePlans...)
}
func (command PartialRecoveryCommand) ContextVariables() []AsyncContextVariable {
	return append([]AsyncContextVariable(nil), command.params.ContextVariables...)
}
func (command PartialRecoveryCommand) NodeInputs() []AsyncNodeInput {
	return append([]AsyncNodeInput(nil), command.params.NodeInputs...)
}
func (command PartialRecoveryCommand) OutboxMessages() []OutboxMessage {
	return append([]OutboxMessage(nil), command.params.OutboxMessages...)
}
func (command PartialRecoveryCommand) Timeline() []TimelineEntry {
	return append([]TimelineEntry(nil), command.params.Timeline...)
}
func (command PartialRecoveryCommand) RecoveryPlan() JSONObject {
	return command.recoveryPlan
}
func (command PartialRecoveryCommand) CreatedAt() time.Time {
	return command.params.CreatedAt
}
func (command PartialRecoveryCommand) IsValid() bool {
	_, err := NewPartialRecoveryCommand(command.params)
	return err == nil
}

type PartialRecoveryRecord struct {
	companyID                   workflow.CompanyID
	idempotencyKey              string
	requestFingerprint          string
	sourceWorkflowExecutionID   execution.WorkflowExecutionID
	recoveryWorkflowExecutionID execution.WorkflowExecutionID
	preservedNodeCount          int
	scheduledNodeCount          int
	resetNodeCount              int
	createdAt                   time.Time
}

func NewPartialRecoveryRecord(
	companyID workflow.CompanyID,
	idempotencyKey string,
	requestFingerprint string,
	sourceWorkflowExecutionID execution.WorkflowExecutionID,
	recoveryWorkflowExecutionID execution.WorkflowExecutionID,
	preservedNodeCount int,
	scheduledNodeCount int,
	resetNodeCount int,
	createdAt time.Time,
) (PartialRecoveryRecord, error) {
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return PartialRecoveryRecord{}, newValidationError("companyID", err.Error())
	}
	normalizedKey, err := normalizeRequiredBoundedString(
		"idempotencyKey", idempotencyKey, MaximumHTTPIdempotencyKeyCharacters)
	if err != nil {
		return PartialRecoveryRecord{}, err
	}
	normalizedFingerprint, err := normalizeHTTPIdempotencyFingerprint(requestFingerprint)
	if err != nil {
		return PartialRecoveryRecord{}, err
	}
	sourceID, err := execution.NewWorkflowExecutionID(sourceWorkflowExecutionID.String())
	if err != nil {
		return PartialRecoveryRecord{}, newValidationError("sourceWorkflowExecutionID", err.Error())
	}
	recoveryID, err := execution.NewWorkflowExecutionID(recoveryWorkflowExecutionID.String())
	if err != nil {
		return PartialRecoveryRecord{}, newValidationError("recoveryWorkflowExecutionID", err.Error())
	}
	if sourceID == recoveryID {
		return PartialRecoveryRecord{}, newValidationError(
			"recoveryWorkflowExecutionID", "must differ from source execution")
	}
	if preservedNodeCount < 0 || scheduledNodeCount <= 0 || resetNodeCount < 0 {
		return PartialRecoveryRecord{}, newValidationError(
			"nodeCounts", "must contain a positive scheduled count and non-negative remaining counts")
	}
	normalizedCreatedAt, err := normalizeRequiredRecordTime("createdAt", createdAt)
	if err != nil {
		return PartialRecoveryRecord{}, err
	}
	return PartialRecoveryRecord{
		companyID: normalizedCompanyID, idempotencyKey: normalizedKey,
		requestFingerprint:          normalizedFingerprint,
		sourceWorkflowExecutionID:   sourceID,
		recoveryWorkflowExecutionID: recoveryID,
		preservedNodeCount:          preservedNodeCount,
		scheduledNodeCount:          scheduledNodeCount,
		resetNodeCount:              resetNodeCount,
		createdAt:                   normalizedCreatedAt,
	}, nil
}

func (record PartialRecoveryRecord) CompanyID() workflow.CompanyID {
	return record.companyID
}
func (record PartialRecoveryRecord) IdempotencyKey() string {
	return record.idempotencyKey
}
func (record PartialRecoveryRecord) RequestFingerprint() string {
	return record.requestFingerprint
}
func (record PartialRecoveryRecord) SourceWorkflowExecutionID() execution.WorkflowExecutionID {
	return record.sourceWorkflowExecutionID
}
func (record PartialRecoveryRecord) RecoveryWorkflowExecutionID() execution.WorkflowExecutionID {
	return record.recoveryWorkflowExecutionID
}
func (record PartialRecoveryRecord) PreservedNodeCount() int {
	return record.preservedNodeCount
}
func (record PartialRecoveryRecord) ScheduledNodeCount() int {
	return record.scheduledNodeCount
}
func (record PartialRecoveryRecord) ResetNodeCount() int {
	return record.resetNodeCount
}
func (record PartialRecoveryRecord) CreatedAt() time.Time {
	return record.createdAt
}
func (record PartialRecoveryRecord) IsValid() bool {
	_, err := NewPartialRecoveryRecord(
		record.companyID, record.idempotencyKey, record.requestFingerprint,
		record.sourceWorkflowExecutionID, record.recoveryWorkflowExecutionID,
		record.preservedNodeCount, record.scheduledNodeCount,
		record.resetNodeCount, record.createdAt,
	)
	return err == nil
}

type PartialRecoveryStore interface {
	CreatePartialRecovery(
		ctx context.Context,
		command PartialRecoveryCommand,
	) (PartialRecoveryRecord, bool, error)
}
