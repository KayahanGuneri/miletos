package com.miletos.features.workflow.service;

import org.springframework.http.HttpHeaders;

public interface WorkflowTriggerLifecycleGateway {

  void disableWorkflowTriggers(Long workflowId, Long companyId, HttpHeaders browserHeaders);
}
