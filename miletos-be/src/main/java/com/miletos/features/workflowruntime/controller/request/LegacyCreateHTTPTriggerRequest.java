package com.miletos.features.workflowruntime.controller.request;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;

public record LegacyCreateHTTPTriggerRequest(
    @NotNull Long workflowId, @NotBlank String triggerNodeId) {}
