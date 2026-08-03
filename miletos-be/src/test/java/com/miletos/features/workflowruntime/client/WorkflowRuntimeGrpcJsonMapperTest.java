package com.miletos.features.workflowruntime.client;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.protobuf.Empty;
import com.google.protobuf.ListValue;
import com.google.protobuf.Message;
import com.google.protobuf.NullValue;
import com.google.protobuf.Struct;
import com.google.protobuf.Timestamp;
import com.google.protobuf.Value;
import com.miletos.features.workflowruntime.grpc.generated.CreateHTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionDefinition;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionError;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionErrorPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionEvent;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionEventPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionLog;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionLogPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionResponse;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionSummary;
import com.miletos.features.workflowruntime.grpc.generated.HTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.ListPluginsResponse;
import com.miletos.features.workflowruntime.grpc.generated.NodeExecution;
import com.miletos.features.workflowruntime.grpc.generated.NodeExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.Plugin;
import com.miletos.features.workflowruntime.grpc.generated.RecoveryResponse;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowDefinition;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowEdge;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowNode;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.stream.Stream;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;

class WorkflowRuntimeGrpcJsonMapperTest {

  private final ObjectMapper objectMapper = new ObjectMapper();
  private WorkflowRuntimeGrpcJsonAdapter adapter;

  @BeforeEach
  void setUp() {
    adapter =
        new WorkflowRuntimeGrpcJsonAdapter(objectMapper, new WorkflowRuntimeGrpcJsonMapperImpl());
  }

  static Stream<Arguments> supportedResponses() {
    return Stream.of(
        Arguments.of(
            ListPluginsResponse.newBuilder()
                .setCount(1)
                .addItems(Plugin.newBuilder().setType("http"))
                .build(),
            "/items/0/type",
            "http"),
        Arguments.of(
            ExecutionResponse.newBuilder().setWorkflowRevision(7).build(),
            "/workflowRevision",
            "7"),
        Arguments.of(
            ExecutionSummary.newBuilder().setExecutionId("execution-summary").build(),
            "/executionId",
            "execution-summary"),
        Arguments.of(
            ExecutionPage.newBuilder()
                .setCursor("execution-cursor")
                .setHasNext(true)
                .addItems(ExecutionSummary.newBuilder().setExecutionId("execution-page"))
                .build(),
            "/next",
            "execution-cursor"),
        Arguments.of(
            ExecutionDefinition.newBuilder()
                .setWorkflowRevision(8)
                .setDefinition(WorkflowDefinition.newBuilder().setWorkflowId("workflow"))
                .build(),
            "/definition/id",
            "workflow"),
        Arguments.of(
            NodeExecutionPage.newBuilder()
                .setCursor("node-cursor")
                .addItems(NodeExecution.newBuilder().setExecutionId("node-execution"))
                .build(),
            "/items/0/workflowExecutionId",
            "node-execution"),
        Arguments.of(
            ExecutionEventPage.newBuilder()
                .setCursor("event-cursor")
                .addItems(ExecutionEvent.newBuilder().setExecutionId("event-execution"))
                .build(),
            "/items/0/workflowExecutionId",
            "event-execution"),
        Arguments.of(
            ExecutionLogPage.newBuilder()
                .setCursor("log-cursor")
                .addItems(ExecutionLog.newBuilder().setExecutionId("log-execution"))
                .build(),
            "/items/0/workflowExecutionId",
            "log-execution"),
        Arguments.of(
            ExecutionErrorPage.newBuilder()
                .setCursor("error-cursor")
                .addItems(ExecutionError.newBuilder().setExecutionId("error-execution"))
                .build(),
            "/items/0/workflowExecutionId",
            "error-execution"),
        Arguments.of(
            RecoveryResponse.newBuilder().setRecoveryExecutionId("recovery").build(),
            "/recoveryExecutionId",
            "recovery"),
        Arguments.of(
            HTTPTriggerResponse.newBuilder().setWorkflowRevision(9).build(),
            "/workflowRevision",
            "9"),
        Arguments.of(
            CreateHTTPTriggerResponse.newBuilder().setPublicUrl("https://trigger").build(),
            "/publicUrl",
            "https://trigger"));
  }

  @ParameterizedTest
  @MethodSource("supportedResponses")
  void mapsEverySupportedResponseType(Message response, String pointer, String expected)
      throws Exception {
    JsonNode document = objectMapper.readTree(adapter.toBrowserJson(response));

    assertThat(document.at(pointer).asText()).isEqualTo(expected);
  }

  @Test
  void preservesBrowserFieldNamesAndProtobufValueSemantics() throws Exception {
    Struct values =
        Struct.newBuilder()
            .putFields(
                "emptyObject",
                Value.newBuilder().setStructValue(Struct.getDefaultInstance()).build())
            .putFields(
                "emptyList",
                Value.newBuilder().setListValue(ListValue.getDefaultInstance()).build())
            .putFields("nullValue", Value.newBuilder().setNullValue(NullValue.NULL_VALUE).build())
            .putFields("enabled", Value.newBuilder().setBoolValue(true).build())
            .build();
    ExecutionDefinition response =
        ExecutionDefinition.newBuilder()
            .setWorkflowRevision(42)
            .setDefinition(
                WorkflowDefinition.newBuilder()
                    .setWorkflowId("workflow-1")
                    .setWorkflowRevision(42)
                    .addNodes(
                        WorkflowNode.newBuilder().setNodeId("node-1").setConfiguration(values))
                    .addEdges(WorkflowEdge.newBuilder().setEdgeId("edge-1"))
                    .setMetadata(Struct.getDefaultInstance()))
            .build();

    JsonNode document = objectMapper.readTree(adapter.toBrowserJson(response));

    assertThat(document.at("/workflowRevision").asLong()).isEqualTo(42);
    assertThat(document.at("/definition/id").asText()).isEqualTo("workflow-1");
    assertThat(document.at("/definition/revision").asLong()).isEqualTo(42);
    assertThat(document.at("/definition/nodes/0/id").asText()).isEqualTo("node-1");
    assertThat(document.at("/definition/edges/0/id").asText()).isEqualTo("edge-1");
    assertThat(document.at("/definition/nodes/0/configuration/emptyObject").isObject()).isTrue();
    assertThat(document.at("/definition/nodes/0/configuration/emptyObject").isEmpty()).isTrue();
    assertThat(document.at("/definition/nodes/0/configuration/emptyList").isArray()).isTrue();
    assertThat(document.at("/definition/nodes/0/configuration/emptyList").isEmpty()).isTrue();
    assertThat(document.at("/definition/nodes/0/configuration/nullValue").isNull()).isTrue();
    assertThat(document.at("/definition/nodes/0/configuration/enabled").asBoolean()).isTrue();
    assertThat(document.at("/definition/metadata").isObject()).isTrue();
  }

  @Test
  void mapsGenericNodeDataTimestampsPaginationAndStableDefaults() throws Exception {
    Struct input =
        Struct.newBuilder()
            .putFields("secret", Value.newBuilder().setStringValue("[REDACTED]").build())
            .build();
    NodeExecutionPage response =
        NodeExecutionPage.newBuilder()
            .setCursor("next-page")
            .setHasNext(true)
            .addItems(
                NodeExecution.newBuilder()
                    .setNodeExecutionId("node-execution-1")
                    .setExecutionId("execution-1")
                    .setCreatedAt(Timestamp.newBuilder().setSeconds(1))
                    .setConfiguration(Struct.getDefaultInstance())
                    .setInputSummary(input)
                    .setOutputSummary(Struct.getDefaultInstance())
                    .setFailureSummary(
                        Struct.newBuilder()
                            .putFields(
                                "code", Value.newBuilder().setStringValue("FAILED").build())))
            .build();

    JsonNode document = objectMapper.readTree(adapter.toBrowserJson(response));

    assertThat(document.path("next").asText()).isEqualTo("next-page");
    assertThat(document.path("hasNext").asBoolean()).isTrue();
    assertThat(document.at("/items/0/workflowExecutionId").asText()).isEqualTo("execution-1");
    assertThat(document.at("/items/0/nodeExecutionId").asText()).isEqualTo("node-execution-1");
    assertThat(document.at("/items/0/createdAt").asText()).isEqualTo("1970-01-01T00:00:01Z");
    assertThat(document.at("/items/0/configuration").isEmpty()).isTrue();
    assertThat(document.at("/items/0/inputSummary/secret").asText()).isEqualTo("[REDACTED]");
    assertThat(document.at("/items/0/outputSummary").isEmpty()).isTrue();
    assertThat(document.at("/items/0/failureSummary/code").asText()).isEqualTo("FAILED");
    assertThat(document.at("/items/0/attempt").asInt()).isZero();
    assertThat(document.has("unknownFields")).isFalse();
  }

  @Test
  void mapsNestedTriggerAndOmitsAbsentOptionalValues() throws Exception {
    CreateHTTPTriggerResponse response =
        CreateHTTPTriggerResponse.newBuilder()
            .setTrigger(
                HTTPTriggerResponse.newBuilder().setTriggerId("trigger-1").setWorkflowRevision(13))
            .setPublicUrl("https://example.test/hooks/token")
            .build();

    JsonNode document = objectMapper.readTree(adapter.toBrowserJson(response));

    assertThat(document.at("/trigger/triggerId").asText()).isEqualTo("trigger-1");
    assertThat(document.at("/trigger/workflowRevision").asLong()).isEqualTo(13);
    assertThat(document.at("/publicUrl").asText()).isEqualTo("https://example.test/hooks/token");
    assertThat(document.at("/trigger/createdAt").isMissingNode()).isTrue();
  }

  @Test
  void normalizesBrowserDefinitionNamesForProtobufRequests() throws Exception {
    JsonNode request =
        objectMapper.readTree(
            """
                {
                  "definition": {
                    "id": "workflow-1",
                    "name": "Example",
                    "revision": 11,
                    "nodes": [{"id": "node-1", "pluginType": "http", "configuration": {}}],
                    "edges": [{"id": "edge-1", "sourceNodeId": "node-1", "targetNodeId": "node-2"}],
                    "metadata": {}
                  },
                  "initialVariables": {"items": [], "settings": {}}
                }
                """);

    var mapped = adapter.toExecuteRequest(request);

    assertThat(mapped.getDefinition().getWorkflowId()).isEqualTo("workflow-1");
    assertThat(mapped.getDefinition().getWorkflowRevision()).isEqualTo(11);
    assertThat(mapped.getDefinition().getNodes(0).getNodeId()).isEqualTo("node-1");
    assertThat(mapped.getDefinition().getEdges(0).getEdgeId()).isEqualTo("edge-1");
    assertThat(
            mapped.getInitialVariables().getFieldsOrThrow("items").getListValue().getValuesCount())
        .isZero();
    assertThat(
            mapped
                .getInitialVariables()
                .getFieldsOrThrow("settings")
                .getStructValue()
                .getFieldsCount())
        .isZero();
  }

  @Test
  void rejectsMalformedRequestsAndUnsupportedResponsesExplicitly() {
    assertThatThrownBy(() -> adapter.toExecuteRequest(objectMapper.createArrayNode()))
        .isInstanceOf(IllegalArgumentException.class)
        .hasMessage("Execution request must be a JSON object");
    assertThatThrownBy(() -> adapter.toBrowserJson(Empty.getDefaultInstance()))
        .isInstanceOf(WorkflowRuntimeMappingException.class)
        .hasMessageContaining("Unsupported runtime response type");
  }

  @Test
  void preservesTheApprovedMapperAndAdapterArchitecture() throws Exception {
    String mapperSource =
        Files.readString(
            Path.of(
                "src/main/java/com/miletos/features/workflowruntime/client/WorkflowRuntimeGrpcJsonMapper.java"));
    String adapterSource =
        Files.readString(
            Path.of(
                "src/main/java/com/miletos/features/workflowruntime/client/WorkflowRuntimeGrpcJsonAdapter.java"));

    assertThat(mapperSource)
        .contains("@Mapper(", "uses = ProtobufValueConverter.class")
        .doesNotContain("instanceof");
    assertThat(adapterSource)
        .doesNotContain(
            "instanceof Execution", "HashMap", "ResponseNormalizer", "Mappers.getMapper")
        .doesNotContain(
            "ExecutionGrpcResponseMapper",
            "HTTPTriggerGrpcResponseMapper",
            "PluginGrpcResponseMapper");
  }
}
