package com.miletos.features.workflow;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.JsonNodeFactory;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflow.controller.request.CreateWorkflowRequest;
import com.miletos.features.workflow.controller.request.NodePositionRequest;
import com.miletos.features.workflow.controller.request.UpdateWorkflowRequest;
import com.miletos.features.workflow.controller.request.WorkflowEdgeRequest;
import com.miletos.features.workflow.controller.request.WorkflowNodeRequest;
import com.miletos.features.workflow.controller.response.NodePositionResponse;
import com.miletos.features.workflow.controller.response.WorkflowAuditUserResponse;
import com.miletos.features.workflow.controller.response.WorkflowEdgeResponse;
import com.miletos.features.workflow.controller.response.WorkflowNodeResponse;
import com.miletos.features.workflow.controller.response.WorkflowPageResponse;
import com.miletos.features.workflow.controller.response.WorkflowResponse;
import com.miletos.features.workflow.controller.response.WorkflowSummaryResponse;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;
import java.util.ArrayList;
import java.util.List;
import org.mapstruct.BeanMapping;
import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.MappingTarget;
import org.mapstruct.Named;
import org.mapstruct.ReportingPolicy;
import org.springframework.data.domain.Page;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.ERROR)
public interface WorkflowMapper {

  @Mapping(target = "id", ignore = true)
  @Mapping(target = "company", source = "user.company")
  @Mapping(target = "name", source = "request.name", qualifiedByName = "trimRequired")
  @Mapping(target = "description", source = "request.description", qualifiedByName = "trimToNull")
  @Mapping(target = "status", constant = "DRAFT")
  @Mapping(target = "revision", constant = "1L")
  @Mapping(
      target = "definitionJson",
      expression = "java(toDefinitionJson(request.nodes(), request.edges(), request.metadata()))")
  @Mapping(target = "createdBy", source = "user")
  @Mapping(target = "updatedBy", source = "user")
  @Mapping(target = "createdAt", ignore = true)
  @Mapping(target = "updatedAt", ignore = true)
  Workflow toEntity(CreateWorkflowRequest request, User user);

  @Mapping(target = "id", ignore = true)
  @Mapping(target = "company", source = "user.company")
  @Mapping(target = "name", source = "request.name", qualifiedByName = "trimRequired")
  @Mapping(target = "description", source = "request.description", qualifiedByName = "trimToNull")
  @Mapping(target = "status", ignore = true)
  @Mapping(target = "revision", ignore = true)
  @Mapping(
      target = "definitionJson",
      expression = "java(toDefinitionJson(request.nodes(), request.edges(), request.metadata()))")
  @Mapping(target = "createdBy", ignore = true)
  @Mapping(target = "updatedBy", source = "user")
  @Mapping(target = "createdAt", ignore = true)
  @Mapping(target = "updatedAt", ignore = true)
  Workflow toEntity(UpdateWorkflowRequest request, User user);

  @BeanMapping(ignoreByDefault = true)
  @Mapping(target = "definitionJson", source = "definitionJson")
  void applyNormalizedDefinition(JsonNode definitionJson, @MappingTarget Workflow workflow);

  @BeanMapping(ignoreByDefault = true)
  @Mapping(target = "name", source = "changes.name")
  @Mapping(target = "description", source = "changes.description")
  @Mapping(target = "definitionJson", source = "definitionJson")
  @Mapping(target = "revision", source = "revision")
  @Mapping(target = "updatedBy", source = "changes.updatedBy")
  void applyContentUpdate(
      Workflow changes, JsonNode definitionJson, Long revision, @MappingTarget Workflow target);

  @BeanMapping(ignoreByDefault = true)
  @Mapping(target = "status", source = "status")
  @Mapping(target = "updatedBy", source = "user")
  void applyLifecycleUpdate(WorkflowStatus status, User user, @MappingTarget Workflow workflow);

  @Mapping(target = "nodes", source = "workflow", qualifiedByName = "nodes")
  @Mapping(target = "edges", source = "workflow", qualifiedByName = "edges")
  @Mapping(target = "metadata", source = "workflow", qualifiedByName = "metadata")
  @Mapping(target = "nodeCount", source = "workflow", qualifiedByName = "nodeCount")
  @Mapping(target = "edgeCount", source = "workflow", qualifiedByName = "edgeCount")
  WorkflowResponse toResponse(Workflow workflow);

  @Mapping(target = "nodeCount", source = "workflow", qualifiedByName = "nodeCount")
  @Mapping(target = "edgeCount", source = "workflow", qualifiedByName = "edgeCount")
  WorkflowSummaryResponse toSummaryResponse(Workflow workflow);

  List<WorkflowSummaryResponse> toSummaryResponses(List<Workflow> workflows);

  WorkflowAuditUserResponse toAuditUserResponse(User user);

  @Mapping(target = "content", source = "page", qualifiedByName = "pageContent")
  @Mapping(target = "page", source = "number")
  @Mapping(target = "size", source = "size")
  @Mapping(target = "totalElements", source = "totalElements")
  @Mapping(target = "totalPages", source = "totalPages")
  @Mapping(target = "first", source = "first")
  @Mapping(target = "last", source = "last")
  WorkflowPageResponse toPageResponse(Page<Workflow> page);

  @Named("pageContent")
  default List<WorkflowSummaryResponse> pageContent(Page<Workflow> page) {
    return toSummaryResponses(page.getContent());
  }

  @Named("trimRequired")
  default String trimRequired(String value) {
    return value == null ? null : value.trim();
  }

  @Named("trimToNull")
  default String trimToNull(String value) {
    if (value == null) {
      return null;
    }

    String trimmed = value.trim();
    return trimmed.isBlank() ? null : trimmed;
  }

  default JsonNode toDefinitionJson(
      List<WorkflowNodeRequest> nodes, List<WorkflowEdgeRequest> edges, JsonNode metadata) {
    ObjectNode definition = JsonNodeFactory.instance.objectNode();
    definition.set("nodes", toNodeArray(nodes));
    definition.set("edges", toEdgeArray(edges));
    definition.set("metadata", copyOrNull(metadata));
    return definition;
  }

  @Named("nodes")
  default List<WorkflowNodeResponse> nodes(Workflow workflow) {
    JsonNode nodes = arrayField(workflow, "nodes");
    List<WorkflowNodeResponse> responses = new ArrayList<>();

    for (JsonNode node : nodes) {
      JsonNode configuration = node.path("configuration");
      JsonNode position = node.path("position");
      String pluginType = node.path("pluginType").asText();
      responses.add(
          new WorkflowNodeResponse(
              node.path("nodeId").asText(),
              optionalText(node.get("displayName")),
              pluginType,
              node.path("pluginVersion").asText(),
              redactConfiguration(configuration),
              new NodePositionResponse(
                  position.path("x").asDouble(), position.path("y").asDouble())));
    }

    return List.copyOf(responses);
  }

  @Named("edges")
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

  @Named("metadata")
  default JsonNode metadata(Workflow workflow) {
    JsonNode definition = workflow.getDefinitionJson();
    if (definition == null || !definition.isObject()) {
      return JsonNodeFactory.instance.objectNode();
    }

    JsonNode metadata = definition.path("metadata");
    return metadata.isObject() ? metadata.deepCopy() : JsonNodeFactory.instance.objectNode();
  }

  @Named("nodeCount")
  default Integer nodeCount(Workflow workflow) {
    return arrayField(workflow, "nodes").size();
  }

  @Named("edgeCount")
  default Integer edgeCount(Workflow workflow) {
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
      nodeObject.put("displayName", optionalText(node.displayName()));
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

  private static String optionalText(JsonNode value) {
    return value != null && value.isTextual() ? optionalText(value.textValue()) : null;
  }

  private static String optionalText(String value) {
    if (value == null) {
      return null;
    }
    String trimmed = value.trim();
    return trimmed.isBlank() ? null : trimmed;
  }

  private static JsonNode redactConfiguration(JsonNode configuration) {
    ObjectNode copy =
        configuration != null && configuration.isObject()
            ? configuration.deepCopy()
            : JsonNodeFactory.instance.objectNode();
    boolean encryptedPasswordPresent = copy.has("sftpPasswordEncrypted");
    copy.remove("sftpPasswordEncrypted");
    copy.remove("sftpPassword");
    if (encryptedPasswordPresent) {
      copy.put("sftpPassword", "");
    }
    return copy;
  }
}
