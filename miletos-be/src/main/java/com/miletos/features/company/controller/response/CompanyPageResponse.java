package com.miletos.features.company.controller.response;

import java.util.List;

public record CompanyPageResponse(
        List<CompanyResponse> content,
        int page,
        int size,
        long totalElements,
        int totalPages,
        boolean first,
        boolean last
) {
}