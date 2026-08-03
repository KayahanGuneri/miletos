package com.miletos.features.workflowruntime.client;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.google.protobuf.Message;
import com.google.protobuf.util.JsonFormat;
import com.miletos.features.workflowruntime.grpc.generated.CreateHTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.ExecuteRequest;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionDefinition;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionErrorPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionEventPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionLogPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionResponse;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionSummary;
import com.miletos.features.workflowruntime.grpc.generated.HTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.ListPluginsResponse;
import com.miletos.features.workflowruntime.grpc.generated.NodeExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.RecoveryResponse;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowDefinition;
import java.io.IOException;
import java.util.Map;
import java.util.function.Function;
import org.springframework.stereotype.Component;

@Component
public class WorkflowRuntimeGrpcJsonAdapter {

  private final ObjectMapper requestObjectMapper;
  private final ObjectMapper responseObjectMapper;
  private final Map<Class<? extends Message>, Function<Message, byte[]>> responseMappings;

  public WorkflowRuntimeGrpcJsonAdapter(
      ObjectMapper objectMapper, WorkflowRuntimeGrpcJsonMapper mapper) {
    this.requestObjectMapper = objectMapper;
    this.responseObjectMapper =
        objectMapper.copy().setSerializationInclusion(JsonInclude.Include.NON_NULL);
    this.responseMappings =
        Map.ofEntries(
            mapping(ListPluginsResponse.class, mapper::map),
            mapping(ExecutionResponse.class, mapper::map),
            mapping(ExecutionSummary.class, mapper::map),
            mapping(ExecutionPage.class, mapper::map),
            mapping(ExecutionDefinition.class, mapper::map),
            mapping(NodeExecutionPage.class, mapper::map),
            mapping(ExecutionEventPage.class, mapper::map),
            mapping(ExecutionLogPage.class, mapper::map),
            mapping(ExecutionErrorPage.class, mapper::map),
            mapping(RecoveryResponse.class, mapper::map),
            mapping(HTTPTriggerResponse.class, mapper::map),
            mapping(CreateHTTPTriggerResponse.class, mapper::map));
  }

  public ExecuteRequest toExecuteRequest(JsonNode browserRequest) {
    if (browserRequest == null || !browserRequest.isObject()) {
      throw new IllegalArgumentException("Execution request must be a JSON object");
    }
    ObjectNode normalized = browserRequest.deepCopy();
    JsonNode definition = normalized.get("definition");
    if (!(definition instanceof ObjectNode definitionObject)) {
      throw new IllegalArgumentException("Workflow definition is required");
    }
    normalizeDefinitionForProto(definitionObject);
    ExecuteRequest.Builder builder = ExecuteRequest.newBuilder();
    try {
      JsonFormat.parser().merge(requestObjectMapper.writeValueAsString(normalized), builder);
      return builder.build();
    } catch (IOException exception) {
      throw new IllegalArgumentException("Execution request is invalid", exception);
    }
  }

  public WorkflowDefinition toWorkflowDefinition(JsonNode browserDefinition) {
    if (!(browserDefinition instanceof ObjectNode definitionObject)) {
      throw new IllegalArgumentException("Workflow definition is required");
    }
    ObjectNode normalized = definitionObject.deepCopy();
    normalizeDefinitionForProto(normalized);
    WorkflowDefinition.Builder builder = WorkflowDefinition.newBuilder();
    try {
      JsonFormat.parser().merge(requestObjectMapper.writeValueAsString(normalized), builder);
      return builder.build();
    } catch (IOException exception) {
      throw new IllegalArgumentException("Workflow definition is invalid", exception);
    }
  }

  public byte[] toBrowserJson(Message message) {
    Function<Message, byte[]> mapping = responseMappings.get(message.getClass());
    if (mapping == null) {
      throw new WorkflowRuntimeMappingException(
          "Unsupported runtime response type: " + message.getClass().getName());
    }
    return mapping.apply(message);
  }

  private <T extends Message, R>
      Map.Entry<Class<? extends Message>, Function<Message, byte[]>> mapping(
          Class<T> messageType, Function<T, R> mapper) {
    return Map.entry(messageType, message -> serialize(mapper.apply(messageType.cast(message))));
  }

  private <T> byte[] serialize(T response) {
    try {
      return responseObjectMapper.writeValueAsBytes(response);
    } catch (IOException exception) {
      throw new WorkflowRuntimeMappingException("Runtime response could not be mapped", exception);
    }
  }

  private void normalizeDefinitionForProto(ObjectNode definition) {
    rename(definition, "id", "workflowId");
    rename(definition, "revision", "workflowRevision");
    JsonNode nodes = definition.get("nodes");
    if (nodes instanceof ArrayNode array) {
      array.forEach(
          node -> {
            if (node instanceof ObjectNode object) {
              rename(object, "id", "nodeId");
            }
          });
    }
    JsonNode edges = definition.get("edges");
    if (edges instanceof ArrayNode array) {
      array.forEach(
          edge -> {
            if (edge instanceof ObjectNode object) {
              rename(object, "id", "edgeId");
            }
          });
    }
  }

  private void rename(ObjectNode object, String source, String target) {
    JsonNode value = object.remove(source);
    if (value != null) {
      object.set(target, value);
    }
  }
}

final class WorkflowRuntimeMappingException extends IllegalStateException {

  WorkflowRuntimeMappingException(String message) {
    super(message);
  }

  WorkflowRuntimeMappingException(String message, Throwable cause) {
    super(message, cause);
  }
}
