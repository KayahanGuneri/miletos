package com.miletos.features.workflow.controller.response;

import java.time.Instant;
import java.util.List;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;

public record WorkflowResponse(
                Long id,
                String name,
                String description,
                WorkflowStatus status,
                Long revision,
                List<WorkflowNodeResponse> nodes,
                List<WorkflowEdgeResponse> edges,
                JsonNode metadata,
                Integer nodeCount,
                Integer edgeCount,
                WorkflowAuditUserResponse createdBy,
                WorkflowAuditUserResponse updatedBy,
                Instant createdAt,
                Instant updatedAt) {
}
