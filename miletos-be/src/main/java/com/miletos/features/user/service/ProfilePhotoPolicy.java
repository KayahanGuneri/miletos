package com.miletos.features.user.service;

import java.util.Base64;
import java.util.Locale;

import org.springframework.stereotype.Component;

import com.miletos.config.ProfilePhotoProperties;
import com.miletos.features.user.exception.ProfilePhotoEmptyException;
import com.miletos.features.user.exception.ProfilePhotoInvalidContentException;
import com.miletos.features.user.exception.ProfilePhotoTooLargeException;
import com.miletos.features.user.exception.ProfilePhotoUnsupportedTypeException;
import com.miletos.features.user.service.input.ProfilePhotoUploadInput;

import lombok.RequiredArgsConstructor;

@Component
@RequiredArgsConstructor
public class ProfilePhotoPolicy {
    private final ProfilePhotoProperties properties;

    public ValidatedProfilePhoto validate(ProfilePhotoUploadInput input) {
        if (input.base64Content() == null || input.base64Content().isBlank()) {
            throw new ProfilePhotoEmptyException();
        }
        String encoded = input.base64Content().trim();
        int separator = encoded.indexOf(',');
        if (separator >= 0) {
            encoded = encoded.substring(separator + 1);
        }
        byte[] content;
        try {
            content = Base64.getDecoder().decode(encoded);
        } catch (IllegalArgumentException exception) {
            throw new ProfilePhotoInvalidContentException(exception);
        }
        if (content.length == 0) {
            throw new ProfilePhotoEmptyException();
        }
        if (content.length > properties.maxSizeBytes()) {
            throw new ProfilePhotoTooLargeException();
        }
        String contentType = normalizeContentType(input.contentType());
        return new ValidatedProfilePhoto(content, contentType, extensionFor(contentType));
    }

    private String normalizeContentType(String contentType) {
        if (contentType == null || contentType.isBlank()) {
            throw new ProfilePhotoUnsupportedTypeException();
        }
        return contentType.toLowerCase(Locale.ROOT);
    }

    private String extensionFor(String contentType) {
        return switch (contentType) {
            case "image/png" -> "png";
            case "image/jpeg" -> "jpg";
            case "image/webp" -> "webp";
            default -> throw new ProfilePhotoUnsupportedTypeException();
        };
    }

    public record ValidatedProfilePhoto(byte[] content, String contentType, String extension) {
    }
}
