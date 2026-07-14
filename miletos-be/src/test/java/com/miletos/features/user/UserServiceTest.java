package com.miletos.features.user;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.services.file.metadata.StoredFileRepository;
import com.miletos.features.user.service.output.ProfilePhotoContent;
import com.miletos.features.user.service.input.ProfilePhotoUploadInput;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Base64;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest(properties = {
        "miletos.security.jwt.secret=test-profile-secret-key-for-miletos-auth-flow-32-bytes-minimum",
        "miletos.security.jwt.issuer=https://miletos-profile-test.local",
        "miletos.security.jwt.access-token-minutes=30",
        "miletos.profile-photo.storage-directory=target/test-profile-photos",
        "miletos.profile-photo.max-size-bytes=16"
})
@Transactional
class UserServiceTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL = "profile-photo-superadmin@miletos.local";
    private static final Path STORAGE_ROOT = Path.of("target/test-profile-photos");

    @Autowired
    private UserService userService;

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private StoredFileRepository storedFileRepository;

    @BeforeEach
    void cleanStorageDirectory() throws IOException {
        if (Files.exists(STORAGE_ROOT)) {
            try (var paths = Files.walk(STORAGE_ROOT)) {
                paths.sorted((left, right) -> right.compareTo(left))
                        .forEach(path -> {
                            try {
                                Files.deleteIfExists(path);
                            } catch (IOException exception) {
                                throw new IllegalStateException("Could not clean test profile photo directory", exception);
                            }
                        });
            }
        }

        if (userRepository.findByEmail(SUPERADMIN_EMAIL).isEmpty()) {
            userRepository.saveAndFlush(new User(
                    null,
                    null,
                    SUPERADMIN_EMAIL,
                    "unused-test-password-hash",
                    "Profile",
                    "Admin",
                    null,
                    true,
                    UserStatus.ACTIVE,
                    OnboardingStatus.NOT_REQUIRED,
                    null,
                    null,
                    null,
                    null));
        }
    }

    @Test
    void returnsCurrentUserProfile() {
        User user = userService.getCurrentUser(SUPERADMIN_EMAIL);

        assertThat(user.getEmail()).isEqualTo(SUPERADMIN_EMAIL);
        assertThat(user.getStatus()).isEqualTo(UserStatus.ACTIVE);
        assertThat(user.getOnboardingStatus()).isEqualTo(OnboardingStatus.NOT_REQUIRED);
    }

    @Test
    void updatesOwnProfilePhotoThroughFileService() {
        byte[] content = "png-bytes".getBytes();

        User updatedUser = userService.updateOwnProfilePhoto(new ProfilePhotoUploadInput(
                SUPERADMIN_EMAIL,
                "avatar.png",
                "image/png",
                toBase64(content)
        ));

        assertThat(updatedUser.getProfilePhotoFile()).isNotNull();
        assertThat(updatedUser.getProfilePhotoFile().getId()).isNotNull();
        assertThat(updatedUser.getProfilePhotoFile().getStoragePath()).endsWith(".png");
        assertThat(updatedUser.getProfilePhotoFile().getContentType()).isEqualTo("image/png");
        assertThat(updatedUser.getProfilePhotoFile().getSizeBytes()).isEqualTo(content.length);
        assertThat(storedFileRepository.findById(updatedUser.getProfilePhotoFile().getId())).isPresent();
        assertThat(Files.exists(STORAGE_ROOT.resolve(updatedUser.getProfilePhotoFile().getStoragePath()))).isTrue();
    }

    @Test
    void loadsOwnProfilePhotoFromStoredFile() {
        byte[] content = "png-bytes".getBytes();

        userService.updateOwnProfilePhoto(new ProfilePhotoUploadInput(
                SUPERADMIN_EMAIL,
                "avatar.png",
                "image/png",
                toBase64(content)
        ));

        ProfilePhotoContent loadedPhoto = userService.getOwnProfilePhoto(SUPERADMIN_EMAIL);

        assertThat(loadedPhoto.content()).isEqualTo(content);
        assertThat(loadedPhoto.contentType()).isEqualTo("image/png");
    }

    @Test
    void rejectsEmptyProfilePhoto() {
        assertThatThrownBy(() -> userService.updateOwnProfilePhoto(new ProfilePhotoUploadInput(
                SUPERADMIN_EMAIL,
                "empty.png",
                "image/png",
                ""
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.PROFILE_PHOTO_EMPTY)
                );
    }

    @Test
    void rejectsInvalidBase64ProfilePhoto() {
        assertThatThrownBy(() -> userService.updateOwnProfilePhoto(new ProfilePhotoUploadInput(
                SUPERADMIN_EMAIL,
                "invalid.png",
                "image/png",
                "not-valid-base64"
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.PROFILE_PHOTO_INVALID_CONTENT)
                );
    }

    @Test
    void rejectsTooLargeProfilePhoto() {
        byte[] content = "this-file-is-too-large".getBytes();

        assertThatThrownBy(() -> userService.updateOwnProfilePhoto(new ProfilePhotoUploadInput(
                SUPERADMIN_EMAIL,
                "large.png",
                "image/png",
                toBase64(content)
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.PROFILE_PHOTO_TOO_LARGE)
                );
    }

    @Test
    void rejectsUnsupportedProfilePhotoContentType() {
        byte[] content = "pdf".getBytes();

        assertThatThrownBy(() -> userService.updateOwnProfilePhoto(new ProfilePhotoUploadInput(
                SUPERADMIN_EMAIL,
                "avatar.pdf",
                "application/pdf",
                toBase64(content)
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.PROFILE_PHOTO_UNSUPPORTED_TYPE)
                );
    }

    @Test
    void rejectsPendingUserProfilePhotoUpdate() {
        Company company = companyRepository.saveAndFlush(new Company("Profile Pending Tenant", CompanyStatus.ACTIVE));

        userRepository.saveAndFlush(new User(
                null,
                company,
                "pending-profile@miletos.local",
                null,
                "Pending",
                "Profile",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        ));

        byte[] content = "png-bytes".getBytes();

        assertThatThrownBy(() -> userService.updateOwnProfilePhoto(new ProfilePhotoUploadInput(
                "pending-profile@miletos.local",
                "avatar.png",
                "image/png",
                toBase64(content)
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.PASSWORD_SETUP_REQUIRED)
                );
    }

    private String toBase64(byte[] content) {
        return Base64.getEncoder().encodeToString(content);
    }
}
