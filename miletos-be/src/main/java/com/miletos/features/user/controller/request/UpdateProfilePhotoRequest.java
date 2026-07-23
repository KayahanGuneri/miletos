package com.miletos.features.user.controller.request;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public record UpdateProfilePhotoRequest(
        @Size(max = 255)
        String originalFilename,

        @NotBlank
        @Size(max = 100)
        String contentType,

        @NotBlank
        String base64Content
) {
}
