package crontrigger

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/cronexpr"
	"miletos-go/internal/features/workflow-runtime/execution"
	executionmodel "miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

var (
	ErrInvalidTrigger       = errors.New("cron trigger registration is invalid")
	ErrBindingInvariant     = errors.New("cron trigger binding invariant failed")
	ErrSchedulerUnavailable = errors.New("cron scheduler is unavailable")
)

type Service struct {
	triggers        *Repository
	workflows       *workflow.WorkflowRepository
	workflowService *workflow.WorkflowService
	executions      *execution.ExecutionService
	registry        *plugin.NodeRegistry
}

type CreateRequest struct {
	CompanyID     string
	Workflow      workflow.Workflow
	TriggerNodeID string
	Expression    string
	Timezone      string
	ResolvedMode  ResolvedMode
}

func NewService(
	triggers *Repository,
	workflows *workflow.WorkflowRepository,
	workflowService *workflow.WorkflowService,
	executions *execution.ExecutionService,
	registry *plugin.NodeRegistry,
) *Service {
	return &Service{
		triggers:        triggers,
		workflows:       workflows,
		workflowService: workflowService,
		executions:      executions,
		registry:        registry,
	}
}

func (service *Service) Create(
	ctx context.Context,
	request CreateRequest,
) (Binding, error) {
	if request.ResolvedMode != ModeAsync {
		return Binding{}, ErrInvalidTrigger
	}
	if !service.executions.AsyncAvailable() {
		return Binding{}, execution.ErrAsyncUnavailable
	}
	request.Workflow.CompanyID = request.CompanyID
	// Graph and plugin validation runs first so that an invalid cron expression
	// or timezone is reported with its stable plugin validation code instead of
	// a generic invalid-trigger error.
	if err := service.workflowService.Validate(request.Workflow); err != nil {
		return Binding{}, err
	}
	nextFireAt, schedule, err := cronexpr.Next(
		request.Expression, request.Timezone, time.Now().UTC(),
	)
	if err != nil {
		return Binding{}, ErrInvalidTrigger
	}
	if err := validateTriggerRoot(
		request.Workflow, request.TriggerNodeID, schedule.Expression, schedule.Timezone,
		service.registry,
	); err != nil {
		return Binding{}, ErrInvalidTrigger
	}
	triggerID, err := secureID("cron_trigger_")
	if err != nil {
		return Binding{}, fmt.Errorf("generate cron trigger ID: %w", err)
	}
	snapshotID, err := secureID("snapshot_")
	if err != nil {
		return Binding{}, fmt.Errorf("generate cron trigger snapshot ID: %w", err)
	}
	now := time.Now().UTC()
	binding := Binding{
		ID: triggerID, CompanyID: request.CompanyID,
		WorkflowID: request.Workflow.ID, WorkflowRevision: request.Workflow.Revision,
		SnapshotID: snapshotID, TriggerNodeID: request.TriggerNodeID,
		Expression: schedule.Expression, Timezone: schedule.Timezone,
		Status: StatusActive, NextFireAt: nextFireAt,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := service.triggers.Create(ctx, binding, request.Workflow); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

func (service *Service) Get(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	return service.triggers.FindByID(ctx, companyID, triggerID)
}

func (service *Service) GetActiveByWorkflow(
	ctx context.Context,
	companyID string,
	workflowID string,
) (Binding, error) {
	return service.triggers.FindActiveByWorkflow(ctx, companyID, workflowID)
}

func (service *Service) Disable(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	return service.triggers.Disable(ctx, companyID, triggerID)
}

func (service *Service) DisableWorkflow(ctx context.Context, companyID, workflowID string) (int, error) {
	return service.triggers.DisableWorkflow(ctx, companyID, workflowID)
}

func (service *Service) Fire(
	ctx context.Context,
	occurrence DueOccurrence,
) (execution.ExecutionOutcome, error) {
	if !service.executions.AsyncAvailable() {
		return execution.ExecutionOutcome{}, execution.ErrAsyncUnavailable
	}
	snapshot, err := service.workflows.FindBySnapshotID(
		ctx, occurrence.CompanyID, occurrence.SnapshotID,
	)
	if err != nil {
		return execution.ExecutionOutcome{}, err
	}
	if snapshot.Workflow.CompanyID != occurrence.CompanyID ||
		snapshot.Workflow.ID != occurrence.WorkflowID ||
		snapshot.Workflow.Revision != occurrence.WorkflowRevision {
		return execution.ExecutionOutcome{}, ErrBindingInvariant
	}
	if err := validateTriggerRoot(
		snapshot.Workflow,
		occurrence.TriggerNodeID,
		occurrence.Expression,
		occurrence.Timezone,
		service.registry,
	); err != nil {
		return execution.ExecutionOutcome{}, ErrBindingInvariant
	}
	payload := map[string]any{
		"triggerId":      occurrence.ID,
		"cronExpression": occurrence.Expression,
		"timezone":       occurrence.Timezone,
		"scheduledAt":    occurrence.ScheduledAt.UTC().Format(time.RFC3339Nano),
		"firedAt":        occurrence.FiredAt.UTC().Format(time.RFC3339Nano),
	}
	key := fmt.Sprintf("cron:%s:%s", occurrence.ID, occurrence.ScheduledAt.UTC().Format(time.RFC3339Nano))
	fingerprintPayload := map[string]any{
		"triggerId":   occurrence.ID,
		"snapshotId":  occurrence.SnapshotID,
		"scheduledAt": occurrence.ScheduledAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, _ := json.Marshal(fingerprintPayload)
	digest := sha256.Sum256(encoded)
	return service.executions.ExecuteTriggerFromSnapshot(
		ctx,
		snapshot,
		occurrence.TriggerNodeID,
		payload,
		executionmodel.ExecutionOriginCron,
		key,
		key,
		hex.EncodeToString(digest[:]),
	)
}

func validateTriggerRoot(
	definition workflow.Workflow,
	triggerNodeID string,
	expression string,
	timezone string,
	registry *plugin.NodeRegistry,
) error {
	rootNodes := workflow.Roots(definition)
	if len(rootNodes) != 1 {
		return ErrInvalidTrigger
	}
	rootNode := rootNodes[0]
	if rootNode.ID != triggerNodeID {
		return ErrInvalidTrigger
	}
	if !registry.DeclaresExecutionSource(
		rootNode.Type,
		rootNode.Version,
		string(executionmodel.ExecutionOriginCron),
	) {
		return ErrInvalidTrigger
	}
	return validateBindingConfiguration(rootNode.Configuration, expression, timezone)
}

func validateBindingConfiguration(
	configuration map[string]any,
	expression string,
	timezone string,
) error {
	configuredExpression, ok := configuration["expression"].(string)
	if !ok {
		return ErrInvalidTrigger
	}
	configuredTimezone, _ := configuration["timezone"].(string)
	configured, err := cronexpr.Parse(configuredExpression, configuredTimezone)
	if err != nil {
		return ErrInvalidTrigger
	}
	binding, err := cronexpr.Parse(expression, timezone)
	if err != nil {
		return ErrInvalidTrigger
	}
	if configured.Expression != binding.Expression || configured.Timezone != binding.Timezone {
		return ErrInvalidTrigger
	}
	return nil
}

func secureID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func NormalizeConfiguration(configuration map[string]any) (string, string, error) {
	expression, _ := configuration["expression"].(string)
	timezone, _ := configuration["timezone"].(string)
	schedule, err := cronexpr.Parse(expression, timezone)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(schedule.Expression), schedule.Timezone, nil
}
