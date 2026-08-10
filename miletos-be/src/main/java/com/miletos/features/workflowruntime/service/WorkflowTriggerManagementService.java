package com.miletos.features.workflowruntime.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.JsonNodeFactory;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflow.exception.WorkflowInvalidStateException;
import com.miletos.features.workflow.exception.WorkflowNotFoundException;
import com.miletos.features.workflow.repository.WorkflowRepository;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeGrpcClient;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeResponse;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeDomainException;
import com.miletos.features.workflowruntime.grpc.generated.Plugin;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowNode;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class WorkflowTriggerManagementService {

  private static final Set<String> HTTP_METHODS = Set.of("GET", "POST", "PUT", "PATCH", "DELETE");

  private final WorkflowRepository workflowRepository;
  private final TrustedWorkflowDefinitionMapper definitionMapper;
  private final WorkflowRuntimeGrpcClient runtimeClient;
  private final ExecutionPolicyResolver executionPolicyResolver;

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse createHTTPTrigger(
      User user, Long workflowId, String triggerNodeId, HttpHeaders browserHeaders) {
    ActiveWorkflow activeWorkflow = loadActiveWorkflow(user, workflowId);
    List<WorkflowNode> rootNodes = activeWorkflow.trusted().roots();
    if (rootNodes.size() != 1) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_ROOT_REQUIRED, HttpStatus.UNPROCESSABLE_ENTITY);
    }
    WorkflowNode triggerNode =
        activeWorkflow
            .trusted()
            .findNode(triggerNodeId)
            .orElseThrow(
                () ->
                    domain(ErrorCode.WORKFLOW_TRIGGER_NODE_NOT_FOUND, HttpStatus.BAD_REQUEST));
    if (!rootNodes.getFirst().getNodeId().equals(triggerNode.getNodeId())) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_ROOT_REQUIRED, HttpStatus.UNPROCESSABLE_ENTITY);
    }
    if (!isCompatibleTriggerPlugin(
        triggerNode, "HTTP_WEBHOOK", activeWorkflow.companyId(), browserHeaders)) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_TYPE_INVALID, HttpStatus.UNPROCESSABLE_ENTITY);
    }
    String method = resolveHTTPMethod(activeWorkflow.trusted().configuration(triggerNode.getNodeId()));
    if (!isAsyncTrigger(TrustedTriggerType.HTTP_WEBHOOK)) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_TYPE_INVALID, HttpStatus.CONFLICT);
    }
    return runtimeClient.createHTTPTrigger(
        activeWorkflow.trusted().definition(),
        triggerNode.getNodeId(),
        method,
        activeWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse getHTTPTrigger(
      User user, Long workflowId, String triggerId, HttpHeaders browserHeaders) {
    OwnedWorkflow ownedWorkflow = loadOwnedWorkflow(user, workflowId);
    return runtimeClient.getHTTPTrigger(
        triggerId,
        ownedWorkflow.workflow().getId().toString(),
        ownedWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse getActiveHTTPTrigger(
      User user, Long workflowId, HttpHeaders browserHeaders) {
    OwnedWorkflow ownedWorkflow = loadOwnedWorkflow(user, workflowId);
    return runtimeClient.getActiveHTTPTriggerByWorkflow(
        ownedWorkflow.workflow().getId().toString(),
        ownedWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse disableHTTPTrigger(
      User user, Long workflowId, String triggerId, HttpHeaders browserHeaders) {
    OwnedWorkflow ownedWorkflow = loadOwnedWorkflow(user, workflowId);
    return runtimeClient.disableHTTPTrigger(
        triggerId,
        ownedWorkflow.workflow().getId().toString(),
        ownedWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse createCronTrigger(
      User user, Long workflowId, String triggerNodeId, HttpHeaders browserHeaders) {
    ActiveWorkflow activeWorkflow = loadActiveWorkflow(user, workflowId);
    List<WorkflowNode> rootNodes = activeWorkflow.trusted().roots();
    if (rootNodes.size() != 1) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_ROOT_REQUIRED, HttpStatus.UNPROCESSABLE_ENTITY);
    }
    WorkflowNode triggerNode =
        activeWorkflow
            .trusted()
            .findNode(triggerNodeId)
            .orElseThrow(
                () ->
                    domain(ErrorCode.WORKFLOW_TRIGGER_NODE_NOT_FOUND, HttpStatus.BAD_REQUEST));
    if (!rootNodes.getFirst().getNodeId().equals(triggerNode.getNodeId())) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_ROOT_REQUIRED, HttpStatus.UNPROCESSABLE_ENTITY);
    }
    if (!isCompatibleTriggerPlugin(
        triggerNode, "CRON", activeWorkflow.companyId(), browserHeaders)) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_TYPE_INVALID, HttpStatus.UNPROCESSABLE_ENTITY);
    }
    CronConfiguration cronConfiguration =
        this.cronConfiguration(activeWorkflow.trusted().configuration(triggerNode.getNodeId()));
    if (!isAsyncTrigger(TrustedTriggerType.CRON)) {
      throw domain(ErrorCode.WORKFLOW_TRIGGER_TYPE_INVALID, HttpStatus.CONFLICT);
    }
    return runtimeClient.createCronTrigger(
        activeWorkflow.trusted().definition(),
        triggerNode.getNodeId(),
        cronConfiguration.expression(),
        cronConfiguration.timezone(),
        activeWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse getCronTrigger(
      User user, Long workflowId, String triggerId, HttpHeaders browserHeaders) {
    OwnedWorkflow ownedWorkflow = loadOwnedWorkflow(user, workflowId);
    return runtimeClient.getCronTrigger(
        triggerId,
        ownedWorkflow.workflow().getId().toString(),
        ownedWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse getActiveCronTrigger(
      User user, Long workflowId, HttpHeaders browserHeaders) {
    OwnedWorkflow ownedWorkflow = loadOwnedWorkflow(user, workflowId);
    return runtimeClient.getActiveCronTriggerByWorkflow(
        ownedWorkflow.workflow().getId().toString(),
        ownedWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse disableCronTrigger(
      User user, Long workflowId, String triggerId, HttpHeaders browserHeaders) {
    OwnedWorkflow ownedWorkflow = loadOwnedWorkflow(user, workflowId);
    return runtimeClient.disableCronTrigger(
        triggerId,
        ownedWorkflow.workflow().getId().toString(),
        ownedWorkflow.companyId(),
        browserHeaders);
  }

  @Transactional(readOnly = true)
  public WorkflowRuntimeResponse runWorkflow(
      User user, Long workflowId, JsonNode body, HttpHeaders browserHeaders) {
    validateIdempotencyKey(browserHeaders);
    JsonNode initialVariables = initialVariables(body);
    ActiveWorkflow activeWorkflow = loadActiveWorkflow(user, workflowId);
    ResolvedExecutionMode mode =
        executionPolicyResolver.resolve(ExecutionModePolicy.AUTO, TrustedTriggerType.MANUAL_DIRECT);
    return runtimeClient.executePersistedWorkflow(
        activeWorkflow.trusted().definition(),
        initialVariables,
        mode == ResolvedExecutionMode.SYNC,
        activeWorkflow.companyId(),
        browserHeaders);
  }

  private ActiveWorkflow loadActiveWorkflow(User user, Long workflowId) {
    OwnedWorkflow ownedWorkflow = loadOwnedWorkflow(user, workflowId);
    if (ownedWorkflow.workflow().getStatus() != WorkflowStatus.ACTIVE) {
      throw new WorkflowInvalidStateException();
    }
    return new ActiveWorkflow(
        definitionMapper.map(ownedWorkflow.workflow()), ownedWorkflow.companyId());
  }

  private OwnedWorkflow loadOwnedWorkflow(User user, Long workflowId) {
    Company company = user.getCompany();
    Workflow workflow =
        workflowRepository
            .findByIdAndCompany(workflowId, company)
            .orElseThrow(WorkflowNotFoundException::new);
    return new OwnedWorkflow(workflow, company.getId().toString());
  }

  private String resolveHTTPMethod(JsonNode configuration) {
    String method = configuration.path("method").asText("").trim().toUpperCase(Locale.ROOT);
    if (!HTTP_METHODS.contains(method)) {
      throw domain(ErrorCode.HTTP_TRIGGER_CONFIGURATION_INVALID, HttpStatus.BAD_REQUEST);
    }
    return method;
  }

  private CronConfiguration cronConfiguration(JsonNode configuration) {
    String expression = configuration.path("expression").asText("").trim();
    String timezone = configuration.path("timezone").asText("").trim();
    if (expression.isBlank()) {
      throw domain(ErrorCode.CRON_TRIGGER_CONFIGURATION_INVALID, HttpStatus.BAD_REQUEST);
    }
    if (timezone.isBlank()) {
      timezone = "UTC";
    }
    return new CronConfiguration(expression, timezone);
  }

  private boolean isCompatibleTriggerPlugin(
      WorkflowNode triggerNode,
      String requiredOrigin,
      String companyId,
      HttpHeaders browserHeaders) {
    Plugin plugin =
        runtimeClient.listPluginDescriptors(companyId, browserHeaders).stream()
            .filter(candidate -> candidate.getType().equals(triggerNode.getPluginType()))
            .filter(candidate -> candidate.getVersion().equals(triggerNode.getPluginVersion()))
            .findFirst()
            .orElse(null);
    return plugin != null && plugin.getAllowedRootOriginsList().contains(requiredOrigin);
  }

  private boolean isAsyncTrigger(TrustedTriggerType triggerType) {
    return executionPolicyResolver.resolve(ExecutionModePolicy.AUTO, triggerType)
        == ResolvedExecutionMode.ASYNC;
  }

  private JsonNode initialVariables(JsonNode body) {
    if (body == null) {
      return JsonNodeFactory.instance.objectNode();
    }
    if (!body.isObject()) {
      throw domain(ErrorCode.MANUAL_WORKFLOW_EXECUTION_INVALID, HttpStatus.BAD_REQUEST);
    }
    if (!body.has("initialVariables")) {
      return JsonNodeFactory.instance.objectNode();
    }
    JsonNode initialVariables = body.get("initialVariables");
    if (initialVariables == null || !initialVariables.isObject()) {
      throw domain(ErrorCode.MANUAL_WORKFLOW_EXECUTION_INVALID, HttpStatus.BAD_REQUEST);
    }
    return initialVariables.deepCopy();
  }

  private void validateIdempotencyKey(HttpHeaders browserHeaders) {
    List<String> values = browserHeaders.get("Idempotency-Key");
    if (values == null || values.size() != 1) {
      throw domain(ErrorCode.MANUAL_WORKFLOW_EXECUTION_INVALID, HttpStatus.BAD_REQUEST);
    }
    String key = values.getFirst() == null ? "" : values.getFirst().trim();
    if (key.isBlank() || key.codePointCount(0, key.length()) > 255) {
      throw domain(ErrorCode.MANUAL_WORKFLOW_EXECUTION_INVALID, HttpStatus.BAD_REQUEST);
    }
  }

  private WorkflowRuntimeDomainException domain(ErrorCode code, HttpStatus status) {
    return new WorkflowRuntimeDomainException(code, status);
  }

  private record OwnedWorkflow(Workflow workflow, String companyId) {}

  private record ActiveWorkflow(TrustedWorkflowDefinition trusted, String companyId) {}

  private record CronConfiguration(String expression, String timezone) {}
}
