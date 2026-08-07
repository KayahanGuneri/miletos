package com.miletos.features.workflow.controller.request;

import jakarta.validation.constraints.NotNull;

public record NodePositionRequest(
        @NotNull Double x,
        @NotNull Double y) {
}
