package com.miletos.features.user;

import java.nio.file.Path;

import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import com.miletos.config.ProfilePhotoProperties;
import com.miletos.features.auth.exception.PasswordSetupRequiredException;
import com.miletos.features.user.exception.ProfilePhotoNotFoundException;
import com.miletos.features.user.exception.ProfilePhotoStorageException;
import com.miletos.features.user.exception.UserDisabledException;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.service.ProfilePhotoPolicy;
import com.miletos.features.user.service.input.ProfilePhotoUploadInput;
import com.miletos.features.user.service.output.ProfilePhotoContent;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.services.file.FileStorageException;
import com.miletos.services.file.FileStorageService;
import com.miletos.services.file.StoredContent;
import com.miletos.services.file.StoredContentNotFoundException;
import com.miletos.services.file.metadata.StoredFile;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class UserService {

    private final UserRepository userRepository;
    private final FileStorageService fileStorageService;
    private final AuthenticatedActorResolver authenticatedActorResolver;
    private final ProfilePhotoProperties profilePhotoProperties;
    private final ProfilePhotoPolicy profilePhotoPolicy;

    @Transactional(readOnly = true)
    public User getCurrentUser(String actorEmail) {
        User user = authenticatedActorResolver.resolve(actorEmail);
        validateActiveProfileUser(user);

        return user;
    }

    @Transactional
    public User updateOwnProfilePhoto(
            ProfilePhotoUploadInput input) {
        User user = authenticatedActorResolver.resolve(input.actorEmail());
        validateActiveProfileUser(user);

        ProfilePhotoPolicy.ValidatedProfilePhoto photo = profilePhotoPolicy.validate(input);
        StoredFile storedFile;
        try {
            storedFile = fileStorageService.store(
                    storageRoot(), user.getId(), input.originalFilename(),
                    photo.contentType(), photo.extension(), photo.content());
        } catch (FileStorageException exception) {
            throw new ProfilePhotoStorageException(exception);
        }

        user.setProfilePhotoFile(storedFile);

        return userRepository.save(user);
    }

    @Transactional(readOnly = true)
    public ProfilePhotoContent getOwnProfilePhoto(
            String actorEmail) {
        User user = authenticatedActorResolver.resolve(actorEmail);
        validateActiveProfileUser(user);

        if (user.getProfilePhotoFile() == null) {
            throw new ProfilePhotoNotFoundException();
        }

        try {
            StoredContent content = fileStorageService.load(storageRoot(), user.getProfilePhotoFile());
            return new ProfilePhotoContent(content.content(), content.contentType());
        } catch (StoredContentNotFoundException exception) {
            throw new ProfilePhotoNotFoundException();
        } catch (FileStorageException exception) {
            throw new ProfilePhotoStorageException(exception);
        }
    }

    private Path storageRoot() {
        return Path.of(profilePhotoProperties.storageDirectory());
    }

    private void validateActiveProfileUser(User user) {
        if (user.getStatus() == UserStatus.DISABLED) {
            throw new UserDisabledException();
        }

        if (user.getStatus() == UserStatus.PENDING
                || user.getOnboardingStatus() == OnboardingStatus.PASSWORD_SETUP_REQUIRED) {
            throw new PasswordSetupRequiredException();
        }

        if (user.getStatus() != UserStatus.ACTIVE) {
            throw new UserDisabledException();
        }

        if (user.getOnboardingStatus() != OnboardingStatus.COMPLETED
                && user.getOnboardingStatus() != OnboardingStatus.NOT_REQUIRED) {
            throw new PasswordSetupRequiredException();
        }
    }
}
