package executionhttp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const HeaderIdempotencyKey = "Idempotency-Key"

func readIdempotencyKey(request *http.Request,
) (string, *APIError,
) {
	if request == nil {
		return "",
			newAPIError(http.StatusBadRequest, errorCodeInvalidIdempotencyKey,
				"Idempotency-Key header is invalid.")
	}
	values := request.
		Header.Values(HeaderIdempotencyKey)
	if len(values) != 1 {
		return "", newAPIError(http.StatusBadRequest,
			errorCodeInvalidIdempotencyKey, "Exactly one Idempotency-Key header is required.")
	}
	value :=
		strings.TrimSpace(values[0])
	if value == "" {
		return "",
			newAPIError(http.StatusBadRequest, errorCodeInvalidIdempotencyKey,
				"Idempotency-Key header must not be blank.")
	}
	if utf8.RuneCountInString(value) > repository.MaximumHTTPIdempotencyKeyCharacters {
		return "", newAPIError(
			http.StatusBadRequest, errorCodeInvalidIdempotencyKey, "Idempotency-Key header is too long.",
		)
	}
	return value, nil
}

func fingerprintPartialRecoveryRequest(
	companyID workflow.CompanyID,
	sourceExecutionID execution.WorkflowExecutionID,
) string {
	digest := sha256.Sum256([]byte(
		"partial-recovery|" + companyID.String() + "|" + sourceExecutionID.String()))
	return hex.EncodeToString(digest[:])
}

type asyncExecutionFingerprintEnvelope struct {
	Mode    string               `json:"mode"`
	Request ExecutionRequestBody `json:"request"`
}

func executionIDFromSubresourcePath(path string,
	subresource string) (execution.WorkflowExecutionID,
	*APIError) {
	rawPath :=
		strings.TrimPrefix(path, executionDetailPathPrefix)
	if rawPath == path {
		return "", newAPIError(http.StatusNotFound,
			errorCodeNotFound, "The requested resource was not found.")
	}
	parts :=
		strings.Split(rawPath, "/")
	if len(parts) != 2 ||
		parts[0] == "" || parts[1] != subresource {
		return "", newAPIError(http.StatusNotFound,
			errorCodeNotFound, "The requested resource was not found.")
	}
	workflowExecutionID, err :=
		execution.NewWorkflowExecutionID(parts[0])
	if err != nil {
		return "",
			newAPIError(http.StatusBadRequest, errorCodeInvalidExecutionID,
				"Execution ID is invalid.")
	}
	return workflowExecutionID, nil
}

type ExecutionRequestBody struct {
	Definition       WorkflowDefinitionRequest  `json:"definition"`
	InitialVariables map[string]json.RawMessage `json:"initialVariables,omitempty"`
}

type WorkflowDefinitionRequest struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Revision uint64                `json:"revision"`
	Nodes    []WorkflowNodeRequest `json:"nodes"`
	Edges    []WorkflowEdgeRequest `json:"edges"`
	Metadata json.RawMessage       `json:"metadata,omitempty"`
}

type WorkflowNodeRequest struct {
	ID            string                       `json:"id"`
	PluginType    string                       `json:"pluginType"`
	PluginVersion string                       `json:"pluginVersion"`
	Configuration json.RawMessage              `json:"configuration,omitempty"`
	Position      *WorkflowNodePositionRequest `json:"position,omitempty"`
}

type WorkflowNodePositionRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WorkflowEdgeRequest struct {
	ID               string `json:"id"`
	SourceNodeID     string `json:"sourceNodeId"`
	SourceOutputPort string `json:"sourceOutputPort"`
	TargetNodeID     string `json:"targetNodeId"`
	TargetInputPort  string `json:"targetInputPort"`
}

func parseExecutionErrorsQuery(request *http.Request) (repository.PageRequest, *APIError) {
	return parseExecutionPageQuery(request, "errors")
}

func parseExecutionPageQuery(request *http.Request, resource string) (repository.PageRequest, *APIError) {
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return repository.PageRequest{}, newAPIError(
			http.StatusBadRequest, errorCodeInvalidExecutionQuery, fmt.Sprintf("Execution %s query parameters are invalid.", resource),
		)
	}
	for key, entries := range values {
		if key != "limit" && key != "after" {
			return repository.PageRequest{}, newAPIError(
				http.StatusBadRequest, errorCodeInvalidExecutionQuery, fmt.Sprintf("Execution %s query contains an unsupported parameter.", resource),
			)
		}
		if len(entries) != 1 {
			return repository.PageRequest{}, newAPIError(http.StatusBadRequest,
				errorCodeInvalidExecutionQuery, fmt.Sprintf("Execution %s query parameters must not be repeated.", resource))
		}
	}
	limit := 0
	if rawLimit, exists := queryValue(values, "limit"); exists {
		parsedLimit, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsedLimit <= 0 {
			return repository.PageRequest{}, newAPIError(http.StatusBadRequest,
				errorCodeInvalidExecutionQuery, fmt.Sprintf("Execution %s query limit is invalid.", resource))
		}
		limit = parsedLimit
	}
	after := ""
	if rawAfter, exists := queryValue(values, "after"); exists {
		if strings.TrimSpace(rawAfter) == "" {
			return repository.PageRequest{}, newAPIError(http.StatusBadRequest,
				errorCodeInvalidExecutionQuery, fmt.Sprintf("Execution %s cursor must not be blank.", resource))
		}
		after = rawAfter
	}
	pageRequest, err := repository.NewPageRequest(limit, repository.PageToken(after))
	if err != nil {
		return repository.PageRequest{}, newAPIError(http.StatusBadRequest, errorCodeInvalidExecutionQuery,
			fmt.Sprintf("Execution %s pagination parameters are invalid.", resource))
	}
	return pageRequest, nil
}

func parseExecutionEventsQuery(request *http.Request) (repository.PageRequest, *APIError) {
	return parseExecutionPageQuery(request, "events")
}

func parseExecutionLogsQuery(request *http.Request) (repository.PageRequest, *APIError) {
	return parseExecutionPageQuery(request, "logs")
}

func parseExecutionNodesQuery(request *http.Request) (repository.PageRequest, *APIError) {
	return parseExecutionPageQuery(request, "nodes")
}

const executionDetailPathPrefix = "/api/v1/executions/"

func parseExecutionListQuery(request *http.Request) (
	repository.WorkflowExecutionFilter, repository.PageRequest, *APIError,
) {
	values, err := url.ParseQuery(
		request.URL.RawQuery)
	if err != nil {
		return repository.WorkflowExecutionFilter{}, repository.PageRequest{},
			newAPIError(http.StatusBadRequest, errorCodeInvalidExecutionQuery,
				"Execution query parameters are invalid.")
	}
	allowed := map[string]struct{}{
		"limit": {}, "after": {}, "workflowId": {},
		"status": {}}
	for key, entries := range values {
		if _, exists :=
			allowed[key]; !exists {
			return repository.WorkflowExecutionFilter{},
				repository.PageRequest{}, newAPIError(http.StatusBadRequest,
					errorCodeInvalidExecutionQuery, "Execution query contains an unsupported parameter.")
		}
		if len(entries) != 1 {
			return repository.WorkflowExecutionFilter{}, repository.PageRequest{}, newAPIError(
				http.StatusBadRequest, errorCodeInvalidExecutionQuery, "Execution query parameters must not be repeated.",
			)
		}
	}
	limit := 0
	if rawLimit, exists := queryValue(values,
		"limit"); exists {
		parsedLimit, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil ||
			parsedLimit <= 0 {
			return repository.WorkflowExecutionFilter{},
				repository.PageRequest{}, newAPIError(http.StatusBadRequest,
					errorCodeInvalidExecutionQuery, "Execution query limit is invalid.")
		}
		limit =
			parsedLimit
	}
	after := ""
	if rawAfter, exists :=
		queryValue(values, "after"); exists {
		if strings.TrimSpace(
			rawAfter) == "" {
			return repository.WorkflowExecutionFilter{}, repository.PageRequest{}, newAPIError(
				http.StatusBadRequest, errorCodeInvalidExecutionQuery, "Execution query cursor must not be blank.",
			)
		}
		after = rawAfter
	}
	pageRequest, err := repository.NewPageRequest(
		limit, repository.PageToken(after))
	if err != nil {
		return repository.WorkflowExecutionFilter{}, repository.PageRequest{},
			newAPIError(http.StatusBadRequest, errorCodeInvalidExecutionQuery,
				"Execution pagination parameters are invalid.")
	}
	var workflowID workflow.WorkflowID
	if rawWorkflowID, exists := queryValue(values,
		"workflowId"); exists {
		if strings.TrimSpace(rawWorkflowID) == "" {
			return repository.WorkflowExecutionFilter{}, repository.PageRequest{},
				newAPIError(http.StatusBadRequest, errorCodeInvalidExecutionQuery,
					"Workflow ID filter must not be blank.")
		}
		workflowID = workflow.WorkflowID(
			rawWorkflowID)
	}
	var status execution.WorkflowExecutionStatus
	if rawStatus, exists := queryValue(values,
		"status"); exists {
		if strings.TrimSpace(rawStatus) == "" {
			return repository.WorkflowExecutionFilter{}, repository.PageRequest{},
				newAPIError(http.StatusBadRequest, errorCodeInvalidExecutionQuery,
					"Execution status filter must not be blank.")
		}
		status = execution.WorkflowExecutionStatus(
			rawStatus)
	}
	filter, err := repository.NewWorkflowExecutionFilter(
		workflowID, status)
	if err != nil {
		return repository.WorkflowExecutionFilter{},
			repository.PageRequest{}, newAPIError(http.StatusBadRequest,
				errorCodeInvalidExecutionQuery, "Execution filters are invalid.")
	}
	return filter,
		pageRequest, nil
}

func queryValue(values url.Values,
	key string) (string,
	bool) {
	entries, exists :=
		values[key]
	if !exists ||
		len(entries) != 1 {
		return "",
			false
	}
	return entries[0], true
}

func executionIDFromRequestPath(path string,
) (execution.WorkflowExecutionID, *APIError,
) {
	rawID := strings.TrimPrefix(
		path, executionDetailPathPrefix)
	if rawID == path || rawID == "" ||
		strings.Contains(rawID, "/") {
		return "",
			newAPIError(http.StatusNotFound, errorCodeNotFound,
				"The requested resource was not found.")
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(
		rawID)
	if err != nil {
		return "", newAPIError(
			http.StatusBadRequest, errorCodeInvalidExecutionID, "Execution ID is invalid.",
		)
	}
	return workflowExecutionID, nil
}

var fallbackIDCounter atomic.Uint64

// New creates an opaque identifier using the supplied prefix.
//
// The primary path uses 16 cryptographically random bytes.
// The timestamp/counter path is retained as a fallback when
// the operating-system random source is unavailable.
func newOpaqueID(
	prefix string) string {
	randomBytes :=
		make([]byte, 16)
	if _, err :=
		rand.Read(randomBytes); err == nil {
		return prefix + hex.EncodeToString(
			randomBytes)
	}
	return fmt.Sprintf("%s%x%x",
		prefix, time.Now().UTC().
			UnixNano(), fallbackIDCounter.Add(1))
}

const (
	executionDefinitionSubresource = "definition"
	executionNodesSubresource      = "nodes"
	executionEventsSubresource     = "events"
	executionLogsSubresource       = "logs"
	executionErrorsSubresource     = "errors"
	executionRecoverySubresource   = "recover"
)
