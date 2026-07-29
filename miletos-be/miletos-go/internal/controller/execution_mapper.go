package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"miletos-go/internal/engine"
	"miletos-go/internal/middleware"
	"miletos-go/internal/model"
	"miletos-go/internal/repository"
)

type executionRequest struct {
	Definition       workflowRequest `json:"definition"`
	InitialVariables map[string]any  `json:"initialVariables,omitempty"`
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
	ID            string              `json:"id"`
	PluginType    string              `json:"pluginType"`
	PluginVersion string              `json:"pluginVersion"`
	Configuration map[string]any      `json:"configuration,omitempty"`
	Position      *model.NodePosition `json:"position,omitempty"`
}

type edgeRequest struct {
	ID               string `json:"id"`
	SourceNodeID     string `json:"sourceNodeId"`
	SourceOutputPort string `json:"sourceOutputPort"`
	TargetNodeID     string `json:"targetNodeId"`
	TargetInputPort  string `json:"targetInputPort"`
}

type executionResponse struct {
	ExecutionID      string  `json:"executionId"`
	WorkflowID       string  `json:"workflowId"`
	WorkflowRevision uint64  `json:"workflowRevision"`
	Mode             string  `json:"mode"`
	Status           string  `json:"status"`
	CorrelationID    string  `json:"correlationId"`
	CreatedAt        string  `json:"createdAt"`
	StartedAt        *string `json:"startedAt,omitempty"`
	FinishedAt       *string `json:"finishedAt,omitempty"`
	ScheduledRoots   int     `json:"scheduledRoots,omitempty"`
}

type executionSummaryResponse struct {
	ExecutionID      string  `json:"executionId"`
	WorkflowID       string  `json:"workflowId"`
	WorkflowRevision uint64  `json:"workflowRevision"`
	Mode             string  `json:"mode"`
	Status           string  `json:"status"`
	CorrelationID    string  `json:"correlationId"`
	CreatedAt        string  `json:"createdAt"`
	ValidatingAt     *string `json:"validatingAt,omitempty"`
	QueuedAt         *string `json:"queuedAt,omitempty"`
	StartedAt        *string `json:"startedAt,omitempty"`
	FinishedAt       *string `json:"finishedAt,omitempty"`
	UpdatedAt        string  `json:"updatedAt"`
	IsStalled        bool    `json:"isStalled"`
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

func mapWorkflowRequest(request workflowRequest, companyID string) model.Workflow {
	workflow := model.Workflow{
		ID: request.ID, CompanyID: companyID, Name: request.Name,
		Revision: request.Revision, Metadata: request.Metadata,
		Nodes: make([]model.Node, 0, len(request.Nodes)),
		Edges: make([]model.Edge, 0, len(request.Edges)),
	}
	for _, node := range request.Nodes {
		workflow.Nodes = append(workflow.Nodes, model.Node{
			ID: node.ID, Type: node.PluginType, Version: node.PluginVersion,
			Configuration: node.Configuration, Position: node.Position,
		})
	}
	for _, edge := range request.Edges {
		workflow.Edges = append(workflow.Edges, model.Edge{
			ID: edge.ID, SourceNodeID: edge.SourceNodeID,
			SourceOutputPort: edge.SourceOutputPort,
			TargetNodeID:     edge.TargetNodeID, TargetInputPort: edge.TargetInputPort,
		})
	}
	return workflow
}

func mapExecutionOutcome(outcome engine.ExecutionOutcome) executionResponse {
	execution := outcome.Execution
	return executionResponse{
		ExecutionID: execution.ID, WorkflowID: execution.WorkflowID,
		WorkflowRevision: execution.WorkflowRevision, Mode: execution.Mode,
		Status: execution.Status, CorrelationID: execution.CorrelationID,
		CreatedAt: execution.CreatedAt.UTC().Format(time.RFC3339Nano),
		StartedAt: formatTime(execution.StartedAt), FinishedAt: formatTime(execution.FinishedAt),
		ScheduledRoots: outcome.ScheduledRoots,
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

func mapExecutionPage(page model.Page[model.Execution]) map[string]any {
	items := make([]executionSummaryResponse, 0, len(page.Items))
	for _, execution := range page.Items {
		items = append(items, mapExecutionSummary(execution))
	}
	return map[string]any{"items": items, "next": page.Next, "hasNext": page.HasNext}
}

func mapDefinition(
	executionID string,
	snapshot model.WorkflowSnapshot,
) map[string]any {
	return map[string]any{
		"executionId":      executionID,
		"snapshotId":       snapshot.ID,
		"workflowId":       snapshot.Workflow.ID,
		"workflowRevision": snapshot.Workflow.Revision,
		"workflowName":     snapshot.Workflow.Name,
		"definition":       snapshot.Workflow,
		"createdAt":        snapshot.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapRecoveryOutcome(outcome engine.RecoveryOutcome) map[string]any {
	return map[string]any{
		"sourceExecutionId":   outcome.SourceExecutionID,
		"recoveryExecutionId": outcome.RecoveryExecutionID,
		"status":              outcome.Status,
		"preservedNodeCount":  outcome.PreservedNodeCount,
		"scheduledNodeCount":  outcome.ScheduledNodeCount,
		"resetNodeCount":      outcome.ResetNodeCount,
		"createdAt":           outcome.CreatedAt.UTC().Format(time.RFC3339Nano),
		"replayed":            outcome.Replayed,
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
	case errors.Is(err, engine.ErrAsyncUnavailable):
		WriteAPIError(writer, request, http.StatusServiceUnavailable, "EXECUTION_UNAVAILABLE",
			"Requested execution capability is currently unavailable.")
	case errors.Is(err, engine.ErrRecoveryUnsupported):
		WriteAPIError(writer, request, http.StatusConflict, "RECOVERY_NOT_SUPPORTED",
			"The source execution is not eligible for partial recovery.")
	case errors.Is(err, repository.ErrRecoveryConflict):
		WriteAPIError(writer, request, http.StatusConflict, "RECOVERY_ALREADY_EXISTS",
			"A recovery already exists for the source execution.")
	case errors.Is(err, engine.ErrSyncNonterminal):
		WriteAPIError(writer, request, http.StatusInternalServerError, "EXECUTION_NOT_TERMINAL",
			"Synchronous execution did not reach a terminal state.")
	case errors.Is(err, repository.ErrStateTransition):
		WriteAPIError(writer, request, http.StatusInternalServerError, "INVALID_EXECUTION_STATE",
			"Workflow execution reached an invalid internal state.")
	case errors.Is(err, engine.ErrInvalidWorkflow):
		var validationError *engine.WorkflowValidationError
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
		Status: status, Code: code, Message: message,
		RequestID: middleware.RequestID(request.Context()),
		CorrelationID: middleware.CorrelationID(request.Context()),
		Details: details,
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
		RequestID:     middleware.RequestID(request.Context()),
		CorrelationID: middleware.CorrelationID(request.Context()),
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
