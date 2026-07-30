package com.miletos.features.workflowruntime.client;

import java.io.IOException;
import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.google.protobuf.Message;
import com.google.protobuf.util.JsonFormat;
import com.miletos.features.workflowruntime.grpc.generated.ExecuteRequest;
import com.miletos.features.workflowruntime.grpc.generated.CreateHTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionDefinition;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionErrorPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionEventPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionLogPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionResponse;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionSummary;
import com.miletos.features.workflowruntime.grpc.generated.HTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.NodeExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowDefinition;

@Component
public class WorkflowRuntimeGrpcJsonMapper {

    private final ObjectMapper objectMapper;

    public WorkflowRuntimeGrpcJsonMapper(ObjectMapper objectMapper) {
        this.objectMapper = objectMapper;
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
            JsonFormat.parser().merge(objectMapper.writeValueAsString(normalized), builder);
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
            JsonFormat.parser().merge(objectMapper.writeValueAsString(normalized), builder);
            return builder.build();
        } catch (IOException exception) {
            throw new IllegalArgumentException("Workflow definition is invalid", exception);
        }
    }

    public byte[] toBrowserJson(Message message) {
        try {
            ObjectNode root = (ObjectNode) objectMapper.readTree(
                    JsonFormat.printer()
                            .alwaysPrintFieldsWithNoPresence()
                            .omittingInsignificantWhitespace()
                            .print(message));
            normalizeBrowserResponse(message, root);
            return objectMapper.writeValueAsBytes(root);
        } catch (IOException exception) {
            throw new IllegalStateException("Runtime response could not be mapped", exception);
        }
    }

    private void normalizeBrowserResponse(Message message, ObjectNode root) {
        if (message instanceof ExecutionResponse response) {
            root.put("workflowRevision", response.getWorkflowRevision());
        } else if (message instanceof ExecutionSummary summary) {
            normalizeExecutionSummary(root, summary);
        } else if (message instanceof ExecutionPage page) {
            renameCursor(root);
            ArrayNode items = root.withArray("items");
            for (int index = 0; index < page.getItemsCount(); index++) {
                normalizeExecutionSummary(
                        (ObjectNode) items.get(index), page.getItems(index));
            }
        } else if (message instanceof ExecutionDefinition definition) {
            root.put("workflowRevision", definition.getWorkflowRevision());
            JsonNode nested = root.get("definition");
            if (nested instanceof ObjectNode nestedObject) {
                normalizeDefinitionForBrowser(nestedObject);
            }
        } else if (message instanceof NodeExecutionPage) {
            renameCursor(root);
            renameExecutionIds(root.withArray("items"));
        } else if (message instanceof ExecutionEventPage
                || message instanceof ExecutionLogPage
                || message instanceof ExecutionErrorPage) {
            renameCursor(root);
            renameExecutionIds(root.withArray("items"));
        } else if (message instanceof HTTPTriggerResponse trigger) {
            root.put("workflowRevision", trigger.getWorkflowRevision());
        } else if (message instanceof CreateHTTPTriggerResponse created) {
            JsonNode trigger = root.get("trigger");
            if (trigger instanceof ObjectNode triggerObject) {
                triggerObject.put(
                        "workflowRevision",
                        created.getTrigger().getWorkflowRevision());
            }
        }
    }

    private void normalizeExecutionSummary(
            ObjectNode node,
            ExecutionSummary summary) {
        node.put("workflowRevision", summary.getWorkflowRevision());
    }

    private void renameCursor(ObjectNode root) {
        JsonNode cursor = root.remove("cursor");
        root.put("next", cursor == null ? "" : cursor.asText(""));
    }

    private void renameExecutionIds(ArrayNode items) {
        for (JsonNode item : items) {
            if (item instanceof ObjectNode object) {
                JsonNode executionId = object.remove("executionId");
                if (executionId != null) {
                    object.set("workflowExecutionId", executionId);
                }
            }
        }
    }

    private void normalizeDefinitionForProto(ObjectNode definition) {
        rename(definition, "id", "workflowId");
        rename(definition, "revision", "workflowRevision");
        JsonNode nodes = definition.get("nodes");
        if (nodes instanceof ArrayNode array) {
            array.forEach(node -> {
                if (node instanceof ObjectNode object) {
                    rename(object, "id", "nodeId");
                }
            });
        }
        JsonNode edges = definition.get("edges");
        if (edges instanceof ArrayNode array) {
            array.forEach(edge -> {
                if (edge instanceof ObjectNode object) {
                    rename(object, "id", "edgeId");
                }
            });
        }
    }

    private void normalizeDefinitionForBrowser(ObjectNode definition) {
        rename(definition, "workflowId", "id");
        JsonNode revision = definition.remove("workflowRevision");
        if (revision != null) {
            definition.put("revision", revision.asLong());
        }
        JsonNode nodes = definition.get("nodes");
        if (nodes instanceof ArrayNode array) {
            array.forEach(node -> {
                if (node instanceof ObjectNode object) {
                    rename(object, "nodeId", "id");
                }
            });
        }
        JsonNode edges = definition.get("edges");
        if (edges instanceof ArrayNode array) {
            array.forEach(edge -> {
                if (edge instanceof ObjectNode object) {
                    rename(object, "edgeId", "id");
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
