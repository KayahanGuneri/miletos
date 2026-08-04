package com.miletos.features.workflow;

import java.util.ArrayList;
import java.util.List;

import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.ReportingPolicy;
import org.springframework.data.domain.Page;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.JsonNodeFactory;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.miletos.features.workflow.controller.request.CreateWorkflowRequest;
import com.miletos.features.workflow.controller.request.NodePositionRequest;
import com.miletos.features.workflow.controller.request.UpdateWorkflowRequest;
import com.miletos.features.workflow.controller.request.WorkflowEdgeRequest;
import com.miletos.features.workflow.controller.request.WorkflowNodeRequest;
import com.miletos.features.workflow.controller.response.NodePositionResponse;
import com.miletos.features.workflow.controller.response.WorkflowEdgeResponse;
import com.miletos.features.workflow.controller.response.WorkflowNodeResponse;
import com.miletos.features.workflow.controller.response.WorkflowPageResponse;
import com.miletos.features.workflow.controller.response.WorkflowResponse;
import com.miletos.features.workflow.controller.response.WorkflowSummaryResponse;
import com.miletos.features.workflow.repository.entity.Workflow;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.ERROR)
public interface WorkflowMapper {

    @Mapping(target = "id", ignore = true)
    @Mapping(target = "company", ignore = true)
    @Mapping(target = "normalizedName", ignore = true)
    @Mapping(target = "status", ignore = true)
    @Mapping(target = "revision", ignore = true)
    @Mapping(target = "definitionJson", expression = "java(toDefinitionJson(request.nodes(), request.edges(), request.metadata()))")
    @Mapping(target = "createdByEmail", ignore = true)
    @Mapping(target = "updatedByEmail", ignore = true)
    @Mapping(target = "createdAt", ignore = true)
    @Mapping(target = "updatedAt", ignore = true)
    Workflow toEntity(CreateWorkflowRequest request);

    @Mapping(target = "id", ignore = true)
    @Mapping(target = "company", ignore = true)
    @Mapping(target = "normalizedName", ignore = true)
    @Mapping(target = "status", ignore = true)
    @Mapping(target = "revision", ignore = true)
    @Mapping(target = "definitionJson", expression = "java(toDefinitionJson(request.nodes(), request.edges(), request.metadata()))")
    @Mapping(target = "createdByEmail", ignore = true)
    @Mapping(target = "updatedByEmail", ignore = true)
    @Mapping(target = "createdAt", ignore = true)
    @Mapping(target = "updatedAt", ignore = true)
    Workflow toEntity(UpdateWorkflowRequest request);

    default JsonNode toDefinitionJson(
            List<WorkflowNodeRequest> nodes,
            List<WorkflowEdgeRequest> edges,
            JsonNode metadata) {
        ObjectNode definition = JsonNodeFactory.instance.objectNode();
        definition.set("nodes", toNodeArray(nodes));
        definition.set("edges", toEdgeArray(edges));
        definition.set("metadata", copyOrNull(metadata));
        return definition;
    }

    @Mapping(target = "nodes", expression = "java(nodes(workflow))")
    @Mapping(target = "edges", expression = "java(edges(workflow))")
    @Mapping(target = "metadata", expression = "java(metadata(workflow))")
    @Mapping(target = "nodeCount", expression = "java(nodeCount(workflow))")
    @Mapping(target = "edgeCount", expression = "java(edgeCount(workflow))")
    WorkflowResponse toResponse(Workflow workflow);

    @Mapping(target = "nodeCount", expression = "java(nodeCount(workflow))")
    @Mapping(target = "edgeCount", expression = "java(edgeCount(workflow))")
    WorkflowSummaryResponse toSummaryResponse(Workflow workflow);

    default WorkflowPageResponse toPageResponse(Page<Workflow> page) {
        return new WorkflowPageResponse(
                page.getContent().stream().map(this::toSummaryResponse).toList(),
                page.getNumber(),
                page.getSize(),
                page.getTotalElements(),
                page.getTotalPages(),
                page.isFirst(),
                page.isLast());
    }

    default List<WorkflowNodeResponse> nodes(Workflow workflow) {
        JsonNode nodes = arrayField(workflow, "nodes");
        List<WorkflowNodeResponse> responses = new ArrayList<>();

        for (JsonNode node : nodes) {
            JsonNode configuration = node.path("configuration");
            JsonNode position = node.path("position");
            responses.add(
                    new WorkflowNodeResponse(
                            node.path("nodeId").asText(),
                            node.path("pluginType").asText(),
                            node.path("pluginVersion").asText(),
                            configuration.isObject()
                                    ? configuration.deepCopy()
                                    : JsonNodeFactory.instance.objectNode(),
                            new NodePositionResponse(
                                    position.path("x").asDouble(), position.path("y").asDouble())));
        }

        return List.copyOf(responses);
    }

    default List<WorkflowEdgeResponse> edges(Workflow workflow) {
        JsonNode edges = arrayField(workflow, "edges");
        List<WorkflowEdgeResponse> responses = new ArrayList<>();

        for (JsonNode edge : edges) {
            responses.add(
                    new WorkflowEdgeResponse(
                            edge.path("edgeId").asText(),
                            edge.path("sourceNodeId").asText(),
                            edge.path("sourceOutputPort").asText(),
                            edge.path("targetNodeId").asText(),
                            edge.path("targetInputPort").asText()));
        }

        return List.copyOf(responses);
    }

    default JsonNode metadata(Workflow workflow) {
        JsonNode definition = workflow.getDefinitionJson();
        if (definition == null || !definition.isObject()) {
            return JsonNodeFactory.instance.objectNode();
        }

        JsonNode metadata = definition.path("metadata");
        return metadata.isObject() ? metadata.deepCopy() : JsonNodeFactory.instance.objectNode();
    }

    default int nodeCount(Workflow workflow) {
        return arrayField(workflow, "nodes").size();
    }

    default int edgeCount(Workflow workflow) {
        return arrayField(workflow, "edges").size();
    }

    private static JsonNode arrayField(Workflow workflow, String fieldName) {
        JsonNode definition = workflow.getDefinitionJson();
        if (definition == null || !definition.isObject()) {
            return JsonNodeFactory.instance.arrayNode();
        }

        JsonNode value = definition.path(fieldName);
        return value.isArray() ? value.deepCopy() : JsonNodeFactory.instance.arrayNode();
    }

    private static JsonNode toNodeArray(List<WorkflowNodeRequest> nodes) {
        if (nodes == null) {
            return JsonNodeFactory.instance.nullNode();
        }

        ArrayNode nodeArray = JsonNodeFactory.instance.arrayNode();
        for (WorkflowNodeRequest node : nodes) {
            if (node == null) {
                nodeArray.addNull();
                continue;
            }

            ObjectNode nodeObject = JsonNodeFactory.instance.objectNode();
            nodeObject.put("nodeId", node.nodeId());
            nodeObject.put("pluginType", node.pluginType());
            nodeObject.put("pluginVersion", node.pluginVersion());
            nodeObject.set("configuration", copyOrNull(node.configuration()));

            NodePositionRequest position = node.position();
            if (position == null) {
                nodeObject.set("position", JsonNodeFactory.instance.nullNode());
            } else {
                ObjectNode positionObject = JsonNodeFactory.instance.objectNode();
                positionObject.put("x", position.x());
                positionObject.put("y", position.y());
                nodeObject.set("position", positionObject);
            }
            nodeArray.add(nodeObject);
        }
        return nodeArray;
    }

    private static JsonNode toEdgeArray(List<WorkflowEdgeRequest> edges) {
        if (edges == null) {
            return JsonNodeFactory.instance.nullNode();
        }

        ArrayNode edgeArray = JsonNodeFactory.instance.arrayNode();
        for (WorkflowEdgeRequest edge : edges) {
            if (edge == null) {
                edgeArray.addNull();
                continue;
            }

            ObjectNode edgeObject = JsonNodeFactory.instance.objectNode();
            edgeObject.put("edgeId", edge.edgeId());
            edgeObject.put("sourceNodeId", edge.sourceNodeId());
            edgeObject.put("sourceOutputPort", edge.sourceOutputPort());
            edgeObject.put("targetNodeId", edge.targetNodeId());
            edgeObject.put("targetInputPort", edge.targetInputPort());
            edgeArray.add(edgeObject);
        }
        return edgeArray;
    }

    private static JsonNode copyOrNull(JsonNode value) {
        return value == null ? JsonNodeFactory.instance.nullNode() : value.deepCopy();
    }
}
