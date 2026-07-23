package com.miletos.features.company.controller.response;

import java.time.Instant;

public record InviteUserResponse(
        Long userId,
        Long companyId,
        String email,
        String firstName,
        String lastName,
        String role,
        String status,
        String onboardingStatus,
        Instant inviteExpiresAt
) {
}
