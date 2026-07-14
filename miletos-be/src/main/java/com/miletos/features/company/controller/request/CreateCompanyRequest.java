package com.miletos.features.company.controller.request;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public record CreateCompanyRequest(
        @NotBlank
        @Size(max = 160)
        String name
) {
}
