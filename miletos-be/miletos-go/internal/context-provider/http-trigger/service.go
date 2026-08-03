package httptrigger

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution"
	executionmodel "miletos-go/internal/features/workflow-runtime/execution/model"
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
	root, valid := singleRoot(definition)
	if !valid {
		return ErrInvalidTrigger
	}
	if root.ID != triggerNodeID {
		return ErrInvalidTrigger
	}
	if !registry.DeclaresExecutionSource(
		root.Type,
		root.Version,
		string(executionmodel.ExecutionOriginHTTPWebhook),
	) {
		return ErrInvalidTrigger
	}
	return validateBindingConfiguration(root.Configuration, method)
}

func singleRoot(definition workflow.Workflow) (workflow.WorkflowNode, bool) {
	roots := workflow.Roots(definition)
	if len(roots) != 1 {
		return workflow.WorkflowNode{}, false
	}
	return roots[0], true
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

func (service *Service) Disable(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	binding, err := service.triggers.Disable(ctx, companyID, triggerID)
	binding.TokenHash = nil
	return binding, err
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
	fingerprintPayload := map[string]any{
		"triggerId":  binding.ID,
		"snapshotId": binding.SnapshotID,
		"payload":    stableFingerprintPayload(payload),
	}
	encoded, _ := json.Marshal(fingerprintPayload)
	fingerprintDigest := sha256.Sum256(encoded)
	outcome, err := service.executions.ExecuteAsyncFromSnapshot(
		ctx, snapshot, binding.TriggerNodeID, payload, correlationID,
		key, hex.EncodeToString(fingerprintDigest[:]),
	)
	return outcome, "", err
}

func stableFingerprintPayload(payload map[string]any) map[string]any {
	stable := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "requestId" || key == "correlationId" {
			continue
		}
		stable[key] = value
	}
	return stable
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
