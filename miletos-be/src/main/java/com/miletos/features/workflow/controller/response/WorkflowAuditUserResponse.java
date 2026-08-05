package com.miletos.features.workflow.controller.response;

public record WorkflowAuditUserResponse(
        Long id,
        String email,
        String firstName,
        String lastName) {
}
