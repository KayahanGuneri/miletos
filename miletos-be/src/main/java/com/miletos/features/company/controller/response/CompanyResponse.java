package com.miletos.features.company.controller.response;

import java.time.Instant;

import com.miletos.features.company.repository.entity.CompanyStatus;

public record CompanyResponse(
        Long id,
        String name,
        CompanyStatus status,
        Instant createdAt,
        Instant updatedAt
) {
}
