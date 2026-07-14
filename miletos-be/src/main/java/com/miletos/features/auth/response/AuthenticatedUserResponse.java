package com.miletos.features.auth.response;

import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;

public record AuthenticatedUserResponse(
        Long id,
        Long companyId,
        String email,
        String firstName,
        String lastName,
        UserRole role,
        boolean superAdmin,
        UserStatus status,
        OnboardingStatus onboardingStatus
) {
}
