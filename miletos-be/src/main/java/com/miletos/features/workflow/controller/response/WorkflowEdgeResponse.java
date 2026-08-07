package com.miletos.features.workflow.controller.response;

public record WorkflowEdgeResponse(
                String edgeId,
                String sourceNodeId,
                String sourceOutputPort,
                String targetNodeId,
                String targetInputPort) {
}
