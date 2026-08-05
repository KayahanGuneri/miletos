package com.miletos.features.workflow.service;

import com.miletos.features.workflow.repository.entity.Workflow;
import org.springframework.http.HttpHeaders;

/**
 * Runs the authoritative runtime graph and plugin validation for a persisted workflow. The workflow
 * feature depends on this narrow contract so that protobuf and gRPC stay inside the workflowruntime
 * feature.
 */
public interface WorkflowRuntimeValidationGateway {

  void validateWorkflowDefinition(Workflow workflow, Long companyId, HttpHeaders browserHeaders);
}
