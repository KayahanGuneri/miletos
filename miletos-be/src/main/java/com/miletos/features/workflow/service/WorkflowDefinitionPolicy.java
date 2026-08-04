package com.miletos.features.workflow.service;

import java.util.HashSet;
import java.util.Set;

import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.miletos.features.workflow.exception.InvalidWorkflowDefinitionException;

import lombok.RequiredArgsConstructor;

@Component
@RequiredArgsConstructor
public class WorkflowDefinitionPolicy {

    private final ObjectMapper objectMapper;

    public JsonNode validateAndNormalizeDefinition(JsonNode definitionJson) {
        if (definitionJson == null || !definitionJson.isObject()) {
            throw new InvalidWorkflowDefinitionException();
        }

        JsonNode nodes = definitionJson.get("nodes");
        JsonNode edges = definitionJson.get("edges");
        JsonNode metadata = definitionJson.get("metadata");
        if (nodes == null
                || !nodes.isArray()
                || edges == null
                || !edges.isArray()
                || metadata == null
                || !metadata.isObject()) {
            throw new InvalidWorkflowDefinitionException();
        }

        Set<String> nodeIds = new HashSet<>();
        Set<String> edgeIds = new HashSet<>();

        for (JsonNode node : nodes) {
            validateNode(node, nodeIds);
        }

        for (JsonNode edge : edges) {
            validateEdge(edge, edgeIds);
        }

        ObjectNode definition = objectMapper.createObjectNode();
        definition.set("nodes", nodes.deepCopy());
        definition.set("edges", edges.deepCopy());
        definition.set("metadata", metadata.deepCopy());
        return definition;
    }

    private void validateNode(JsonNode node, Set<String> nodeIds) {
        JsonNode position = node == null ? null : node.get("position");
        JsonNode x = position == null ? null : position.get("x");
        JsonNode y = position == null ? null : position.get("y");
        String nodeId = textValue(node, "nodeId");
        if (node == null
                || !node.isObject()
                || nodeId == null
                || textValue(node, "pluginType") == null
                || textValue(node, "pluginVersion") == null
                || node.get("configuration") == null
                || !node.get("configuration").isObject()
                || position == null
                || !position.isObject()
                || x == null
                || !x.isNumber()
                || !Double.isFinite(x.asDouble())
                || y == null
                || !y.isNumber()
                || !Double.isFinite(y.asDouble())
                || !nodeIds.add(nodeId)) {
            throw new InvalidWorkflowDefinitionException();
        }
    }

    private void validateEdge(JsonNode edge, Set<String> edgeIds) {
        String edgeId = textValue(edge, "edgeId");
        if (edge == null
                || !edge.isObject()
                || edgeId == null
                || textValue(edge, "sourceNodeId") == null
                || textValue(edge, "sourceOutputPort") == null
                || textValue(edge, "targetNodeId") == null
                || textValue(edge, "targetInputPort") == null
                || !edgeIds.add(edgeId)) {
            throw new InvalidWorkflowDefinitionException();
        }
    }

    private String textValue(JsonNode object, String fieldName) {
        if (object == null || !object.isObject()) {
            return null;
        }
        JsonNode value = object.get(fieldName);
        return value != null && value.isTextual() ? value.textValue() : null;
    }
}
