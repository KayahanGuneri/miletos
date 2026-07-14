package com.miletos.features.user;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import com.miletos.config.ProfilePhotoProperties;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.service.input.ProfilePhotoUploadInput;
import com.miletos.features.user.service.ProfilePhotoPolicy;
import com.miletos.services.file.FileStorageService;
import com.miletos.services.file.metadata.StoredFile;
import com.miletos.security.AuthenticatedActorResolver;
import java.nio.file.Path;
import org.junit.jupiter.api.Test;

class UserReplacementTest {
    @Test
    void replacementStoresNewFileAndUpdatesUserReferenceInSameServiceOperation() {
        UserRepository users = mock(UserRepository.class);
        FileStorageService storage = mock(FileStorageService.class);
        AuthenticatedActorResolver actors = mock(AuthenticatedActorResolver.class);
        ProfilePhotoPolicy policy = mock(ProfilePhotoPolicy.class);
        ProfilePhotoProperties properties = new ProfilePhotoProperties("photos", 1024);
        User user = new User(
                null,
                null,
                "actor@miletos.local",
                "hash",
                "Ada",
                "Lovelace",
                UserRole.USER,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null);
        user.setId(7L);
        StoredFile oldFile = new StoredFile(7L, "old.png", "image/png", "7/old.png", 1L);
        StoredFile newFile = new StoredFile(7L, "new.png", "image/png", "7/new.png", 1L);
        user.setProfilePhotoFile(oldFile);
        ProfilePhotoUploadInput input =
                new ProfilePhotoUploadInput(user.getEmail(), "new.png", "image/png", "AQ==");
        when(actors.resolve(user.getEmail())).thenReturn(user);
        when(policy.validate(input)).thenReturn(
                new ProfilePhotoPolicy.ValidatedProfilePhoto(new byte[] {1}, "image/png", "png"));
        when(storage.store(any(Path.class), any(), any(), any(), any(), any())).thenReturn(newFile);
        when(users.save(user)).thenReturn(user);
        UserService service = new UserService(users, storage, actors, properties, policy);

        User updated = service.updateOwnProfilePhoto(input);

        assertThat(updated.getProfilePhotoFile()).isSameAs(newFile);
        verify(storage).store(Path.of("photos"), 7L, "new.png", "image/png", "png", new byte[] {1});
        verify(users).save(user);
    }
}
