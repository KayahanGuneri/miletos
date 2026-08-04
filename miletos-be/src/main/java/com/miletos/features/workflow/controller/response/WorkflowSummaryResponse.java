package com.miletos.features.workflow.controller.response;

import java.time.Instant;

import com.miletos.features.workflow.repository.entity.WorkflowStatus;

public record WorkflowSummaryResponse(
                Long id,
                String name,
                String description,
                WorkflowStatus status,
                long revision,
                int nodeCount,
                int edgeCount,
                String createdByEmail,
                String updatedByEmail,
                Instant createdAt,
                Instant updatedAt) {
}
