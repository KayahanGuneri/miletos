package com.miletos.features.workflowruntime.client;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.google.protobuf.Any;
import com.google.protobuf.Empty;
import com.google.rpc.ErrorInfo;
import com.google.rpc.Status;
import com.miletos.features.workflowruntime.config.WorkflowRuntimeProperties;
import com.miletos.features.workflowruntime.grpc.generated.CreateHTTPTriggerRequest;
import com.miletos.features.workflowruntime.grpc.generated.DisableHTTPTriggerRequest;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionServiceGrpc;
import com.miletos.features.workflowruntime.grpc.generated.GetExecutionRequest;
import com.miletos.features.workflowruntime.grpc.generated.GetHTTPTriggerRequest;
import com.miletos.features.workflowruntime.grpc.generated.HTTPTriggerServiceGrpc;
import com.miletos.features.workflowruntime.grpc.generated.ListExecutionResourceRequest;
import com.miletos.features.workflowruntime.grpc.generated.ListExecutionsRequest;
import com.miletos.features.workflowruntime.grpc.generated.PluginServiceGrpc;
import com.miletos.features.workflowruntime.grpc.generated.RecoverExecutionRequest;
import com.miletos.features.workflowruntime.grpc.generated.ValidationIssue;
import io.grpc.CallOptions;
import io.grpc.Channel;
import io.grpc.ClientCall;
import io.grpc.ClientInterceptor;
import io.grpc.ClientInterceptors;
import io.grpc.ManagedChannel;
import io.grpc.Metadata;
import io.grpc.MethodDescriptor;
import io.grpc.StatusRuntimeException;
import io.grpc.protobuf.StatusProto;
import io.grpc.stub.MetadataUtils;
import java.time.Instant;
import java.util.List;
import java.util.function.Function;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.util.MultiValueMap;

@Component
public class WorkflowRuntimeGrpcClient {

  private static final int MAXIMUM_EXECUTION_PAGE_SIZE = 100;
  private static final int MAXIMUM_RESOURCE_PAGE_SIZE = 200;

  private static final Metadata.Key<String> AUTHORIZATION =
      Metadata.Key.of("authorization", Metadata.ASCII_STRING_MARSHALLER);
  private static final Metadata.Key<String> COMPANY =
      Metadata.Key.of("x-miletos-company-id", Metadata.ASCII_STRING_MARSHALLER);
  private static final Metadata.Key<String> CORRELATION =
      Metadata.Key.of("x-correlation-id", Metadata.ASCII_STRING_MARSHALLER);
  private static final Metadata.Key<String> REQUEST_ID =
      Metadata.Key.of("x-request-id", Metadata.ASCII_STRING_MARSHALLER);
  private static final Metadata.Key<String> IDEMPOTENCY =
      Metadata.Key.of("idempotency-key", Metadata.ASCII_STRING_MARSHALLER);

  private final ManagedChannel channel;
  private final WorkflowRuntimeProperties properties;
  private final WorkflowRuntimeGrpcJsonAdapter jsonAdapter;
  private final ObjectMapper objectMapper;

  public WorkflowRuntimeGrpcClient(
      ManagedChannel workflowRuntimeChannel,
      WorkflowRuntimeProperties properties,
      WorkflowRuntimeGrpcJsonAdapter jsonAdapter,
      ObjectMapper objectMapper) {
    this.channel = workflowRuntimeChannel;
    this.properties = properties;
    this.jsonAdapter = jsonAdapter;
    this.objectMapper = objectMapper;
  }

  public WorkflowRuntimeResponse getPlugins(String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub -> PluginServiceGrpc.newBlockingStub(stub).listPlugins(Empty.getDefaultInstance()));
  }

  public WorkflowRuntimeResponse executeSync(
      JsonNode body, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .executeSync(jsonAdapter.toExecuteRequest(body)));
  }

  public WorkflowRuntimeResponse executeAsync(
      JsonNode body, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.ACCEPTED,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .executeAsync(jsonAdapter.toExecuteRequest(body)));
  }

  public WorkflowRuntimeResponse recoverExecution(
      String executionId, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.ACCEPTED,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .recoverExecution(
                    RecoverExecutionRequest.newBuilder().setExecutionId(executionId).build()));
  }

  public WorkflowRuntimeResponse listExecutions(
      MultiValueMap<String, String> query, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .listExecutions(listExecutionsRequest(query)));
  }

  public WorkflowRuntimeResponse getExecution(
      String executionId, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub).getExecution(executionRequest(executionId)));
  }

  public WorkflowRuntimeResponse getExecutionDefinition(
      String executionId, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .getExecutionDefinition(executionRequest(executionId)));
  }

  public WorkflowRuntimeResponse getExecutionNodes(
      String executionId,
      MultiValueMap<String, String> query,
      String companyId,
      HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .listExecutionNodes(resourceRequest(executionId, query)));
  }

  public WorkflowRuntimeResponse getExecutionEvents(
      String executionId,
      MultiValueMap<String, String> query,
      String companyId,
      HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .listExecutionEvents(resourceRequest(executionId, query)));
  }

  public WorkflowRuntimeResponse getExecutionLogs(
      String executionId,
      MultiValueMap<String, String> query,
      String companyId,
      HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .listExecutionLogs(resourceRequest(executionId, query)));
  }

  public WorkflowRuntimeResponse getExecutionErrors(
      String executionId,
      MultiValueMap<String, String> query,
      String companyId,
      HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            ExecutionServiceGrpc.newBlockingStub(stub)
                .listExecutionErrors(resourceRequest(executionId, query)));
  }

  public WorkflowRuntimeResponse createHTTPTrigger(
      JsonNode body, String companyId, HttpHeaders browserHeaders) {
    if (body == null || !body.isObject()) {
      return localError(
          HttpStatus.BAD_REQUEST,
          "INVALID_HTTP_TRIGGER_REQUEST",
          "HTTP trigger request must be a JSON object.",
          browserHeaders);
    }
    CreateHTTPTriggerRequest request;
    try {
      request =
          CreateHTTPTriggerRequest.newBuilder()
              .setDefinition(jsonAdapter.toWorkflowDefinition(body.get("definition")))
              .setTriggerNodeId(body.path("triggerNodeId").asText())
              .setHttpMethod(body.path("method").asText())
              .setResolvedMode("ASYNC")
              .build();
    } catch (IllegalArgumentException exception) {
      return localError(
          HttpStatus.BAD_REQUEST,
          "INVALID_HTTP_TRIGGER_REQUEST",
          exception.getMessage(),
          browserHeaders);
    }
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.CREATED,
        stub -> HTTPTriggerServiceGrpc.newBlockingStub(stub).createHTTPTrigger(request));
  }

  public WorkflowRuntimeResponse getHTTPTrigger(
      String triggerId, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            HTTPTriggerServiceGrpc.newBlockingStub(stub)
                .getHTTPTrigger(
                    GetHTTPTriggerRequest.newBuilder().setTriggerId(triggerId).build()));
  }

  public WorkflowRuntimeResponse disableHTTPTrigger(
      String triggerId, String companyId, HttpHeaders browserHeaders) {
    return invoke(
        companyId,
        browserHeaders,
        HttpStatus.OK,
        stub ->
            HTTPTriggerServiceGrpc.newBlockingStub(stub)
                .disableHTTPTrigger(
                    DisableHTTPTriggerRequest.newBuilder().setTriggerId(triggerId).build()));
  }

  <T extends com.google.protobuf.Message> WorkflowRuntimeResponse invoke(
      String companyId,
      HttpHeaders browserHeaders,
      HttpStatus successStatus,
      Function<Channel, T> operation) {
    Metadata metadata = metadata(companyId, browserHeaders);
    Channel callChannel =
        ClientInterceptors.intercept(
            channel,
            MetadataUtils.newAttachHeadersInterceptor(metadata),
            new DeadlineInterceptor(properties.requestTimeout().toMillis()));
    try {
      T response = operation.apply(callChannel);
      return response(successStatus, jsonAdapter.toBrowserJson(response), browserHeaders);
    } catch (StatusRuntimeException exception) {
      return mapError(exception, browserHeaders);
    } catch (WorkflowRuntimeMappingException exception) {
      return localError(
          HttpStatus.INTERNAL_SERVER_ERROR,
          "RUNTIME_RESPONSE_MAPPING_FAILED",
          "An unexpected internal error occurred.",
          browserHeaders);
    } catch (IllegalArgumentException exception) {
      return localError(
          HttpStatus.BAD_REQUEST,
          "INVALID_EXECUTION_REQUEST",
          exception.getMessage(),
          browserHeaders);
    }
  }

  private Metadata metadata(String companyId, HttpHeaders browserHeaders) {
    Metadata metadata = new Metadata();
    metadata.put(AUTHORIZATION, "Bearer " + properties.internalServiceToken());
    metadata.put(COMPANY, companyId);
    putSingle(metadata, CORRELATION, browserHeaders, "X-Correlation-ID");
    putSingle(metadata, REQUEST_ID, browserHeaders, "X-Request-ID");
    putSingle(metadata, IDEMPOTENCY, browserHeaders, "Idempotency-Key");
    return metadata;
  }

  private void putSingle(
      Metadata metadata, Metadata.Key<String> key, HttpHeaders headers, String headerName) {
    List<String> values = headers.get(headerName);
    if (values != null && values.size() == 1 && !values.getFirst().isBlank()) {
      metadata.put(key, values.getFirst().trim());
    }
  }

  private WorkflowRuntimeResponse mapError(
      StatusRuntimeException exception, HttpHeaders browserHeaders) {
    Status grpcStatus = StatusProto.fromThrowable(exception);
    io.grpc.Status.Code grpcCode = exception.getStatus().getCode();
    String reason = "";
    ArrayNode details = objectMapper.createArrayNode();
    if (grpcStatus != null) {
      for (Any detail : grpcStatus.getDetailsList()) {
        try {
          if (detail.is(ErrorInfo.class)) {
            reason = detail.unpack(ErrorInfo.class).getReason();
          } else if (detail.is(ValidationIssue.class)) {
            ValidationIssue issue = detail.unpack(ValidationIssue.class);
            ObjectNode mapped = objectMapper.createObjectNode();
            mapped.put("code", issue.getCode());
            mapped.put("field", issue.getField());
            mapped.put("reason", issue.getReason());
            mapped.put("nodeId", issue.getNodeId());
            mapped.put("edgeId", issue.getEdgeId());
            mapped.put("pluginType", issue.getPluginType());
            mapped.put("pluginVersion", issue.getPluginVersion());
            mapped.put("expected", issue.getExpected());
            mapped.put("actual", issue.getActual());
            mapped.set("cyclePath", objectMapper.valueToTree(issue.getCyclePathList()));
            details.add(mapped);
          }
        } catch (Exception ignored) {
          // Unknown or incompatible details are intentionally not exposed.
        }
      }
    }
    HttpStatus status = httpStatus(grpcCode, reason);
    String code = reason.isBlank() ? defaultErrorCode(grpcCode) : reason;
    ObjectNode body = errorBody(status, code, safeMessage(status, code), browserHeaders);
    if (!details.isEmpty()) {
      body.set("details", details);
    }
    return response(status, write(body), browserHeaders);
  }

  private HttpStatus httpStatus(io.grpc.Status.Code code, String reason) {
    return switch (code) {
      case INVALID_ARGUMENT ->
          "WORKFLOW_VALIDATION_FAILED".equals(reason)
              ? HttpStatus.UNPROCESSABLE_ENTITY
              : HttpStatus.BAD_REQUEST;
      case UNAUTHENTICATED -> HttpStatus.UNAUTHORIZED;
      case PERMISSION_DENIED -> HttpStatus.FORBIDDEN;
      case NOT_FOUND -> HttpStatus.NOT_FOUND;
      case ALREADY_EXISTS, FAILED_PRECONDITION -> HttpStatus.CONFLICT;
      case DEADLINE_EXCEEDED -> HttpStatus.GATEWAY_TIMEOUT;
      case UNAVAILABLE, CANCELLED -> HttpStatus.SERVICE_UNAVAILABLE;
      default -> HttpStatus.INTERNAL_SERVER_ERROR;
    };
  }

  private String defaultErrorCode(io.grpc.Status.Code code) {
    return switch (code) {
      case UNAUTHENTICATED -> "UNAUTHORIZED";
      case PERMISSION_DENIED -> "FORBIDDEN";
      case NOT_FOUND -> "NOT_FOUND";
      case ALREADY_EXISTS -> "IDEMPOTENCY_KEY_REUSED";
      case FAILED_PRECONDITION -> "FAILED_PRECONDITION";
      case DEADLINE_EXCEEDED -> "REQUEST_TIMEOUT";
      case UNAVAILABLE, CANCELLED -> "EXECUTION_UNAVAILABLE";
      case INVALID_ARGUMENT -> "INVALID_EXECUTION_REQUEST";
      default -> "INTERNAL_SERVER_ERROR";
    };
  }

  private String safeMessage(HttpStatus status, String code) {
    if ("WORKFLOW_VALIDATION_FAILED".equals(code)) {
      return "Workflow definition failed validation.";
    }
    return switch (status) {
      case BAD_REQUEST -> "The runtime request is invalid.";
      case UNAUTHORIZED -> "Runtime authentication failed.";
      case FORBIDDEN -> "The runtime operation is not permitted.";
      case NOT_FOUND -> "The requested runtime resource was not found.";
      case CONFLICT -> "The runtime operation conflicts with current state.";
      case GATEWAY_TIMEOUT -> "The runtime request exceeded its deadline.";
      case SERVICE_UNAVAILABLE -> "Workflow execution is currently unavailable.";
      default -> "An unexpected internal error occurred.";
    };
  }

  private WorkflowRuntimeResponse localError(
      HttpStatus status, String code, String message, HttpHeaders browserHeaders) {
    return response(
        status, write(errorBody(status, code, message, browserHeaders)), browserHeaders);
  }

  private ObjectNode errorBody(
      HttpStatus status, String code, String message, HttpHeaders browserHeaders) {
    ObjectNode body = objectMapper.createObjectNode();
    body.put("timestamp", Instant.now().toString());
    body.put("status", status.value());
    body.put("code", code);
    body.put("message", message);
    body.put("requestId", browserHeaders.getFirst("X-Request-ID"));
    body.put("correlationId", browserHeaders.getFirst("X-Correlation-ID"));
    return body;
  }

  private WorkflowRuntimeResponse response(
      HttpStatus status, byte[] body, HttpHeaders browserHeaders) {
    HttpHeaders responseHeaders = new HttpHeaders();
    responseHeaders.setContentType(org.springframework.http.MediaType.APPLICATION_JSON);
    copyHeader(browserHeaders, responseHeaders, "X-Correlation-ID");
    copyHeader(browserHeaders, responseHeaders, "X-Request-ID");
    return new WorkflowRuntimeResponse(status, responseHeaders, body);
  }

  private void copyHeader(HttpHeaders source, HttpHeaders target, String name) {
    String value = source.getFirst(name);
    if (value != null && !value.isBlank()) {
      target.set(name, value);
    }
  }

  private byte[] write(JsonNode value) {
    try {
      return objectMapper.writeValueAsBytes(value);
    } catch (Exception exception) {
      throw new IllegalStateException("Runtime response could not be encoded", exception);
    }
  }

  private GetExecutionRequest executionRequest(String executionId) {
    return GetExecutionRequest.newBuilder().setExecutionId(executionId).build();
  }

  ListExecutionsRequest listExecutionsRequest(MultiValueMap<String, String> query) {
    return ListExecutionsRequest.newBuilder()
        .setWorkflowId(first(query, "workflowId"))
        .setStatus(first(query, "status"))
        .setCursor(cursor(query))
        .setLimit(limit(query, MAXIMUM_EXECUTION_PAGE_SIZE))
        .build();
  }

  ListExecutionResourceRequest resourceRequest(
      String executionId, MultiValueMap<String, String> query) {
    return ListExecutionResourceRequest.newBuilder()
        .setExecutionId(executionId)
        .setCursor(cursor(query))
        .setLimit(limit(query, MAXIMUM_RESOURCE_PAGE_SIZE))
        .build();
  }

  private String first(MultiValueMap<String, String> query, String key) {
    String value = query.getFirst(key);
    return value == null ? "" : value;
  }

  private int limit(MultiValueMap<String, String> query, int maximum) {
    List<String> values = query.get("limit");
    if (values == null || values.isEmpty()) {
      return 0;
    }
    if (values.size() != 1 || values.getFirst() == null || values.getFirst().isBlank()) {
      throw new IllegalArgumentException("limit must be specified once");
    }
    String value = values.getFirst().trim();
    try {
      int parsed = Integer.parseInt(value);
      if (parsed < 1 || parsed > maximum) {
        throw new IllegalArgumentException("limit must be between 1 and " + maximum);
      }
      return parsed;
    } catch (NumberFormatException exception) {
      throw new IllegalArgumentException("limit must be an integer", exception);
    }
  }

  private String cursor(MultiValueMap<String, String> query) {
    List<String> values = query.get("after");
    if (values == null || values.isEmpty()) {
      return "";
    }
    if (values.size() != 1 || values.getFirst() == null || values.getFirst().isBlank()) {
      throw new IllegalArgumentException("after cursor must be specified once");
    }
    return values.getFirst().trim();
  }

  private record DeadlineInterceptor(long timeoutMillis) implements ClientInterceptor {
    @Override
    public <ReqT, RespT> ClientCall<ReqT, RespT> interceptCall(
        MethodDescriptor<ReqT, RespT> method, CallOptions callOptions, Channel next) {
      return next.newCall(
          method,
          callOptions.withDeadlineAfter(timeoutMillis, java.util.concurrent.TimeUnit.MILLISECONDS));
    }
  }
}
