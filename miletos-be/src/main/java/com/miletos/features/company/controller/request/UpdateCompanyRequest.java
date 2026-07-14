package com.miletos.features.company.controller.request;

import com.miletos.features.company.repository.entity.CompanyStatus;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;

public record UpdateCompanyRequest(
        @NotBlank
        @Size(max = 160)
        String name,

        @NotNull
        CompanyStatus status
) {
}
