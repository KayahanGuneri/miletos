package com.miletos.features.workflowruntime.controller.request;

import jakarta.validation.constraints.NotBlank;

public record CreateWorkflowTriggerRequest(@NotBlank String triggerNodeId) {}
