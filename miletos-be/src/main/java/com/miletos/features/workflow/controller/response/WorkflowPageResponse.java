package com.miletos.features.workflow.controller.response;

import java.util.List;

public record WorkflowPageResponse(
                List<WorkflowSummaryResponse> content,
                Integer page,
                Integer size,
                Long totalElements,
                Integer totalPages,
                Boolean first,
                Boolean last) {
}
