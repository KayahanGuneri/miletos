package com.miletos.features.workflowruntime.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.JsonNodeFactory;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowDefinition;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowNode;
import java.util.List;
import java.util.Optional;
import java.util.Set;

public record TrustedWorkflowDefinition(WorkflowDefinition definition) {

  public List<WorkflowNode> roots() {
    Set<String> targetedNodeIds =
        definition.getEdgesList().stream()
            .map(edge -> edge.getTargetNodeId())
            .collect(java.util.stream.Collectors.toUnmodifiableSet());
    return definition.getNodesList().stream()
        .filter(node -> !targetedNodeIds.contains(node.getNodeId()))
        .toList();
  }

  public Optional<WorkflowNode> findNode(String nodeId) {
    return definition.getNodesList().stream()
        .filter(node -> node.getNodeId().equals(nodeId))
        .findFirst();
  }

  public JsonNode configuration(String nodeId) {
    return findNode(nodeId)
        .map(WorkflowNode::getConfiguration)
        .map(com.miletos.features.workflowruntime.client.ProtobufValueConverter::struct)
        .orElseGet(JsonNodeFactory.instance::objectNode);
  }
}
