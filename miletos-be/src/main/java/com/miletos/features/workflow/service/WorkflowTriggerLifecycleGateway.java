package com.miletos.features.workflow.service;

import com.miletos.features.workflow.repository.entity.Workflow;
import org.springframework.http.HttpHeaders;

public interface WorkflowTriggerLifecycleGateway {

  void activateWorkflowSources(Workflow workflow, Long companyId, HttpHeaders browserHeaders);

  void disableWorkflowTriggers(Long workflowId, Long companyId, HttpHeaders browserHeaders);
}
