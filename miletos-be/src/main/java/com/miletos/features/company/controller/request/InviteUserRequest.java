package com.miletos.features.company.controller.request;

import com.miletos.features.user.repository.entity.UserRole;

import jakarta.validation.constraints.Email;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;

public record InviteUserRequest(
                @NotNull Long companyId,

                @NotBlank @Email @Size(max = 320) String email,

                @NotBlank @Size(max = 120) String firstName,

                @NotBlank @Size(max = 120) String lastName,

                @NotNull UserRole role) {
}
