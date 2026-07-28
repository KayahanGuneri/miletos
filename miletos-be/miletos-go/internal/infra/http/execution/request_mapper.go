package executionhttp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"miletos-go/internal/engine"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

func mapAsyncExecutionRequest(
	body ExecutionRequestBody, companyID workflow.CompanyID, correlationID string,
) (engine.ExecutionRequest, *APIError,
) {
	definition, apiError := mapWorkflowDefinitionRequest(
		body.Definition, companyID)
	if apiError != nil {
		return engine.ExecutionRequest{},
			apiError
	}
	initialVariables, apiError := mapInitialVariables(body.InitialVariables)
	if apiError != nil {
		return engine.ExecutionRequest{}, apiError
	}
	executionID, err := execution.NewWorkflowExecutionID(
		newOpaqueID("exec_"),
	)
	if err != nil {
		return engine.ExecutionRequest{}, newAPIError(500,
			errorCodeInternalServerError, "An unexpected internal error occurred.")
	}
	request, err :=
		engine.NewAsyncExecutionRequest(executionID, definition,
			correlationID, initialVariables)
	if err != nil {
		return engine.ExecutionRequest{},
			invalidExecutionRequest("request", "execution request contains invalid or reserved values")
	}
	return request, nil
}

func fingerprintAsyncExecutionRequest(body ExecutionRequestBody,
) (string, *APIError,
) {
	canonical, err := canonicalJSON(
		asyncExecutionFingerprintEnvelope{Mode: execution.ExecutionModeAsync.
			String(), Request: body,
		})
	if err != nil {
		return "", newAPIError(
			500, errorCodeInternalServerError, "An unexpected internal error occurred.",
		)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(
		digest[:]), nil
}

func canonicalJSON(
	value any) ([]byte,
	error) {
	encoded, err :=
		json.Marshal(value)
	if err != nil {
		return nil,
			err
	}
	decoder := json.NewDecoder(bytes.NewReader(
		encoded))
	decoder.UseNumber()
	var canonical any
	if err :=
		decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	return json.Marshal(
		canonical)
}

func mapSyncExecutionRequest(
	body ExecutionRequestBody, companyID workflow.CompanyID, correlationID string,
) (engine.ExecutionRequest, *APIError,
) {
	definition, apiError := mapWorkflowDefinitionRequest(
		body.Definition, companyID)
	if apiError != nil {
		return engine.ExecutionRequest{},
			apiError
	}
	initialVariables, apiError := mapInitialVariables(body.InitialVariables)
	if apiError != nil {
		return engine.ExecutionRequest{}, apiError
	}
	executionID, err := execution.NewWorkflowExecutionID(
		newOpaqueID("exec_"),
	)
	if err != nil {
		return engine.ExecutionRequest{}, newAPIError(500,
			errorCodeInternalServerError, "An unexpected internal error occurred.")
	}
	request, err :=
		engine.NewExecutionRequest(executionID, definition,
			correlationID, initialVariables)
	if err != nil {
		return engine.ExecutionRequest{},
			invalidExecutionRequest("request", "execution request contains invalid or reserved values")
	}
	return request, nil
}

func mapWorkflowDefinitionRequest(request WorkflowDefinitionRequest,
	companyID workflow.CompanyID) (workflow.WorkflowDefinition,
	*APIError) {
	nodes :=
		make([]workflow.NodeDefinition, 0,
			len(request.Nodes))
	for index, nodeRequest := range request.Nodes {
		node, apiError :=
			mapWorkflowNodeRequest(nodeRequest, index)
		if apiError != nil {
			return workflow.WorkflowDefinition{}, apiError
		}
		nodes = append(nodes,
			node)
	}
	edges := make(
		[]workflow.EdgeDefinition, 0, len(request.Edges),
	)
	for index, edgeRequest := range request.Edges {
		edge, apiError := mapWorkflowEdgeRequest(
			edgeRequest, index)
		if apiError != nil {
			return workflow.WorkflowDefinition{},
				apiError
		}
		edges = append(edges, edge)
	}
	definition, err := workflow.NewWorkflowDefinition(workflow.WorkflowID(
		request.ID),
		companyID, request.Name,
		request.Revision,
		nodes, edges,
		jsonObjectOrDefault(request.Metadata))
	if err != nil {
		return workflow.WorkflowDefinition{}, invalidExecutionRequest(
			"definition", "workflow definition contains invalid identity, revision, metadata, or structure")
	}
	return definition,
		nil
}

func mapWorkflowNodeRequest(request WorkflowNodeRequest, index int,
) (workflow.NodeDefinition, *APIError,
) {
	var position *workflow.NodePosition
	if request.Position != nil {
		value, err := workflow.NewNodePosition(
			request.Position.X, request.Position.Y)
		if err != nil {
			return workflow.NodeDefinition{},
				invalidExecutionRequest(fmt.Sprintf("definition.nodes[%d].position",
					index), "node position must contain finite coordinates",
				)
		}
		position = &value
	}
	node, err := workflow.NewNodeDefinition(
		workflow.NodeID(request.ID),
		workflow.PluginType(request.PluginType), workflow.PluginVersion(
			request.PluginVersion),
		jsonObjectOrDefault(request.Configuration),
		position)
	if err != nil {
		return workflow.NodeDefinition{},
			invalidExecutionRequest(fmt.Sprintf("definition.nodes[%d]",
				index), "node contains invalid identity, plugin identity, or configuration JSON",
			)
	}
	return node, nil
}

func mapWorkflowEdgeRequest(request WorkflowEdgeRequest,
	index int) (workflow.EdgeDefinition,
	*APIError) {
	edge, err :=
		workflow.NewEdgeDefinition(workflow.EdgeID(request.ID), workflow.NodeID(
			request.SourceNodeID),
			request.SourceOutputPort, workflow.NodeID(
				request.TargetNodeID),
			request.TargetInputPort)
	if err != nil {
		return workflow.EdgeDefinition{}, invalidExecutionRequest(
			fmt.Sprintf("definition.edges[%d]", index), "edge contains invalid identity, endpoint, or port information")
	}
	return edge,
		nil
}

func mapInitialVariables(values map[string]json.RawMessage) (
	map[string]runtime.RuntimeValue, *APIError) {
	if len(values) == 0 {
		return map[string]runtime.RuntimeValue{}, nil
	}
	mapped :=
		make(map[string]runtime.RuntimeValue, len(values))
	for key, rawValue := range values {
		value, err := runtime.NewRuntimeValue(
			rawValue)
		if err != nil {
			return nil, invalidExecutionRequest(
				"initialVariables."+key, "initial variable must contain a valid JSON value")
		}
		mapped[key] =
			value
	}
	return mapped, nil
}

func jsonObjectOrDefault(value json.RawMessage,
) []byte {
	if len(bytes.TrimSpace(
		value)) == 0 {
		return []byte(`{}`)
	}
	return bytes.Clone(value)
}

func invalidExecutionRequest(
	field string, reason string) *APIError {
	return newAPIError(400, errorCodeInvalidExecutionRequest,
		"Execution request is invalid.", ErrorDetail{Field: field,
			Reason: reason})
}
