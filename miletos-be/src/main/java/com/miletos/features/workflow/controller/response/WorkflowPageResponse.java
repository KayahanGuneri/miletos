package com.miletos.features.workflow.controller.response;

import java.util.List;

public record WorkflowPageResponse(
                List<WorkflowSummaryResponse> content,
                int page,
                int size,
                long totalElements,
                int totalPages,
                boolean first,
                boolean last) {
}
