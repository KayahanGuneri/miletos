package com.miletos.features.workflow.controller.request;

import java.util.List;

import com.fasterxml.jackson.databind.JsonNode;

import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;

public record CreateWorkflowRequest(
                @NotBlank @Size(min = 3, max = 120) String name,
                @Size(max = 1000) String description,
                @NotNull List<@Valid WorkflowNodeRequest> nodes,
                @NotNull List<@Valid WorkflowEdgeRequest> edges,
                @NotNull JsonNode metadata) {
}
