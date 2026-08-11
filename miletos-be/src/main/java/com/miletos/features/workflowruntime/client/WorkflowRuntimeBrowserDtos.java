package com.miletos.features.workflowruntime.client;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;

final class WorkflowRuntimeBrowserDtos {

  private WorkflowRuntimeBrowserDtos() {}

  record Port(String name, String displayName, String description) {}

  record EdgeConstraint(int minimum, Integer maximum, boolean unlimited) {}

  record Plugin(
      String type,
      String version,
      String displayName,
      String description,
      String inputMode,
      boolean acceptsInitialVariables,
      List<Port> inputPorts,
      List<Port> outputPorts,
      EdgeConstraint inputEdgeConstraint,
      EdgeConstraint outputEdgeConstraint,
      List<String> allowedRootOrigins,
      String contextProvider) {}

  record PluginPage(List<Plugin> items, int count) {}

  record NodePosition(double x, double y) {}

  record WorkflowNode(
      String id,
      String pluginType,
      String pluginVersion,
      JsonNode configuration,
      NodePosition position) {}

  record WorkflowEdge(
      String id,
      String sourceNodeId,
      String sourceOutputPort,
      String targetNodeId,
      String targetInputPort) {}

  record WorkflowDefinition(
      String id,
      String name,
      long revision,
      List<WorkflowNode> nodes,
      List<WorkflowEdge> edges,
      JsonNode metadata) {}

  record ExecutionResponse(
      String executionId,
      String workflowId,
      long workflowRevision,
      String snapshotId,
      String mode,
      String status,
      String executionOrigin,
      String correlationId,
      String requestId,
      String createdAt,
      String startedAt,
      String finishedAt,
      int scheduledRoots,
      boolean replayed,
      JsonNode terminalOutputs,
      JsonNode failureSummary) {}

  record ExecutionSummary(
      String executionId,
      String workflowId,
      long workflowRevision,
      String snapshotId,
      String mode,
      String status,
      String executionOrigin,
      String correlationId,
      String createdAt,
      String validatingAt,
      String queuedAt,
      String startedAt,
      String finishedAt,
      String updatedAt,
      JsonNode terminalOutputs,
      JsonNode failureSummary,
      boolean isStalled) {}

  record ExecutionPage(List<ExecutionSummary> items, String next, boolean hasNext) {}

  record ExecutionDefinition(
      String executionId,
      String snapshotId,
      String workflowId,
      long workflowRevision,
      String workflowName,
      WorkflowDefinition definition,
      String createdAt) {}

  record NodeExecution(
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
      JsonNode inputSummary,
      JsonNode outputSummary,
      JsonNode failureSummary,
      JsonNode configuration) {}

  record NodeExecutionPage(List<NodeExecution> items, String next, boolean hasNext) {}

  record ExecutionEvent(
      String eventId,
      String workflowExecutionId,
      String nodeExecutionId,
      String sequenceNumber,
      String type,
      String previousStatus,
      String newStatus,
      String correlationId,
      String causationId,
      String safeMessage,
      JsonNode metadata,
      String createdAt) {}

  record ExecutionEventPage(List<ExecutionEvent> items, String next, boolean hasNext) {}

  record ExecutionLog(
      String logId,
      String workflowExecutionId,
      String nodeExecutionId,
      String sequenceNumber,
      String level,
      String message,
      JsonNode metadata,
      String createdAt) {}

  record ExecutionLogPage(List<ExecutionLog> items, String next, boolean hasNext) {}

  record ExecutionError(
      String errorId,
      String workflowExecutionId,
      String nodeExecutionId,
      String relatedEventId,
      String category,
      String code,
      String safeMessage,
      boolean retryable,
      JsonNode details,
      String createdAt) {}

  record ExecutionErrorPage(List<ExecutionError> items, String next, boolean hasNext) {}

  record RecoveryResponse(
      String sourceExecutionId,
      String recoveryExecutionId,
      String status,
      int preservedNodeCount,
      int scheduledNodeCount,
      int resetNodeCount,
      String createdAt,
      boolean replayed) {}

  record HTTPTriggerResponse(
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

  record CreateHTTPTriggerResponse(HTTPTriggerResponse trigger, String publicUrl) {}

  record CronTriggerResponse(
      String triggerId,
      String workflowId,
      long workflowRevision,
      String snapshotId,
      String triggerNodeId,
      String cronExpression,
      String timezone,
      String status,
      String nextFireAt,
      String lastScheduledAt,
      String lastFiredAt,
      String createdAt,
      String updatedAt,
      String disabledAt) {}

  record CreateCronTriggerResponse(CronTriggerResponse trigger) {}
}
