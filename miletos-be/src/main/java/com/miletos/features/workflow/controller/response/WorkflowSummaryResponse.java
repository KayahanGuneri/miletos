package com.miletos.features.workflow.controller.response;

import java.time.Instant;

import com.miletos.features.workflow.repository.entity.WorkflowStatus;

public record WorkflowSummaryResponse(
                Long id,
                String name,
                String description,
                WorkflowStatus status,
                Long revision,
                Integer nodeCount,
                Integer edgeCount,
                WorkflowAuditUserResponse createdBy,
                WorkflowAuditUserResponse updatedBy,
                Instant createdAt,
                Instant updatedAt) {
}
