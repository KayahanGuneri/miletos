package com.miletos.features.workflowruntime.service;

import com.miletos.features.workflow.service.WorkflowTriggerLifecycleGateway;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeGrpcClient;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpHeaders;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class WorkflowTriggerLifecycleGatewayAdapter implements WorkflowTriggerLifecycleGateway {

  private final WorkflowRuntimeGrpcClient runtimeClient;

  @Override
  public void disableWorkflowTriggers(Long workflowId, Long companyId, HttpHeaders browserHeaders) {
    runtimeClient.disableWorkflowTriggers(
        workflowId.toString(), companyId.toString(), browserHeaders);
  }
}
