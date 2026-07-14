package com.miletos.features.auth.request;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public record ChangePasswordRequest(
                @NotBlank @Size(min = 8, max = 120) String currentPassword,

                @NotBlank @Size(min = 8, max = 120) String newPassword) {
}
