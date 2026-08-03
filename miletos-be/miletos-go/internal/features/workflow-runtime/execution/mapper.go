package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/requestcontext"
)

type executionRequest struct {
	Definition       workflowRequest `json:"definition"`
	InitialVariables map[string]any  `json:"initialVariables,omitempty"`
}

type ExecutionCommand struct {
	CorrelationID  string
	IdempotencyKey string
	Definition     workflow.Workflow
	StartInput     map[string]any
	Fingerprint    string
}

type workflowRequest struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Revision uint64         `json:"revision"`
	Nodes    []nodeRequest  `json:"nodes"`
	Edges    []edgeRequest  `json:"edges"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type nodeRequest struct {
	ID            string                 `json:"id"`
	PluginType    string                 `json:"pluginType"`
	PluginVersion string                 `json:"pluginVersion"`
	Configuration map[string]any         `json:"configuration,omitempty"`
	Position      *workflow.NodePosition `json:"position,omitempty"`
}

type edgeRequest struct {
	ID               string `json:"id"`
	SourceNodeID     string `json:"sourceNodeId"`
	SourceOutputPort string `json:"sourceOutputPort"`
	TargetNodeID     string `json:"targetNodeId"`
	TargetInputPort  string `json:"targetInputPort"`
}

func mapExecutionCommand(
	request executionRequest,
	companyID string,
	correlationID string,
	idempotencyKey string,
	mode string,
) ExecutionCommand {
	return ExecutionCommand{
		Definition:     mapWorkflowRequest(request.Definition, companyID),
		StartInput:     request.InitialVariables,
		CorrelationID:  correlationID,
		IdempotencyKey: idempotencyKey,
		Fingerprint:    fingerprint(map[string]any{"request": request, "mode": mode}),
	}
}

func recoveryFingerprint(companyID string, executionID string) string {
	return fingerprint(map[string]string{
		"companyId": companyID, "executionId": executionID,
	})
}

type executionResponse struct {
	ExecutionID      string                `json:"executionId"`
	WorkflowID       string                `json:"workflowId"`
	WorkflowRevision uint64                `json:"workflowRevision"`
	Mode             string                `json:"mode"`
	Status           model.ExecutionStatus `json:"status"`
	CorrelationID    string                `json:"correlationId"`
	CreatedAt        string                `json:"createdAt"`
	StartedAt        *string               `json:"startedAt,omitempty"`
	FinishedAt       *string               `json:"finishedAt,omitempty"`
	ScheduledRoots   int                   `json:"scheduledRoots,omitempty"`
	Replayed         bool                  `json:"replayed"`
}

type executionSummaryResponse struct {
	ExecutionID      string                `json:"executionId"`
	WorkflowID       string                `json:"workflowId"`
	WorkflowRevision uint64                `json:"workflowRevision"`
	Mode             string                `json:"mode"`
	Status           model.ExecutionStatus `json:"status"`
	CorrelationID    string                `json:"correlationId"`
	CreatedAt        string                `json:"createdAt"`
	ValidatingAt     *string               `json:"validatingAt,omitempty"`
	QueuedAt         *string               `json:"queuedAt,omitempty"`
	StartedAt        *string               `json:"startedAt,omitempty"`
	FinishedAt       *string               `json:"finishedAt,omitempty"`
	UpdatedAt        string                `json:"updatedAt"`
	IsStalled        bool                  `json:"isStalled"`
}

type executionPageResponse struct {
	Items   []executionSummaryResponse `json:"items"`
	Next    string                     `json:"next"`
	HasNext bool                       `json:"hasNext"`
}

type executionDefinitionResponse struct {
	ExecutionID      string            `json:"executionId"`
	SnapshotID       string            `json:"snapshotId"`
	WorkflowID       string            `json:"workflowId"`
	WorkflowRevision uint64            `json:"workflowRevision"`
	WorkflowName     string            `json:"workflowName"`
	Definition       workflow.Workflow `json:"definition"`
	CreatedAt        string            `json:"createdAt"`
}

type recoveryResponse struct {
	SourceExecutionID   string                `json:"sourceExecutionId"`
	RecoveryExecutionID string                `json:"recoveryExecutionId"`
	Status              model.ExecutionStatus `json:"status"`
	PreservedNodeCount  int                   `json:"preservedNodeCount"`
	ScheduledNodeCount  int                   `json:"scheduledNodeCount"`
	ResetNodeCount      int                   `json:"resetNodeCount"`
	CreatedAt           string                `json:"createdAt"`
	Replayed            bool                  `json:"replayed"`
}

type apiErrorResponse struct {
	Timestamp     string           `json:"timestamp"`
	Status        int              `json:"status"`
	Code          string           `json:"code"`
	Message       string           `json:"message"`
	RequestID     string           `json:"requestId,omitempty"`
	CorrelationID string           `json:"correlationId,omitempty"`
	Details       []apiErrorDetail `json:"details,omitempty"`
}

type apiErrorDetail struct {
	Code          string   `json:"code,omitempty"`
	Field         string   `json:"field,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	NodeID        string   `json:"nodeId,omitempty"`
	EdgeID        string   `json:"edgeId,omitempty"`
	PluginType    string   `json:"pluginType,omitempty"`
	PluginVersion string   `json:"pluginVersion,omitempty"`
	Expected      string   `json:"expected,omitempty"`
	Actual        string   `json:"actual,omitempty"`
	CyclePath     []string `json:"cyclePath,omitempty"`
}

func mapWorkflowRequest(request workflowRequest, companyID string) workflow.Workflow {
	definition := workflow.Workflow{
		ID: request.ID, CompanyID: companyID, Name: request.Name,
		Revision: request.Revision, Metadata: request.Metadata,
		Nodes: make([]workflow.WorkflowNode, 0, len(request.Nodes)),
		Edges: make([]workflow.Edge, 0, len(request.Edges)),
	}
	for _, node := range request.Nodes {
		definition.Nodes = append(definition.Nodes, workflow.WorkflowNode{
			ID: node.ID, Type: node.PluginType, Version: node.PluginVersion,
			Configuration: node.Configuration, Position: node.Position,
		})
	}
	for _, edge := range request.Edges {
		definition.Edges = append(definition.Edges, workflow.Edge{
			ID: edge.ID, SourceNodeID: edge.SourceNodeID,
			SourceOutputPort: edge.SourceOutputPort,
			TargetNodeID:     edge.TargetNodeID, TargetInputPort: edge.TargetInputPort,
		})
	}
	return definition
}

func mapExecutionOutcome(outcome ExecutionOutcome) executionResponse {
	execution := outcome.Execution
	return executionResponse{
		ExecutionID: execution.ID, WorkflowID: execution.WorkflowID,
		WorkflowRevision: execution.WorkflowRevision, Mode: execution.Mode,
		Status: execution.Status, CorrelationID: execution.CorrelationID,
		CreatedAt: execution.CreatedAt.UTC().Format(time.RFC3339Nano),
		StartedAt: formatTime(execution.StartedAt), FinishedAt: formatTime(execution.FinishedAt),
		ScheduledRoots: outcome.ScheduledEntryNodes,
		Replayed:       outcome.Replayed,
	}
}

func mapExecutionSummary(execution model.Execution) executionSummaryResponse {
	return executionSummaryResponse{
		ExecutionID: execution.ID, WorkflowID: execution.WorkflowID,
		WorkflowRevision: execution.WorkflowRevision, Mode: execution.Mode,
		Status: execution.Status, CorrelationID: execution.CorrelationID,
		CreatedAt:    execution.CreatedAt.UTC().Format(time.RFC3339Nano),
		ValidatingAt: formatTime(execution.ValidatingAt), QueuedAt: formatTime(execution.QueuedAt),
		StartedAt: formatTime(execution.StartedAt), FinishedAt: formatTime(execution.FinishedAt),
		UpdatedAt: execution.UpdatedAt.UTC().Format(time.RFC3339Nano),
		IsStalled: execution.IsStalled,
	}
}

func mapExecutionPage(page model.Page[model.Execution]) executionPageResponse {
	items := make([]executionSummaryResponse, 0, len(page.Items))
	for _, execution := range page.Items {
		items = append(items, mapExecutionSummary(execution))
	}
	return executionPageResponse{Items: items, Next: page.Next, HasNext: page.HasNext}
}

func mapDefinition(
	executionID string,
	snapshot workflow.WorkflowSnapshot,
) executionDefinitionResponse {
	return executionDefinitionResponse{
		ExecutionID: executionID, SnapshotID: snapshot.ID,
		WorkflowID: snapshot.Workflow.ID, WorkflowRevision: snapshot.Workflow.Revision,
		WorkflowName: snapshot.Workflow.Name, Definition: snapshot.Workflow,
		CreatedAt: snapshot.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapRecoveryOutcome(outcome RecoveryOutcome) recoveryResponse {
	return recoveryResponse{
		SourceExecutionID:   outcome.SourceExecutionID,
		RecoveryExecutionID: outcome.RecoveryExecutionID,
		Status:              outcome.Status, PreservedNodeCount: outcome.PreservedNodeCount,
		ScheduledNodeCount: outcome.ScheduledNodeCount, ResetNodeCount: outcome.ResetNodeCount,
		CreatedAt: outcome.CreatedAt.UTC().Format(time.RFC3339Nano), Replayed: outcome.Replayed,
	}
}

func formatTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func fingerprint(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func writeExecutionError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		WriteAPIError(writer, request, http.StatusNotFound, "NOT_FOUND",
			"The requested execution was not found.")
	case errors.Is(err, repository.ErrIdempotencyConflict):
		WriteAPIError(writer, request, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED",
			"Idempotency-Key was already used for a different request.")
	case errors.Is(err, context.DeadlineExceeded):
		WriteAPIError(writer, request, http.StatusGatewayTimeout, "REQUEST_TIMEOUT",
			"Workflow execution exceeded the allowed request deadline.")
	case errors.Is(err, ErrAsyncUnavailable):
		WriteAPIError(writer, request, http.StatusServiceUnavailable, "EXECUTION_UNAVAILABLE",
			"Requested execution capability is currently unavailable.")
	case errors.Is(err, ErrStartInputNotAccepted):
		WriteAPIErrorDetails(
			writer,
			request,
			http.StatusUnprocessableEntity,
			"WORKFLOW_VALIDATION_FAILED",
			"Workflow definition failed validation.",
			[]apiErrorDetail{{
				Code:   "INITIAL_VARIABLES_NOT_ACCEPTED",
				Field:  "initialVariables",
				Reason: "No workflow entry node accepts the supplied initial variables.",
			}},
		)
	case errors.Is(err, ErrInvalidExecutionOrigin):
		WriteAPIError(writer, request, http.StatusUnprocessableEntity,
			"INVALID_EXECUTION_ORIGIN",
			"The workflow is incompatible with this execution origin.")
	case errors.Is(err, ErrRecoveryUnsupported):
		WriteAPIError(writer, request, http.StatusConflict, "RECOVERY_NOT_SUPPORTED",
			"The source execution is not eligible for partial recovery.")
	case errors.Is(err, repository.ErrRecoveryConflict):
		WriteAPIError(writer, request, http.StatusConflict, "RECOVERY_ALREADY_EXISTS",
			"A recovery already exists for the source execution.")
	case errors.Is(err, ErrSyncNonterminal):
		WriteAPIError(writer, request, http.StatusInternalServerError, "EXECUTION_NOT_TERMINAL",
			"Synchronous execution did not reach a terminal state.")
	case errors.Is(err, repository.ErrStateTransition):
		WriteAPIError(writer, request, http.StatusInternalServerError, "INVALID_EXECUTION_STATE",
			"Workflow execution reached an invalid internal state.")
	case errors.Is(err, workflow.ErrInvalidWorkflow):
		var validationError *workflow.WorkflowValidationError
		details := make([]apiErrorDetail, 0)
		if errors.As(err, &validationError) {
			for _, issue := range validationError.Issues {
				details = append(details, apiErrorDetail{
					Code: issue.Code, Field: issue.Field, Reason: issue.Reason,
					NodeID: issue.NodeID, EdgeID: issue.EdgeID,
					PluginType: issue.PluginType, PluginVersion: issue.PluginVersion,
					Expected: issue.Expected, Actual: issue.Actual, CyclePath: issue.CyclePath,
				})
			}
		}
		WriteAPIErrorDetails(
			writer, request, http.StatusUnprocessableEntity,
			"WORKFLOW_VALIDATION_FAILED", "Workflow definition failed validation.", details,
		)
	default:
		WriteAPIError(writer, request, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected internal error occurred.")
	}
}

func WriteAPIErrorDetails(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	message string,
	details []apiErrorDetail,
) {
	writeJSON(writer, status, apiErrorResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Status:    status, Code: code, Message: message,
		RequestID:     requestcontext.RequestID(request.Context()),
		CorrelationID: requestcontext.CorrelationID(request.Context()),
		Details:       details,
	})
}

func WriteAPIError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	message string,
) {
	writeJSON(writer, status, apiErrorResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Status:    status, Code: code, Message: message,
		RequestID:     requestcontext.RequestID(request.Context()),
		CorrelationID: requestcontext.CorrelationID(request.Context()),
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
