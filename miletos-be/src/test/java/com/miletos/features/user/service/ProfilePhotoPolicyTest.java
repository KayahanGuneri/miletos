package com.miletos.features.user.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.config.ProfilePhotoProperties;
import com.miletos.features.user.exception.ProfilePhotoEmptyException;
import com.miletos.features.user.exception.ProfilePhotoInvalidContentException;
import com.miletos.features.user.exception.ProfilePhotoTooLargeException;
import com.miletos.features.user.exception.ProfilePhotoUnsupportedTypeException;
import com.miletos.features.user.service.input.ProfilePhotoUploadInput;
import java.util.Base64;
import org.junit.jupiter.api.Test;

class ProfilePhotoPolicyTest {
    private final ProfilePhotoPolicy policy =
            new ProfilePhotoPolicy(new ProfilePhotoProperties("unused", 4));

    @Test
    void acceptsPng() { assertAccepted("image/png", "png"); }

    @Test
    void acceptsJpeg() { assertAccepted("image/jpeg", "jpg"); }

    @Test
    void acceptsWebp() { assertAccepted("image/webp", "webp"); }

    @Test
    void rejectsGif() { assertUnsupported("image/gif"); }

    @Test
    void rejectsPdf() { assertUnsupported("application/pdf"); }

    @Test
    void rejectsEmptyDecodedContent() {
        assertThatThrownBy(() -> policy.validate(input("image/png", "data:image/png;base64,")))
                .isInstanceOf(ProfilePhotoEmptyException.class);
    }

    @Test
    void rejectsInvalidBase64() {
        assertThatThrownBy(() -> policy.validate(input("image/png", "not-base64")))
                .isInstanceOf(ProfilePhotoInvalidContentException.class);
    }

    @Test
    void rejectsContentOverConfiguredLimit() {
        assertThatThrownBy(() -> policy.validate(input("image/png", encode(new byte[5]))))
                .isInstanceOf(ProfilePhotoTooLargeException.class);
    }

    private void assertAccepted(String contentType, String extension) {
        ProfilePhotoPolicy.ValidatedProfilePhoto result =
                policy.validate(input(contentType, encode(new byte[] {1, 2, 3})));
        assertThat(result.contentType()).isEqualTo(contentType);
        assertThat(result.extension()).isEqualTo(extension);
    }

    private void assertUnsupported(String contentType) {
        assertThatThrownBy(() -> policy.validate(input(contentType, encode(new byte[] {1}))))
                .isInstanceOf(ProfilePhotoUnsupportedTypeException.class);
    }

    private ProfilePhotoUploadInput input(String contentType, String content) {
        return new ProfilePhotoUploadInput("actor@miletos.local", "avatar", contentType, content);
    }

    private String encode(byte[] content) {
        return Base64.getEncoder().encodeToString(content);
    }
}
