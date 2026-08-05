package com.miletos.features.workflowruntime.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.miletos.features.workflow.exception.InvalidWorkflowDefinitionException;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.service.WorkflowDefinitionPolicy;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeGrpcJsonAdapter;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class TrustedWorkflowDefinitionMapper {

  private final WorkflowRuntimeGrpcJsonAdapter jsonAdapter;
  private final WorkflowDefinitionPolicy workflowDefinitionPolicy;

  public TrustedWorkflowDefinition map(Workflow workflow) {
    JsonNode persistedDefinition = workflow.getDefinitionJson();
    if (!(persistedDefinition instanceof ObjectNode definition)) {
      throw new InvalidWorkflowDefinitionException();
    }

    ObjectNode trustedDefinition =
        (ObjectNode) workflowDefinitionPolicy.validateAndNormalizeDefinition(definition).deepCopy();
    trustedDefinition.put("workflowId", workflow.getId().toString());
    trustedDefinition.put("name", workflow.getName());
    trustedDefinition.put("workflowRevision", workflow.getRevision());
    try {
      return new TrustedWorkflowDefinition(jsonAdapter.toWorkflowDefinition(trustedDefinition));
    } catch (IllegalArgumentException exception) {
      throw new InvalidWorkflowDefinitionException();
    }
  }
}
