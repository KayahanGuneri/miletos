package httptrigger

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution"
	executionmodel "miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
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
}

type CreateRequest struct {
	CompanyID     string
	Workflow      workflow.Workflow
	TriggerNodeID string
	Method        string
	ResolvedMode  ResolvedMode
}

func NewService(
	publicBaseURL string,
	triggers *Repository,
	workflows *workflow.WorkflowRepository,
	workflowService *workflow.WorkflowService,
	executions *execution.ExecutionService,
	registry *plugin.NodeRegistry,
) *Service {
	return &Service{
		publicBaseURL:   strings.TrimRight(publicBaseURL, "/"),
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
) (CreatedBinding, error) {
	if service.publicBaseURL == "" {
		return CreatedBinding{}, ErrPublicURLUnavailable
	}
	request.Workflow.CompanyID = request.CompanyID
	if request.ResolvedMode != ModeAsync {
		return CreatedBinding{}, ErrInvalidTrigger
	}
	method, err := normalizeMethod(request.Method)
	if err != nil {
		return CreatedBinding{}, err
	}
	if err := service.workflowService.Validate(request.Workflow); err != nil {
		return CreatedBinding{}, err
	}
	if err := validateTriggerRoot(
		request.Workflow, request.TriggerNodeID, method, service.registry,
	); err != nil {
		return CreatedBinding{}, ErrInvalidTrigger
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
		Method:           method,
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
	method string,
	registry *plugin.NodeRegistry,
) error {
	rootNode, valid := rootNodeByID(definition, triggerNodeID)
	if !valid {
		return ErrInvalidTrigger
	}
	if !registry.DeclaresExecutionSource(
		rootNode.Type,
		rootNode.Version,
		string(executionmodel.ExecutionOriginHTTPWebhook),
	) {
		return ErrInvalidTrigger
	}
	return validateBindingConfiguration(rootNode.Configuration, method)
}

func rootNodeByID(definition workflow.Workflow, triggerNodeID string) (workflow.WorkflowNode, bool) {
	for _, rootNode := range workflow.Roots(definition) {
		if rootNode.ID == triggerNodeID {
			return rootNode, true
		}
	}
	return workflow.WorkflowNode{}, false
}

func validateBindingConfiguration(configuration map[string]any, method string) error {
	configuredMethod, ok := configuration["method"].(string)
	if !ok || strings.ToUpper(strings.TrimSpace(configuredMethod)) != method {
		return ErrInvalidTrigger
	}
	return nil
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
		binding.Method,
		service.registry,
	); err != nil {
		return execution.ExecutionOutcome{}, "", ErrBindingInvariant
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
	outcome, err := service.executions.ExecuteAsyncFromSnapshot(
		ctx, snapshot, binding.TriggerNodeID, payload, correlationID,
		key, invocationFingerprint(binding, payload),
	)
	return outcome, "", err
}

// stableFingerprintPayload keeps only the logical values of a webhook call.
// Request identifiers, correlation identifiers, secrets, and transport/hop-by-hop
// headers are excluded so a retry of the same logical call replays instead of
// conflicting. Accepted non-transport headers (for example Content-Type and
// caller-defined safe custom headers) remain part of request identity.
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

func normalizeMethod(method string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	switch normalized {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return normalized, nil
	default:
		return "", ErrInvalidTrigger
	}
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
