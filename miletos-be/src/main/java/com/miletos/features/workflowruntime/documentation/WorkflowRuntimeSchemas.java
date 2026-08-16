package com.miletos.features.workflowruntime.documentation;

import io.swagger.v3.oas.annotations.media.Schema;
import java.util.List;
import java.util.Map;

public final class WorkflowRuntimeSchemas {

  private WorkflowRuntimeSchemas() {}

  public record ExecutionRequest(
      @Schema(requiredMode = Schema.RequiredMode.REQUIRED) WorkflowDefinition definition,
      Map<String, Object> initialVariables) {}

  public record WorkflowDefinition(
      @Schema(example = "order-approval") String id,
      @Schema(example = "Order approval") String name,
      @Schema(example = "1") long revision,
      List<WorkflowNode> nodes,
      List<WorkflowEdge> edges,
      Map<String, Object> metadata) {}

  public record WorkflowNode(
      String id,
      @Schema(example = "core.pass-through") String pluginType,
      @Schema(example = "v1") String pluginVersion,
      Map<String, Object> configuration,
      NodePosition position) {}

  public record NodePosition(double x, double y) {}

  public record WorkflowEdge(
      String id,
      String sourceNodeId,
      String sourceOutputPort,
      String targetNodeId,
      String targetInputPort) {}

  public record ExecutionResponse(
      String executionId,
      String workflowId,
      long workflowRevision,
      String mode,
      String status,
      String correlationId,
      String createdAt,
      String startedAt,
      String finishedAt,
      Integer scheduledRoots,
      boolean replayed) {}

  public record ExecutionPage(List<ExecutionSummaryResponse> items, String next, boolean hasNext) {}

  public record ExecutionSummaryResponse(
      String executionId,
      String workflowId,
      long workflowRevision,
      String mode,
      String status,
      String correlationId,
      String createdAt,
      String validatingAt,
      String queuedAt,
      String startedAt,
      String finishedAt,
      String updatedAt,
      boolean isStalled) {}

  public record DefinitionResponse(
      String executionId,
      String snapshotId,
      String workflowId,
      long workflowRevision,
      String workflowName,
      WorkflowDefinition definition,
      String createdAt) {}

  public record NodeExecution(
      String nodeExecutionId,
      String workflowExecutionId,
      String nodeId,
      String pluginType,
      String pluginVersion,
      String status,
      int attempt,
      String createdAt,
      String readyAt,
      String queuedAt,
      String startedAt,
      String finishedAt,
      String nextAttemptAt,
      String updatedAt,
      Map<String, Object> configuration,
      Map<String, Object> inputSummary,
      Map<String, Object> outputSummary,
      Map<String, Object> failureSummary) {}

  public record NodeExecutionPage(List<NodeExecution> items, String next, boolean hasNext) {}

  public record ExecutionEvent(
      String eventId,
      String workflowExecutionId,
      String nodeExecutionId,
      long sequenceNumber,
      String type,
      String previousStatus,
      String newStatus,
      String correlationId,
      String causationId,
      String safeMessage,
      Map<String, Object> metadata,
      String createdAt) {}

  public record ExecutionEventPage(List<ExecutionEvent> items, String next, boolean hasNext) {}

  public record ExecutionLog(
      String logId,
      String workflowExecutionId,
      String nodeExecutionId,
      long sequenceNumber,
      String level,
      String message,
      Map<String, Object> metadata,
      String createdAt) {}

  public record ExecutionLogPage(List<ExecutionLog> items, String next, boolean hasNext) {}

  public record ExecutionError(
      String errorId,
      String workflowExecutionId,
      String nodeExecutionId,
      String relatedEventId,
      String category,
      String code,
      String safeMessage,
      boolean retryable,
      Map<String, Object> details,
      String createdAt) {}

  public record ExecutionErrorPage(List<ExecutionError> items, String next, boolean hasNext) {}

  public record PluginPort(String name, String displayName, String description) {}

  public record EdgeConstraint(int minimum, Integer maximum, boolean unlimited) {}

  public record Plugin(
      String type,
      String version,
      String displayName,
      String description,
      String inputMode,
      boolean acceptsInitialVariables,
      List<PluginPort> inputPorts,
      List<PluginPort> outputPorts,
      EdgeConstraint inputEdgeConstraint,
      EdgeConstraint outputEdgeConstraint) {}

  public record PluginPage(List<Plugin> items, int count) {}

  public record RecoveryResponse(
      String sourceExecutionId,
      String recoveryExecutionId,
      String status,
      int preservedNodeCount,
      int scheduledNodeCount,
      int resetNodeCount,
      String createdAt,
      boolean replayed) {}

  public record CreateHTTPTriggerRequest(
      @Schema(requiredMode = Schema.RequiredMode.REQUIRED) WorkflowDefinition definition,
      @Schema(requiredMode = Schema.RequiredMode.REQUIRED) String triggerNodeId,
      @Schema(
              requiredMode = Schema.RequiredMode.REQUIRED,
              allowableValues = {"GET", "POST", "PUT", "PATCH", "DELETE"})
          String method) {}

  public record HTTPTriggerResponse(
      String triggerId,
      String workflowId,
      long workflowRevision,
      String snapshotId,
      String triggerNodeId,
      String httpMethod,
      String status,
      String resolvedMode,
      String createdAt,
      String updatedAt,
      String disabledAt) {}

  public record CreateHTTPTriggerResponse(
      HTTPTriggerResponse trigger,
      @Schema(
              description =
                  "One-time public webhook URL. " + "It is omitted from Get and Disable responses.")
          String publicUrl) {}

  public record ApiError(
      String timestamp,
      int status,
      @Schema(
              description = "Stable API error code.",
              allowableValues = {
                "WORKFLOW_VALIDATION_FAILED",
                "NOT_FOUND",
                "IDEMPOTENCY_KEY_REUSED",
                "INVALID_EXECUTION_ORIGIN",
                "RECOVERY_ALREADY_EXISTS",
                "INTERNAL_SERVER_ERROR"
              })
          String code,
      String message,
      String requestId,
      String correlationId,
      List<ValidationIssue> details) {}

  public record ValidationIssue(
      @Schema(
              allowableValues = {
                "PLUGIN_NOT_FOUND",
                "UNKNOWN_OUTPUT_PORT",
                "UNKNOWN_INPUT_PORT",
                "INPUT_EDGE_COUNT_BELOW_MINIMUM",
                "INPUT_EDGE_COUNT_ABOVE_MAXIMUM",
                "OUTPUT_EDGE_COUNT_BELOW_MINIMUM",
                "OUTPUT_EDGE_COUNT_ABOVE_MAXIMUM",
                "INVALID_PLUGIN_CONFIGURATION",
                "CYCLE_DETECTED",
                "INITIAL_VARIABLES_NOT_ACCEPTED",
                "INVALID_WORKFLOW_DEFINITION"
              })
          String code,
      String field,
      String reason,
      String nodeId,
      String edgeId,
      String pluginType,
      String pluginVersion,
      String expected,
      String actual,
      List<String> cyclePath) {}
}
