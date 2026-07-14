package com.miletos.features.user.service.input;

public record ProfilePhotoUploadInput(
        String actorEmail,
        String originalFilename,
        String contentType,
        String base64Content
) {
}