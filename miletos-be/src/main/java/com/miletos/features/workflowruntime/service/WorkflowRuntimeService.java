package com.miletos.features.workflowruntime.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.company.exception.CompanyNotFoundException;
import com.miletos.features.company.exception.CompanyOperationForbiddenException;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeGrpcClient;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeResponse;
import com.miletos.features.workflowruntime.exception.CompanyContextRequiredException;
import com.miletos.security.AuthenticatedActorResolver;
import java.util.List;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpHeaders;
import org.springframework.stereotype.Service;
import org.springframework.util.MultiValueMap;

@Service
public class WorkflowRuntimeService {

  private static final String COMPANY_HEADER = "X-Miletos-Company-ID";
  private static final Logger LOGGER = LoggerFactory.getLogger(WorkflowRuntimeService.class);

  private final AuthenticatedActorResolver authenticatedActorResolver;
  private final CompanyRepository companyRepository;
  private final WorkflowRuntimeGrpcClient runtimeClient;
  private final ExecutionPolicyResolver executionPolicyResolver;

  @Autowired
  public WorkflowRuntimeService(
      AuthenticatedActorResolver authenticatedActorResolver,
      CompanyRepository companyRepository,
      WorkflowRuntimeGrpcClient runtimeClient,
      ExecutionPolicyResolver executionPolicyResolver) {
    this.authenticatedActorResolver = authenticatedActorResolver;
    this.companyRepository = companyRepository;
    this.runtimeClient = runtimeClient;
    this.executionPolicyResolver = executionPolicyResolver;
  }

  public WorkflowRuntimeService(
      AuthenticatedActorResolver authenticatedActorResolver,
      WorkflowRuntimeGrpcClient runtimeClient) {
    this(authenticatedActorResolver, null, runtimeClient, new ExecutionPolicyResolver());
  }

  public WorkflowRuntimeResponse getPlugins(String actorEmail, HttpHeaders browserHeaders) {
    return runtimeClient.getPlugins(resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse executeSync(
      String actorEmail, JsonNode body, HttpHeaders browserHeaders) {
    return execute(
        actorEmail,
        body,
        browserHeaders,
        ExecutionModePolicy.SYNC,
        TrustedTriggerType.MANUAL_DIRECT);
  }

  public WorkflowRuntimeResponse executeAsync(
      String actorEmail, JsonNode body, HttpHeaders browserHeaders) {
    return execute(
        actorEmail,
        body,
        browserHeaders,
        ExecutionModePolicy.ASYNC,
        TrustedTriggerType.MANUAL_DIRECT);
  }

  public WorkflowRuntimeResponse execute(
      String actorEmail, JsonNode body, HttpHeaders browserHeaders) {
    return execute(
        actorEmail,
        body,
        browserHeaders,
        ExecutionModePolicy.AUTO,
        TrustedTriggerType.MANUAL_DIRECT);
  }

  WorkflowRuntimeResponse execute(
      String actorEmail,
      JsonNode body,
      HttpHeaders browserHeaders,
      ExecutionModePolicy policy,
      TrustedTriggerType triggerType) {
    String companyId = resolveCompanyId(actorEmail, browserHeaders);
    ResolvedExecutionMode mode = executionPolicyResolver.resolve(policy, triggerType);
    LOGGER.info(
        "Workflow execution mode resolved: companyId={}, triggerType={}, mode={}",
        companyId,
        triggerType,
        mode);
    return mode == ResolvedExecutionMode.SYNC
        ? runtimeClient.executeSync(body, companyId, browserHeaders)
        : runtimeClient.executeAsync(body, companyId, browserHeaders);
  }

  public WorkflowRuntimeResponse recoverExecution(
      String actorEmail, String executionId, HttpHeaders browserHeaders) {
    return runtimeClient.recoverExecution(
        executionId, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse listExecutions(
      String actorEmail, MultiValueMap<String, String> query, HttpHeaders browserHeaders) {
    return runtimeClient.listExecutions(
        query, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse getExecution(
      String actorEmail, String executionId, HttpHeaders browserHeaders) {
    return runtimeClient.getExecution(
        executionId, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse getExecutionDefinition(
      String actorEmail, String executionId, HttpHeaders browserHeaders) {
    return runtimeClient.getExecutionDefinition(
        executionId, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse getExecutionNodes(
      String actorEmail,
      String executionId,
      MultiValueMap<String, String> query,
      HttpHeaders browserHeaders) {
    return runtimeClient.getExecutionNodes(
        executionId, query, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse getExecutionEvents(
      String actorEmail,
      String executionId,
      MultiValueMap<String, String> query,
      HttpHeaders browserHeaders) {
    return runtimeClient.getExecutionEvents(
        executionId, query, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse getExecutionLogs(
      String actorEmail,
      String executionId,
      MultiValueMap<String, String> query,
      HttpHeaders browserHeaders) {
    return runtimeClient.getExecutionLogs(
        executionId, query, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse getExecutionErrors(
      String actorEmail,
      String executionId,
      MultiValueMap<String, String> query,
      HttpHeaders browserHeaders) {
    return runtimeClient.getExecutionErrors(
        executionId, query, resolveCompanyId(actorEmail, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse getHTTPTrigger(
      User user, String triggerId, HttpHeaders browserHeaders) {
    return runtimeClient.getHTTPTrigger(
        triggerId, resolveCompanyId(user, browserHeaders), browserHeaders);
  }

  public WorkflowRuntimeResponse disableHTTPTrigger(
      User user, String triggerId, HttpHeaders browserHeaders) {
    return runtimeClient.disableHTTPTrigger(
        triggerId, resolveCompanyId(user, browserHeaders), browserHeaders);
  }

  private String resolveCompanyId(String actorEmail, HttpHeaders browserHeaders) {
    return resolveCompanyId(authenticatedActorResolver.resolve(actorEmail), browserHeaders);
  }

  private String resolveCompanyId(User authenticatedUser, HttpHeaders browserHeaders) {
    List<String> headerValues = browserHeaders.get(COMPANY_HEADER);
    List<String> requestedCompanyIds =
        (headerValues == null ? List.<String>of() : headerValues)
            .stream().map(String::trim).filter(value -> !value.isBlank()).toList();

    if (!authenticatedUser.isSuperAdmin()) {
      if (authenticatedUser.getCompany() == null
          || authenticatedUser.getCompany().getId() == null) {
        throw new CompanyContextRequiredException();
      }

      String authenticatedCompanyId = authenticatedUser.getCompany().getId().toString();
      for (String requestedCompanyId : requestedCompanyIds) {
        if (!authenticatedCompanyId.equals(requestedCompanyId)) {
          throw new CompanyOperationForbiddenException();
        }
      }

      return authenticatedCompanyId;
    }

    if (requestedCompanyIds.isEmpty()) {
      throw new CompanyContextRequiredException();
    }
    if (requestedCompanyIds.size() != 1) {
      throw new CompanyOperationForbiddenException();
    }
    String requestedCompanyId = requestedCompanyIds.getFirst();

    Long companyId;
    try {
      companyId = Long.valueOf(requestedCompanyId);
    } catch (NumberFormatException exception) {
      throw new CompanyNotFoundException();
    }

    if (companyId <= 0 || companyRepository == null || !companyRepository.existsById(companyId)) {
      throw new CompanyNotFoundException();
    }

    return companyId.toString();
  }
}
