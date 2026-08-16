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
	"miletos-go/internal/features/workflow-runtime/execution/repository"
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

type scenarioStartInfrastructure struct {
	service *Service
	ctx     context.Context
	request CreateRequest
	created Binding
}

func newScenarioStartInfrastructure(
	service *Service,
	ctx context.Context,
	request CreateRequest,
) *scenarioStartInfrastructure {
	return &scenarioStartInfrastructure{service: service, ctx: ctx, request: request}
}

func (infrastructure *scenarioStartInfrastructure) CreateCronTrigger(
	expression string,
	timezone string,
) error {
	infrastructure.request.Expression = expression
	infrastructure.request.Timezone = timezone
	created, err := infrastructure.service.createBinding(
		infrastructure.ctx,
		infrastructure.request,
	)
	if err != nil {
		return err
	}
	infrastructure.created = created
	return nil
}

func (infrastructure *scenarioStartInfrastructure) Result() Binding {
	return infrastructure.created
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
	if err := service.workflowService.Validate(request.Workflow); err != nil {
		return Binding{}, err
	}
	rootNode, err := resolveTriggerRoot(
		request.Workflow, request.TriggerNodeID, service.registry,
	)
	if err != nil {
		return Binding{}, err
	}
	registration, exists := service.registry.Get(rootNode.Type)
	if !exists {
		return Binding{}, ErrInvalidTrigger
	}
	lifecycles := plugin.NewLifecycles()
	cronInfrastructure := newScenarioStartInfrastructure(service, ctx, request)
	pluginContext := plugin.NewContext(plugin.ContextOptions{
		Runtime:        ctx,
		Configuration:  rootNode.Configuration,
		CompanyID:      request.CompanyID,
		Lifecycles:     lifecycles,
		Infrastructure: plugin.Infrastructure{Cron: cronInfrastructure},
	})
	if err := registration.Handler(pluginContext); err != nil {
		return Binding{}, ErrInvalidTrigger
	}
	if err := lifecycles.InvokeScenarioStart(); err != nil {
		return Binding{}, ErrInvalidTrigger
	}
	return cronInfrastructure.Result(), nil
}

func (service *Service) createBinding(
	ctx context.Context,
	request CreateRequest,
) (Binding, error) {
	nextFireAt, schedule, err := cronexpr.Next(
		request.Expression, request.Timezone, time.Now().UTC(),
	)
	if err != nil {
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

func (service *Service) GetActiveByWorkflowAndNode(
	ctx context.Context,
	companyID string,
	workflowID string,
	triggerNodeID string,
) (Binding, error) {
	return service.triggers.FindActiveByWorkflowAndNode(
		ctx, companyID, workflowID, triggerNodeID,
	)
}

func (service *Service) Disable(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	binding, changed, err := service.triggers.Disable(ctx, companyID, triggerID)
	if err != nil {
		return Binding{}, err
	}
	if !changed && binding.Status != StatusDisabled {
		return Binding{}, repository.ErrStateTransition
	}
	return binding, nil
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
	if _, err := resolveTriggerRoot(
		snapshot.Workflow,
		occurrence.TriggerNodeID,
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

func resolveTriggerRoot(
	definition workflow.Workflow,
	triggerNodeID string,
	registry *plugin.NodeRegistry,
) (workflow.WorkflowNode, error) {
	if strings.TrimSpace(triggerNodeID) == "" {
		return workflow.WorkflowNode{}, ErrInvalidTrigger
	}
	for _, rootNode := range workflow.Roots(definition) {
		if rootNode.ID != triggerNodeID {
			continue
		}
		if !registry.DeclaresExecutionSource(
			rootNode.Type, string(executionmodel.ExecutionOriginCron),
		) {
			return workflow.WorkflowNode{}, ErrInvalidTrigger
		}
		return rootNode, nil
	}
	return workflow.WorkflowNode{}, ErrInvalidTrigger
}

func secureID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}
