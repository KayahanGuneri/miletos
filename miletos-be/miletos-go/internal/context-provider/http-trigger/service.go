package httptrigger

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution"
	executionmodel "miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	pluginstate "miletos-go/internal/features/workflow-runtime/plugin-state"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

var (
	ErrInvalidTrigger       = errors.New("HTTP trigger registration is invalid")
	ErrPublicURLUnavailable = errors.New("public HTTP trigger URL is not configured")
	ErrMethodNotAllowed     = errors.New("HTTP trigger method is not allowed")
	ErrBindingInvariant     = errors.New("HTTP trigger binding invariant failed")
)

type Service struct {
	publicBaseURL   string
	triggers        *Repository
	workflows       *workflow.WorkflowRepository
	workflowService *workflow.WorkflowService
	executions      *execution.ExecutionService
	registry        *plugin.NodeRegistry
	state           *pluginstate.Repository
	emitter         *execution.PluginEmitter
}

type CreateRequest struct {
	CompanyID     string
	Workflow      workflow.Workflow
	TriggerNodeID string
	Method        string
	ResolvedMode  ResolvedMode
}

type scenarioStartInfrastructure struct {
	service *Service
	ctx     context.Context
	request CreateRequest
	created CreatedBinding
}

func newScenarioStartInfrastructure(
	service *Service,
	ctx context.Context,
	request CreateRequest,
) *scenarioStartInfrastructure {
	return &scenarioStartInfrastructure{service: service, ctx: ctx, request: request}
}

func (infrastructure *scenarioStartInfrastructure) CreateHTTPURL(
	method plugin.HTTPMethod,
) (plugin.HTTPBinding, error) {
	infrastructure.request.Method = string(method)
	created, err := infrastructure.service.createBinding(
		infrastructure.ctx,
		infrastructure.request,
	)
	if err != nil {
		return plugin.HTTPBinding{}, err
	}
	infrastructure.created = created
	return plugin.HTTPBinding{
		URL: created.PublicURL, Identity: created.Binding.ID,
	}, nil
}

func (infrastructure *scenarioStartInfrastructure) OnRequest(plugin.HTTPRequestHandler) error {
	return nil
}

func (infrastructure *scenarioStartInfrastructure) Result() CreatedBinding {
	return infrastructure.created
}

type requestInfrastructure struct {
	currentIdentity string
	method          string
	payload         any
	handler         plugin.HTTPRequestHandler
}

func (infrastructure *requestInfrastructure) CreateHTTPURL(
	method plugin.HTTPMethod,
) (plugin.HTTPBinding, error) {
	if string(method) != infrastructure.method || infrastructure.currentIdentity == "" {
		return plugin.HTTPBinding{}, ErrInvalidTrigger
	}
	return plugin.HTTPBinding{Identity: infrastructure.currentIdentity}, nil
}

func (infrastructure *requestInfrastructure) OnRequest(
	handler plugin.HTTPRequestHandler,
) error {
	if handler == nil {
		return ErrInvalidTrigger
	}
	infrastructure.handler = handler
	return nil
}

func (infrastructure *requestInfrastructure) Dispatch() (any, error) {
	if infrastructure.handler == nil {
		return nil, ErrInvalidTrigger
	}
	return infrastructure.handler(infrastructure.currentIdentity, infrastructure.payload)
}

func NewService(
	publicBaseURL string,
	triggers *Repository,
	workflows *workflow.WorkflowRepository,
	workflowService *workflow.WorkflowService,
	executions *execution.ExecutionService,
	registry *plugin.NodeRegistry,
	state *pluginstate.Repository,
	emitter *execution.PluginEmitter,
) *Service {
	return &Service{
		publicBaseURL:   strings.TrimRight(publicBaseURL, "/"),
		triggers:        triggers,
		workflows:       workflows,
		workflowService: workflowService,
		executions:      executions,
		registry:        registry,
		state:           state,
		emitter:         emitter,
	}
}

func (service *Service) StartScenario(
	ctx context.Context,
	request CreateRequest,
) (CreatedBinding, error) {
	request.Workflow.CompanyID = request.CompanyID
	if request.ResolvedMode != ModeAsync {
		return CreatedBinding{}, ErrInvalidTrigger
	}
	if err := service.workflowService.Validate(request.Workflow); err != nil {
		return CreatedBinding{}, err
	}
	if err := validateTriggerRoot(
		request.Workflow, request.TriggerNodeID, service.registry,
	); err != nil {
		return CreatedBinding{}, ErrInvalidTrigger
	}
	rootNode, valid := rootNodeByID(request.Workflow, request.TriggerNodeID)
	if !valid {
		return CreatedBinding{}, ErrInvalidTrigger
	}
	registration, exists := service.registry.Get(rootNode.Type)
	if !exists {
		return CreatedBinding{}, fmt.Errorf(
			"%w: node %q", plugin.ErrNodeRegistrationNotFound, rootNode.Type,
		)
	}
	lifecycles := plugin.NewLifecycles()
	httpInfrastructure := newScenarioStartInfrastructure(service, ctx, request)
	infrastructure := plugin.Infrastructure{HTTP: httpInfrastructure}
	if service.emitter != nil {
		infrastructure.Emitter = service.emitter.ForContext(ctx, registration.Key)
	}
	pluginContext := plugin.NewContext(plugin.ContextOptions{
		Runtime:       ctx,
		Configuration: rootNode.Configuration,
		CompanyID:     request.CompanyID,
		Lifecycles:    lifecycles,
		Storage: pluginstate.NewStorage(ctx, service.state, pluginstate.Scope{
			CompanyID:        request.CompanyID,
			WorkflowID:       request.Workflow.ID,
			WorkflowRevision: request.Workflow.Revision,
			NodeID:           rootNode.ID,
		}),
		Access: execution.NewNodeAccess(
			request.Workflow, rootNode.ID, registration.RoutingMode,
		),
		Infrastructure: infrastructure,
	})
	if err := registration.Handler(pluginContext); err != nil {
		return CreatedBinding{}, err
	}
	if err := lifecycles.InvokeScenarioStart(); err != nil {
		return CreatedBinding{}, err
	}
	return httpInfrastructure.Result(), nil
}

func (service *Service) createBinding(
	ctx context.Context,
	request CreateRequest,
) (CreatedBinding, error) {
	if service.publicBaseURL == "" {
		return CreatedBinding{}, ErrPublicURLUnavailable
	}
	rawToken, tokenHash, err := secureToken()
	if err != nil {
		return CreatedBinding{}, fmt.Errorf("generate HTTP trigger credential: %w", err)
	}
	triggerID, err := secureID("trigger_")
	if err != nil {
		return CreatedBinding{}, fmt.Errorf("generate HTTP trigger ID: %w", err)
	}
	snapshotID, err := secureID("snapshot_")
	if err != nil {
		return CreatedBinding{}, fmt.Errorf("generate HTTP trigger snapshot ID: %w", err)
	}
	now := time.Now().UTC()
	binding := Binding{
		ID:               triggerID,
		CompanyID:        request.CompanyID,
		WorkflowID:       request.Workflow.ID,
		WorkflowRevision: request.Workflow.Revision,
		SnapshotID:       snapshotID,
		TriggerNodeID:    request.TriggerNodeID,
		Method:           request.Method,
		TokenHash:        tokenHash,
		Status:           StatusActive,
		ResolvedMode:     request.ResolvedMode,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := service.triggers.Create(ctx, binding, request.Workflow); err != nil {
		return CreatedBinding{}, err
	}
	binding.TokenHash = nil
	return CreatedBinding{
		Binding:   binding,
		PublicURL: service.publicBaseURL + "/hooks/" + rawToken,
	}, nil
}

func validateTriggerRoot(
	definition workflow.Workflow,
	triggerNodeID string,
	registry *plugin.NodeRegistry,
) error {
	rootNode, valid := rootNodeByID(definition, triggerNodeID)
	if !valid {
		return ErrInvalidTrigger
	}
	if !registry.DeclaresExecutionSource(
		rootNode.Type,
		string(executionmodel.ExecutionOriginHTTPWebhook),
	) {
		return ErrInvalidTrigger
	}
	return nil
}

func rootNodeByID(definition workflow.Workflow, triggerNodeID string) (workflow.WorkflowNode, bool) {
	for _, rootNode := range workflow.Roots(definition) {
		if rootNode.ID == triggerNodeID {
			return rootNode, true
		}
	}
	return workflow.WorkflowNode{}, false
}

func (service *Service) Get(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	binding, err := service.triggers.FindByID(ctx, companyID, triggerID)
	binding.TokenHash = nil
	return binding, err
}

func (service *Service) GetActiveByWorkflowAndNode(
	ctx context.Context,
	companyID string,
	workflowID string,
	triggerNodeID string,
) (Binding, error) {
	binding, err := service.triggers.FindActiveByWorkflowAndNode(
		ctx, companyID, workflowID, triggerNodeID,
	)
	binding.TokenHash = nil
	return binding, err
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
	binding.TokenHash = nil
	return binding, nil
}

func (service *Service) DisableWorkflow(ctx context.Context, companyID, workflowID string) (int, error) {
	return service.triggers.DisableWorkflow(ctx, companyID, workflowID)
}

func (service *Service) Invoke(
	ctx context.Context,
	rawToken string,
	method string,
	payload map[string]any,
	correlationID string,
	externalIdempotencyKey string,
) (execution.ExecutionOutcome, string, error) {
	digest := sha256.Sum256([]byte(rawToken))
	binding, err := service.triggers.FindActiveByTokenHash(ctx, digest[:])
	if err != nil {
		return execution.ExecutionOutcome{}, "", err
	}
	if method != binding.Method {
		return execution.ExecutionOutcome{}, binding.Method, ErrMethodNotAllowed
	}
	if binding.ResolvedMode != ModeAsync {
		return execution.ExecutionOutcome{}, "", ErrBindingInvariant
	}
	snapshot, err := service.workflows.FindBySnapshotID(
		ctx, binding.CompanyID, binding.SnapshotID,
	)
	if err != nil {
		return execution.ExecutionOutcome{}, "", err
	}
	if snapshot.Workflow.CompanyID != binding.CompanyID ||
		snapshot.Workflow.ID != binding.WorkflowID ||
		snapshot.Workflow.Revision != binding.WorkflowRevision {
		return execution.ExecutionOutcome{}, "", ErrBindingInvariant
	}
	if err := validateTriggerRoot(
		snapshot.Workflow,
		binding.TriggerNodeID,
		service.registry,
	); err != nil {
		return execution.ExecutionOutcome{}, "", ErrBindingInvariant
	}
	dispatch, err := service.dispatchRequest(
		ctx, snapshot.Workflow, binding, payload,
	)
	if err != nil {
		return execution.ExecutionOutcome{}, "", err
	}
	key := externalIdempotencyKey
	if key == "" {
		key, err = secureID("hook_call_")
		if err != nil {
			return execution.ExecutionOutcome{}, "", fmt.Errorf(
				"generate HTTP trigger invocation ID: %w", err,
			)
		}
	}
	outcome, err := service.executions.ExecuteDispatchedFromSnapshot(
		ctx, snapshot, binding.TriggerNodeID, dispatch,
		executionmodel.ExecutionOriginHTTPWebhook, correlationID,
		key, invocationFingerprint(binding, payload),
	)
	return outcome, "", err
}

func (service *Service) dispatchRequest(
	ctx context.Context,
	definition workflow.Workflow,
	binding Binding,
	payload map[string]any,
) (execution.SourceDispatch, error) {
	node, exists := rootNodeByID(definition, binding.TriggerNodeID)
	if !exists {
		return execution.SourceDispatch{}, ErrBindingInvariant
	}
	registration, exists := service.registry.Get(node.Type)
	if !exists {
		return execution.SourceDispatch{}, ErrBindingInvariant
	}
	lifecycles := plugin.NewLifecycles()
	httpInfrastructure := &requestInfrastructure{
		currentIdentity: binding.ID,
		method:          binding.Method,
		payload:         payload,
	}
	infrastructure := plugin.Infrastructure{
		HTTP: httpInfrastructure,
	}
	if service.emitter != nil {
		infrastructure.Emitter = service.emitter.ForContext(ctx, registration.Key)
	}
	access := execution.NewNodeAccessCapture(
		definition, binding.TriggerNodeID, registration.RoutingMode,
	)
	pluginContext := plugin.NewContext(plugin.ContextOptions{
		Runtime:       ctx,
		Configuration: node.Configuration,
		CompanyID:     binding.CompanyID,
		Lifecycles:    lifecycles,
		Storage: pluginstate.NewStorage(ctx, service.state, pluginstate.Scope{
			CompanyID:        binding.CompanyID,
			WorkflowID:       binding.WorkflowID,
			WorkflowRevision: binding.WorkflowRevision,
			NodeID:           binding.TriggerNodeID,
		}),
		Access:         access,
		Infrastructure: infrastructure,
		Payload:        payload,
	})
	if err := registration.Handler(pluginContext); err != nil {
		return execution.SourceDispatch{}, err
	}
	if err := lifecycles.InvokeScenarioStart(); err != nil {
		return execution.SourceDispatch{}, err
	}
	output, err := httpInfrastructure.Dispatch()
	if err != nil {
		return execution.SourceDispatch{}, err
	}
	return execution.SourceDispatch{Output: output, Routing: access.Outcome()}, nil
}

func stableFingerprintPayload(payload map[string]any) map[string]any {
	stable := make(map[string]any, 4)
	if method, present := payload["method"]; present {
		if typed, ok := method.(string); ok {
			stable["method"] = strings.ToUpper(strings.TrimSpace(typed))
		} else {
			stable["method"] = method
		}
	}
	if query, present := payload["query"]; present {
		stable["query"] = fingerprintQuery(query)
	} else {
		stable["query"] = map[string][]string{}
	}
	if headers, present := payload["headers"]; present {
		if typed, ok := headers.(map[string][]string); ok {
			stable["headers"] = fingerprintHeaders(typed)
		} else {
			stable["headers"] = fingerprintHeaders(nil)
		}
	} else {
		stable["headers"] = fingerprintHeaders(nil)
	}
	if body, present := payload["body"]; present {
		stable["body"] = body
	}
	return stable
}

func invocationFingerprint(binding Binding, payload map[string]any) string {
	stable := stableFingerprintPayload(payload)
	stable["triggerId"] = binding.ID
	stable["snapshotId"] = binding.SnapshotID
	stable["method"] = binding.Method
	return execution.Fingerprint(stable)
}

func secureToken() (string, []byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", nil, err
	}
	raw := base64.RawURLEncoding.EncodeToString(value)
	digest := sha256.Sum256([]byte(raw))
	return raw, digest[:], nil
}

func secureID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}
