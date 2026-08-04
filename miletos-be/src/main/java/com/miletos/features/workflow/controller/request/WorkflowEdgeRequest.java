package com.miletos.features.workflow.controller.request;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public record WorkflowEdgeRequest(
                @NotBlank @Size(max = 120) String edgeId,
                @NotBlank @Size(max = 120) String sourceNodeId,
                @NotBlank @Size(max = 120) String sourceOutputPort,
                @NotBlank @Size(max = 120) String targetNodeId,
                @NotBlank @Size(max = 120) String targetInputPort) {
}
