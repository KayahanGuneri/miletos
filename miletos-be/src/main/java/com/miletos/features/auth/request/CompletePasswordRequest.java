package com.miletos.features.auth.request;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public record CompletePasswordRequest(
                @NotBlank String token,

                @NotBlank @Size(min = 8, max = 120) String password) {
}
