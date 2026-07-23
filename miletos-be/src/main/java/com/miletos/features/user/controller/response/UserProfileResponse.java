package com.miletos.features.user.controller.response;

public record UserProfileResponse(
        Long id,
        Long companyId,
        String email,
        String firstName,
        String lastName,
        String role,
        String status,
        String onboardingStatus,
        boolean superAdmin,
        Long profilePhotoFileId
) {
}
