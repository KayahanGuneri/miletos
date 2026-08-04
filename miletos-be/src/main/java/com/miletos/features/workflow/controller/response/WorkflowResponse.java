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
                long revision,
                List<WorkflowNodeResponse> nodes,
                List<WorkflowEdgeResponse> edges,
                JsonNode metadata,
                int nodeCount,
                int edgeCount,
                String createdByEmail,
                String updatedByEmail,
                Instant createdAt,
                Instant updatedAt) {
}
