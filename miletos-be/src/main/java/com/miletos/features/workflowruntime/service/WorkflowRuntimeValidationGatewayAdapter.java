package com.miletos.features.workflowruntime.service;

import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.service.WorkflowRuntimeValidationGateway;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeGrpcClient;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpHeaders;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class WorkflowRuntimeValidationGatewayAdapter implements WorkflowRuntimeValidationGateway {

  private final TrustedWorkflowDefinitionMapper definitionMapper;
  private final WorkflowRuntimeGrpcClient runtimeClient;

  @Override
  public void validateWorkflowDefinition(
      Workflow workflow, Long companyId, HttpHeaders browserHeaders) {
    runtimeClient.validateWorkflow(
        definitionMapper.map(workflow).definition(), companyId.toString(), browserHeaders);
  }
}
