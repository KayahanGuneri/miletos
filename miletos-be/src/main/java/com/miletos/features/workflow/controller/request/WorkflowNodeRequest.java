package com.miletos.features.workflow.controller.request;

import com.fasterxml.jackson.databind.JsonNode;

import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;

public record WorkflowNodeRequest(
                @NotBlank @Size(max = 120) String nodeId,
                @NotBlank @Size(max = 160) String pluginType,
                @NotBlank @Size(max = 80) String pluginVersion,
                @NotNull JsonNode configuration,
                @NotNull @Valid NodePositionRequest position) {
}
