package com.miletos.features.auth.service;

import java.time.Clock;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Map;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import com.miletos.common.util.EmailNormalizer;
import com.miletos.features.auth.command.ChangePasswordCommand;
import com.miletos.features.auth.command.CompletePasswordCommand;
import com.miletos.features.auth.command.ForgotPasswordCommand;
import com.miletos.features.auth.command.LoginCommand;
import com.miletos.features.auth.command.ResetPasswordCommand;
import com.miletos.features.auth.exception.InvalidCredentialsException;
import com.miletos.features.auth.exception.InvalidOnboardingStateException;
import com.miletos.features.auth.exception.InviteTokenAlreadyUsedException;
import com.miletos.features.auth.exception.InviteTokenExpiredException;
import com.miletos.features.auth.exception.InviteTokenNotFoundException;
import com.miletos.features.auth.exception.PasswordResetTokenAlreadyUsedException;
import com.miletos.features.auth.exception.PasswordResetTokenExpiredException;
import com.miletos.features.auth.exception.PasswordResetTokenNotFoundException;
import com.miletos.features.auth.exception.PasswordSetupRequiredException;
import com.miletos.features.auth.model.JwtToken;
import com.miletos.features.auth.model.LoginResult;
import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.auth.repository.entity.AuthTokenType;
import com.miletos.features.user.exception.AuthenticatedUserNotFoundException;
import com.miletos.features.user.exception.UserDisabledException;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.security.session.ActiveSessionService;
import com.miletos.security.token.JwtTokenService;
import com.miletos.services.mail.EmailMessage;
import com.miletos.services.mail.EmailSender;
import com.miletos.services.mail.MailTemplateRenderer;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class AuthService {

        private final UserRepository userRepository;
        private final AuthTokenRepository authTokenRepository;
        private final PasswordEncoder passwordEncoder;
        private final JwtTokenService jwtTokenService;
        private final ActiveSessionService activeSessionService;
        private final SecureTokenGenerator secureTokenGenerator;
        private final TokenHasher tokenHasher;
        private final ExpiredInviteCleanup expiredInviteCleanup;
        private final EmailSender emailSender;
        private final MailTemplateRenderer mailTemplateRenderer;
        private final Clock clock;

        @Value("${miletos.web.base-url}")
        private String webBaseUrl;

        @Value("${miletos.security.password-reset-token-minutes}")
        private long passwordResetTokenMinutes;

        @Transactional(readOnly = true)
        public LoginResult login(LoginCommand command) {
                User user = findLoginUser(command.email());

                validateUserCanLogin(user);
                validatePassword(
                                command.password(),
                                user.getPasswordHash());

                String sessionId = activeSessionService.createSession(
                                user.getId());

                JwtToken accessToken = jwtTokenService.generateAccessToken(
                                user,
                                sessionId);

                return new LoginResult(user, accessToken);
        }

        @Transactional
        public void changePassword(
                        ChangePasswordCommand command) {
                User user = findAuthenticatedUser(
                                command.actorEmail());

                validateUserCanChangePassword(user);

                validateCurrentPassword(
                                command.currentPassword(),
                                user.getPasswordHash());

                user.setPasswordHash(
                                passwordEncoder.encode(
                                                command.newPassword()));

                userRepository.save(user);
        }

        @Transactional
        public void requestPasswordReset(
                        ForgotPasswordCommand command) {
                String normalizedEmail = EmailNormalizer.normalize(
                                command.email());

                userRepository
                                .findByEmail(normalizedEmail)
                                .filter(this::canReceivePasswordReset)
                                .ifPresent(
                                                this::createAndSendPasswordResetToken);
        }

        @Transactional
        public User resetPassword(
                        ResetPasswordCommand command) {
                String tokenHash = tokenHasher.hash(
                                command.token());

                AuthToken passwordResetToken = findPasswordResetToken(tokenHash);

                validatePasswordResetToken(
                                passwordResetToken);

                User user = passwordResetToken.getUser();

                validateUserCanResetPassword(user);

                Instant now = Instant.now(clock);

                user.setPasswordHash(
                                passwordEncoder.encode(
                                                command.password()));

                passwordResetToken.setUsedAt(now);

                userRepository.save(user);
                authTokenRepository.save(
                                passwordResetToken);

                return user;
        }

        @Transactional(noRollbackFor = InviteTokenExpiredException.class)
        public User completePassword(
                        CompletePasswordCommand command) {
                String tokenHash = tokenHasher.hash(
                                command.token());

                AuthToken inviteToken = findInviteToken(tokenHash);

                validateInviteToken(inviteToken);

                User user = inviteToken.getUser();

                validateInviteUserState(user);

                Instant now = Instant.now(clock);

                user.setPasswordHash(
                                passwordEncoder.encode(
                                                command.password()));

                user.setStatus(UserStatus.ACTIVE);

                user.setOnboardingStatus(
                                OnboardingStatus.COMPLETED);

                inviteToken.setUsedAt(now);

                userRepository.save(user);
                authTokenRepository.save(inviteToken);

                return user;
        }

        private User findLoginUser(String email) {
                String normalizedEmail = EmailNormalizer.normalize(email);

                return userRepository
                                .findByEmail(normalizedEmail)
                                .orElseThrow(
                                                InvalidCredentialsException::new);
        }

        private User findAuthenticatedUser(
                        String actorEmail) {
                String normalizedActorEmail = EmailNormalizer.normalize(actorEmail);

                return userRepository
                                .findByEmail(normalizedActorEmail)
                                .orElseThrow(
                                                AuthenticatedUserNotFoundException::new);
        }

        private void validateUserCanLogin(User user) {
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

                if (user.getPasswordHash() == null
                                || user.getPasswordHash().isBlank()) {
                        throw new InvalidCredentialsException();
                }
        }

        private void validateUserCanChangePassword(
                        User user) {
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

                if (user.getPasswordHash() == null
                                || user.getPasswordHash().isBlank()) {
                        throw new InvalidCredentialsException();
                }
        }

        private void validatePassword(
                        String rawPassword,
                        String passwordHash) {
                if (!passwordEncoder.matches(
                                rawPassword,
                                passwordHash)) {
                        throw new InvalidCredentialsException();
                }
        }

        private void validateCurrentPassword(
                        String currentPassword,
                        String passwordHash) {
                if (!passwordEncoder.matches(
                                currentPassword,
                                passwordHash)) {
                        throw new InvalidCredentialsException();
                }
        }

        private boolean canReceivePasswordReset(
                        User user) {
                return user.getStatus() == UserStatus.ACTIVE
                                && (user.getOnboardingStatus() == OnboardingStatus.COMPLETED
                                                || user.getOnboardingStatus() == OnboardingStatus.NOT_REQUIRED)
                                && user.getPasswordHash() != null
                                && !user.getPasswordHash().isBlank();
        }

        private void createAndSendPasswordResetToken(
                        User user) {
                String rawToken = secureTokenGenerator.generateToken();

                String tokenHash = tokenHasher.hash(rawToken);

                Instant expiresAt = Instant.now(clock).plus(
                                passwordResetTokenMinutes,
                                ChronoUnit.MINUTES);

                authTokenRepository.save(
                                AuthToken.passwordReset(
                                                user,
                                                tokenHash,
                                                expiresAt));

                sendPasswordResetEmail(
                                user,
                                rawToken);
        }

        private AuthToken findPasswordResetToken(
                        String tokenHash) {
                return authTokenRepository
                                .findByTokenHashAndType(
                                                tokenHash,
                                                AuthTokenType.PASSWORD_RESET)
                                .orElseThrow(
                                                PasswordResetTokenNotFoundException::new);
        }

        private void validatePasswordResetToken(
                        AuthToken passwordResetToken) {
                if (passwordResetToken.getUsedAt() != null) {
                        throw new PasswordResetTokenAlreadyUsedException();
                }

                if (!passwordResetToken
                                .getExpiresAt()
                                .isAfter(Instant.now(clock))) {
                        throw new PasswordResetTokenExpiredException();
                }
        }

        private void validateUserCanResetPassword(
                        User user) {
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
        }

        private void sendPasswordResetEmail(
                        User user,
                        String rawToken) {
                String resetLink = webBaseUrl
                                + "/reset-password?token="
                                + rawToken;

                Map<String, String> variables = Map.of(
                                "firstName", user.getFirstName(),
                                "resetLink", resetLink,
                                "expiresInMinutes", Long.toString(passwordResetTokenMinutes));
                String textBody = mailTemplateRenderer.renderText(
                                "password-reset.txt",
                                variables);
                String htmlBody = mailTemplateRenderer.renderHtml("password-reset.html", variables);

                emailSender.send(new EmailMessage(
                                user.getEmail(), "Reset your Miletos password", textBody, htmlBody));
        }

        private AuthToken findInviteToken(
                        String tokenHash) {
                return authTokenRepository
                                .findByTokenHashAndType(
                                                tokenHash,
                                                AuthTokenType.INVITE)
                                .orElseThrow(
                                                InviteTokenNotFoundException::new);
        }

        private void validateInviteToken(
                        AuthToken inviteToken) {
                if (inviteToken.getUsedAt() != null) {
                        throw new InviteTokenAlreadyUsedException();
                }

                if (!inviteToken
                                .getExpiresAt()
                                .isAfter(Instant.now(clock))) {
                        expiredInviteCleanup
                                        .hardDeletePendingUserForExpiredInvite(
                                                        inviteToken);

                        throw new InviteTokenExpiredException();
                }
        }

        private void validateInviteUserState(
                        User user) {
                if (user.getStatus() != UserStatus.PENDING
                                || user.getOnboardingStatus() != OnboardingStatus.PASSWORD_SETUP_REQUIRED) {
                        throw new InvalidOnboardingStateException();
                }
        }
}
