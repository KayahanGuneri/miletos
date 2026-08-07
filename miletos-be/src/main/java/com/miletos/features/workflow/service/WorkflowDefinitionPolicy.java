package com.miletos.features.workflow.service;

import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import java.util.function.Predicate;

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
        JsonNode nodes = definitionJson.get("nodes");
        JsonNode edges = definitionJson.get("edges");
        JsonNode metadata = definitionJson.get("metadata");
        Set<String> nodeIds = new HashSet<>();
        Set<String> edgeIds = new HashSet<>();

        LinkedHashMap<String, Predicate<JsonNode>> definitionValidators = new LinkedHashMap<>();
        definitionValidators.put(
                "metadata must be a JSON object",
                ignored -> metadata.isObject());
        validateAll(definitionValidators, definitionJson);

        for (JsonNode node : nodes) {
            validateAll(nodeValidators(nodeIds), node);
        }

        for (JsonNode edge : edges) {
            validateAll(edgeValidators(edgeIds, nodeIds), edge);
        }

        ObjectNode definition = objectMapper.createObjectNode();
        definition.set("nodes", nodes.deepCopy());
        definition.set("edges", edges.deepCopy());
        definition.set("metadata", metadata.deepCopy());
        return definition;
    }

    private LinkedHashMap<String, Predicate<JsonNode>> nodeValidators(Set<String> nodeIds) {
        LinkedHashMap<String, Predicate<JsonNode>> validators = new LinkedHashMap<>();

        validators.put(
                "configuration must be a JSON object",
                node -> node.get("configuration").isObject());
        validators.put(
                "position x must be finite",
                node -> isFiniteNumber(node.get("position"), "x"));
        validators.put(
                "position y must be finite",
                node -> isFiniteNumber(node.get("position"), "y"));
        validators.put(
                "nodeId must be unique",
                node -> nodeIds.add(textValue(node, "nodeId")));

        return validators;
    }

    private LinkedHashMap<String, Predicate<JsonNode>> edgeValidators(
            Set<String> edgeIds,
            Set<String> nodeIds) {
        LinkedHashMap<String, Predicate<JsonNode>> validators = new LinkedHashMap<>();

        validators.put(
                "edgeId must be unique",
                edge -> edgeIds.add(textValue(edge, "edgeId")));
        validators.put(
                "source node must exist",
                edge -> nodeIds.contains(textValue(edge, "sourceNodeId")));
        validators.put(
                "target node must exist",
                edge -> nodeIds.contains(textValue(edge, "targetNodeId")));

        return validators;
    }

    private void validateAll(
            Map<String, Predicate<JsonNode>> validators,
            JsonNode candidate) {
        validators.forEach((validationName, validator) -> {
            boolean valid = validator.test(candidate);

            if (!valid) {
                throw new InvalidWorkflowDefinitionException();
            }
        });
    }

    private boolean isFiniteNumber(JsonNode object, String fieldName) {
        if (object == null || !object.isObject()) {
            return false;
        }

        JsonNode value = object.get(fieldName);
        return value != null && value.isNumber() && Double.isFinite(value.asDouble());
    }

    private String textValue(JsonNode object, String fieldName) {
        if (object == null || !object.isObject()) {
            return null;
        }

        JsonNode value = object.get(fieldName);
        return value != null && value.isTextual() ? value.textValue() : null;
    }
}
