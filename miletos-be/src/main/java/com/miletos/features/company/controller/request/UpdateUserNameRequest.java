package com.miletos.features.company.controller.request;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public record UpdateUserNameRequest(
        @NotBlank
        @Size(max = 120)
        String firstName,

        @NotBlank
        @Size(max = 120)
        String lastName
) {
}
