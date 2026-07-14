package com.miletos.features.auth.response;

import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.UserStatus;

public record CompletePasswordResponse(
        Long userId,
        String email,
        UserStatus status,
        OnboardingStatus onboardingStatus
) {
}
