package com.miletos.features.workflowruntime.client;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.protobuf.Any;
import com.google.protobuf.Empty;
import com.google.rpc.ErrorInfo;
import com.miletos.features.workflowruntime.config.WorkflowRuntimeProperties;
import io.grpc.ManagedChannel;
import io.grpc.StatusRuntimeException;
import io.grpc.protobuf.StatusProto;
import java.time.Duration;
import java.util.List;
import java.util.function.Function;
import java.util.stream.Stream;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.util.MultiValueMap;

class WorkflowRuntimeGrpcClientTest {

  private final ObjectMapper objectMapper = new ObjectMapper();
  private WorkflowRuntimeGrpcClient client;

  @BeforeEach
  void setUp() {
    var adapter =
        new WorkflowRuntimeGrpcJsonAdapter(objectMapper, new WorkflowRuntimeGrpcJsonMapperImpl());
    var properties =
        new WorkflowRuntimeProperties(
            "localhost", 9091, "test-internal-service-token-value-32-bytes", Duration.ofSeconds(5));
    client =
        new WorkflowRuntimeGrpcClient(
            mock(ManagedChannel.class), properties, adapter, objectMapper);
  }

  @Test
  void classifiesMalformedBrowserRequestsAsBadRequests() throws Exception {
    WorkflowRuntimeResponse response =
        client.invoke(
            "company-1",
            new HttpHeaders(),
            HttpStatus.OK,
            ignored -> clientAdapter().toExecuteRequest(objectMapper.createArrayNode()));

    assertError(response, HttpStatus.BAD_REQUEST, "INVALID_EXECUTION_REQUEST");
  }

  @Test
  void classifiesUnsupportedProtobufResponsesAsInternalErrorsWithoutLeakingDetails()
      throws Exception {
    WorkflowRuntimeResponse response =
        client.invoke(
            "company-1", new HttpHeaders(), HttpStatus.OK, ignored -> Empty.getDefaultInstance());

    JsonNode body =
        assertError(response, HttpStatus.INTERNAL_SERVER_ERROR, "RUNTIME_RESPONSE_MAPPING_FAILED");
    assertThat(body.path("message").asText()).isEqualTo("An unexpected internal error occurred.");
    assertThat(new String(response.body())).doesNotContain("Empty", "protobuf", "Unsupported");
  }

  @Test
  void mapsIdempotencyRequestInProgressToStructuredConflict() throws Exception {
    WorkflowRuntimeResponse response =
        client.invoke(
            "company-1",
            new HttpHeaders(),
            HttpStatus.ACCEPTED,
            ignored -> {
              throw grpcError(
                  io.grpc.Status.Code.FAILED_PRECONDITION, "IDEMPOTENCY_REQUEST_IN_PROGRESS");
            });

    assertError(response, HttpStatus.CONFLICT, "IDEMPOTENCY_REQUEST_IN_PROGRESS");
  }

  @Test
  void keepsIdempotencyKeyReusedMappingUnchanged() throws Exception {
    WorkflowRuntimeResponse response =
        client.invoke(
            "company-1",
            new HttpHeaders(),
            HttpStatus.ACCEPTED,
            ignored -> {
              throw grpcError(io.grpc.Status.Code.ALREADY_EXISTS, "IDEMPOTENCY_KEY_REUSED");
            });

    assertError(response, HttpStatus.CONFLICT, "IDEMPOTENCY_KEY_REUSED");
  }

  @Test
  void keepsUnexpectedGrpcFailuresAsInternalErrors() throws Exception {
    WorkflowRuntimeResponse response =
        client.invoke(
            "company-1",
            new HttpHeaders(),
            HttpStatus.ACCEPTED,
            ignored -> {
              throw io.grpc.Status.INTERNAL.withDescription("database detail").asRuntimeException();
            });

    JsonNode body =
        assertError(response, HttpStatus.INTERNAL_SERVER_ERROR, "INTERNAL_SERVER_ERROR");
    assertThat(body.path("message").asText()).isEqualTo("An unexpected internal error occurred.");
    assertThat(new String(response.body())).doesNotContain("database detail");
  }

  @Test
  void buildsExecutionAndResourcePaginationAtTheirDocumentedBoundaries() {
    var execution = client.listExecutionsRequest(query("limit", "100", "after", " next "));
    assertThat(execution.getLimit()).isEqualTo(100);
    assertThat(execution.getCursor()).isEqualTo("next");

    var resource = client.resourceRequest("execution-1", query("limit", "200", "after", " page "));
    assertThat(resource.getExecutionId()).isEqualTo("execution-1");
    assertThat(resource.getLimit()).isEqualTo(200);
    assertThat(resource.getCursor()).isEqualTo("page");

    assertThat(client.listExecutionsRequest(new LinkedMultiValueMap<>()).getLimit()).isZero();
    assertThat(client.resourceRequest("execution-1", new LinkedMultiValueMap<>()).getLimit())
        .isZero();
    assertThat(client.listExecutionsRequest(query("limit", "1")).getLimit()).isEqualTo(1);
    assertThat(client.resourceRequest("execution-1", query("limit", "1")).getLimit()).isEqualTo(1);
  }

  static Stream<Arguments> invalidPagination() {
    return Stream.of(
        Arguments.of("limit", List.of("0")),
        Arguments.of("limit", List.of("-1")),
        Arguments.of("limit", List.of("invalid")),
        Arguments.of("limit", List.of("201")),
        Arguments.of("limit", List.of("")),
        Arguments.of("limit", List.of("1", "2")),
        Arguments.of("after", List.of("")),
        Arguments.of("after", List.of("one", "two")));
  }

  @ParameterizedTest
  @MethodSource("invalidPagination")
  void rejectsMalformedPaginationConsistentlyAcrossExecutionResourceEndpoints(
      String key, List<String> values) throws Exception {
    MultiValueMap<String, String> query = new LinkedMultiValueMap<>();
    query.put(key, values);
    List<Function<MultiValueMap<String, String>, WorkflowRuntimeResponse>> endpoints =
        List.of(
            value -> client.listExecutions(value, "company-1", new HttpHeaders()),
            value -> client.getExecutionNodes("execution-1", value, "company-1", new HttpHeaders()),
            value ->
                client.getExecutionEvents("execution-1", value, "company-1", new HttpHeaders()),
            value -> client.getExecutionLogs("execution-1", value, "company-1", new HttpHeaders()),
            value ->
                client.getExecutionErrors("execution-1", value, "company-1", new HttpHeaders()));

    for (Function<MultiValueMap<String, String>, WorkflowRuntimeResponse> endpoint : endpoints) {
      assertError(endpoint.apply(query), HttpStatus.BAD_REQUEST, "INVALID_EXECUTION_REQUEST");
    }
  }

  @Test
  void enforcesDifferentExecutionAndResourceMaximums() {
    assertThatThrownBy(() -> client.listExecutionsRequest(query("limit", "101")))
        .isInstanceOf(IllegalArgumentException.class)
        .hasMessageContaining("between 1 and 100");
    assertThat(client.resourceRequest("execution-1", query("limit", "101")).getLimit())
        .isEqualTo(101);
    assertThatThrownBy(() -> client.resourceRequest("execution-1", query("limit", "201")))
        .isInstanceOf(IllegalArgumentException.class)
        .hasMessageContaining("between 1 and 200");
  }

  private WorkflowRuntimeGrpcJsonAdapter clientAdapter() {
    return new WorkflowRuntimeGrpcJsonAdapter(
        objectMapper, new WorkflowRuntimeGrpcJsonMapperImpl());
  }

  private static StatusRuntimeException grpcError(io.grpc.Status.Code code, String reason) {
    ErrorInfo errorInfo =
        ErrorInfo.newBuilder().setReason(reason).setDomain("miletos.runtime").build();
    com.google.rpc.Status grpcStatus =
        com.google.rpc.Status.newBuilder()
            .setCode(code.value())
            .setMessage("structured runtime failure")
            .addDetails(Any.pack(errorInfo))
            .build();
    return StatusProto.toStatusRuntimeException(grpcStatus);
  }

  private JsonNode assertError(
      WorkflowRuntimeResponse response, HttpStatus expectedStatus, String expectedCode)
      throws Exception {
    assertThat(response.status()).isEqualTo(expectedStatus);
    JsonNode body = objectMapper.readTree(response.body());
    assertThat(body.path("status").asInt()).isEqualTo(expectedStatus.value());
    assertThat(body.path("code").asText()).isEqualTo(expectedCode);
    return body;
  }

  private static MultiValueMap<String, String> query(String... entries) {
    MultiValueMap<String, String> result = new LinkedMultiValueMap<>();
    for (int index = 0; index < entries.length; index += 2) {
      result.add(entries[index], entries[index + 1]);
    }
    return result;
  }
}
