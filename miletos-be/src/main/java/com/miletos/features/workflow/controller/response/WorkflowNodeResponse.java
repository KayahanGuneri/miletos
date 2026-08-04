package com.miletos.features.workflow.controller.response;

import com.fasterxml.jackson.databind.JsonNode;

public record WorkflowNodeResponse(
                String nodeId,
                String pluginType,
                String pluginVersion,
                JsonNode configuration,
                NodePositionResponse position) {
}
