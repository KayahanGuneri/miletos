package com.miletos.features.auth.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.services.mail.EmailMessage;
import com.miletos.services.mail.EmailSender;
import com.miletos.features.auth.command.ForgotPasswordCommand;
import com.miletos.features.auth.command.ResetPasswordCommand;
import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.auth.repository.entity.AuthTokenType;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import java.time.Clock;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Primary;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest(properties = {
        "miletos.web.base-url=http://localhost:3000",
        "miletos.security.password-reset-token-minutes=30",
        "miletos.security.jwt.secret="
                + "test-reset-secret-key-for-miletos-auth-flow-"
                + "32-bytes-minimum",
        "miletos.security.jwt.issuer="
                + "https://miletos-reset-test.local",
        "miletos.security.jwt.access-token-minutes=30"
})
@Transactional
class AuthServicePasswordResetTest
        extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL =
            "password-reset-superadmin@miletos.local";

    @Autowired
    private AuthService authService;

    @Autowired
    private TokenHasher tokenHasher;

    @Autowired
    private PasswordEncoder passwordEncoder;

    @Autowired
    private Clock clock;

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private AuthTokenRepository authTokenRepository;

    @Autowired
    private RecordingEmailSender recordingEmailSender;

    @BeforeEach
    void prepareTestState() {
        recordingEmailSender.clear();

        if (userRepository.findByEmail(SUPERADMIN_EMAIL).isEmpty()) {
            userRepository.saveAndFlush(new User(
                    null,
                    null,
                    SUPERADMIN_EMAIL,
                    passwordEncoder.encode("Reset123!"),
                    "Reset",
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
    void requestPasswordResetCreatesTokenAndSendsMailForActiveUser() {
        authService.requestPasswordReset(
                new ForgotPasswordCommand(
                        SUPERADMIN_EMAIL
                )
        );

        User user = userRepository
                .findByEmail(SUPERADMIN_EMAIL)
                .orElseThrow();

        List<AuthToken> tokens =
                authTokenRepository
                        .findAllByTypeAndUser_Id(
                                AuthTokenType.PASSWORD_RESET,
                                user.getId()
                        );

        assertThat(tokens).hasSize(1);

        assertThat(
                tokens.getFirst().getTokenHash()
        ).hasSize(64);

        assertThat(
                tokens.getFirst().getExpiresAt()
        ).isAfter(Instant.now(clock));

        assertThat(
                recordingEmailSender.messages()
        ).hasSize(1);

        assertThat(
                recordingEmailSender
                        .messages()
                        .getFirst()
                        .to()
        ).isEqualTo(SUPERADMIN_EMAIL);

        assertThat(
                recordingEmailSender
                        .messages()
                        .getFirst()
                        .body()
        )
                .contains(
                        "http://localhost:3000/"
                                + "reset-password?token="
                )
                .contains("30 minutes");
    }

    @Test
    void requestPasswordResetDoesNotRevealUnknownEmail() {
        authService.requestPasswordReset(
                new ForgotPasswordCommand(
                        "unknown-reset@miletos.local"
                )
        );

        assertThat(
                recordingEmailSender.messages()
        ).isEmpty();
    }

    @Test
    void resetPasswordChangesPasswordAndMarksTokenUsed() {
        TestPasswordReset reset =
                createPasswordResetToken(
                        "valid-reset-token",
                        Instant.now(clock).plus(
                                30,
                                ChronoUnit.MINUTES
                        ),
                        UserStatus.ACTIVE,
                        OnboardingStatus.COMPLETED
                );

        User updatedUser =
                authService.resetPassword(
                        new ResetPasswordCommand(
                                "valid-reset-token",
                                "NewResetPassword123!"
                        )
                );

        AuthToken updatedToken =
                authTokenRepository
                        .findById(
                                reset.passwordResetToken()
                                        .getId()
                        )
                        .orElseThrow();

        assertThat(updatedUser.getId())
                .isEqualTo(reset.user().getId());

        assertThat(
                passwordEncoder.matches(
                        "NewResetPassword123!",
                        updatedUser.getPasswordHash()
                )
        ).isTrue();

        assertThat(updatedToken.getUsedAt())
                .isNotNull();
    }

    @Test
    void rejectsMissingPasswordResetToken() {
        assertThatThrownBy(() ->
                authService.resetPassword(
                        new ResetPasswordCommand(
                                "missing-reset-token",
                                "NewResetPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode
                                        .PASSWORD_RESET_TOKEN_NOT_FOUND
                        )
                );
    }

    @Test
    void rejectsExpiredPasswordResetToken() {
        createPasswordResetToken(
                "expired-reset-token",
                Instant.now(clock).minus(
                        1,
                        ChronoUnit.MINUTES
                ),
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED
        );

        assertThatThrownBy(() ->
                authService.resetPassword(
                        new ResetPasswordCommand(
                                "expired-reset-token",
                                "NewResetPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode
                                        .PASSWORD_RESET_TOKEN_EXPIRED
                        )
                );
    }

    @Test
    void rejectsAlreadyUsedPasswordResetToken() {
        TestPasswordReset reset =
                createPasswordResetToken(
                        "used-reset-token",
                        Instant.now(clock).plus(
                                30,
                                ChronoUnit.MINUTES
                        ),
                        UserStatus.ACTIVE,
                        OnboardingStatus.COMPLETED
                );

        reset.passwordResetToken().setUsedAt(
                Instant.now(clock)
        );

        authTokenRepository.saveAndFlush(
                reset.passwordResetToken()
        );

        assertThatThrownBy(() ->
                authService.resetPassword(
                        new ResetPasswordCommand(
                                "used-reset-token",
                                "NewResetPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode
                                        .PASSWORD_RESET_TOKEN_ALREADY_USED
                        )
                );
    }

    @Test
    void rejectsDisabledUserPasswordReset() {
        createPasswordResetToken(
                "disabled-reset-token",
                Instant.now(clock).plus(
                        30,
                        ChronoUnit.MINUTES
                ),
                UserStatus.DISABLED,
                OnboardingStatus.COMPLETED
        );

        assertThatThrownBy(() ->
                authService.resetPassword(
                        new ResetPasswordCommand(
                                "disabled-reset-token",
                                "NewResetPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.USER_DISABLED
                        )
                );
    }

    @Test
    void rejectsPendingUserPasswordReset() {
        createPasswordResetToken(
                "pending-reset-token",
                Instant.now(clock).plus(
                        30,
                        ChronoUnit.MINUTES
                ),
                UserStatus.PENDING,
                OnboardingStatus
                        .PASSWORD_SETUP_REQUIRED
        );

        assertThatThrownBy(() ->
                authService.resetPassword(
                        new ResetPasswordCommand(
                                "pending-reset-token",
                                "NewResetPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.PASSWORD_SETUP_REQUIRED
                        )
                );
    }

    private TestPasswordReset createPasswordResetToken(
            String rawToken,
            Instant expiresAt,
            UserStatus userStatus,
            OnboardingStatus onboardingStatus
    ) {
        Company company = companyRepository.saveAndFlush(
                new Company(
                        "Password Reset Tenant "
                                + rawToken,
                        CompanyStatus.ACTIVE
                )
        );

        User user = userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        rawToken + "@miletos.local",
                        passwordEncoder.encode(
                                "OldPassword123!"
                        ),
                        "Reset",
                        "User",
                        UserRole.USER,
                        false,
                        userStatus,
                        onboardingStatus,
                        null,
                        null,
                        null,
                        null
                )
        );

        AuthToken passwordResetToken =
                authTokenRepository
                        .saveAndFlush(
                                AuthToken.passwordReset(
                                        user,
                                        tokenHasher.hash(
                                                rawToken
                                        ),
                                        expiresAt
                                )
                        );

        return new TestPasswordReset(
                user,
                passwordResetToken
        );
    }

    @TestConfiguration
    static class AuthServicePasswordResetTestConfig {

        @Bean
        @Primary
        RecordingEmailSender recordingEmailSender() {
            return new RecordingEmailSender();
        }
    }

    static class RecordingEmailSender
            implements EmailSender {

        private final List<MailMessage> messages =
                new ArrayList<>();

        @Override
        public void send(EmailMessage email) {
            messages.add(
                    new MailMessage(
                            email.to(),
                            email.subject(),
                            email.textBody()
                    )
            );
        }

        List<MailMessage> messages() {
            return messages;
        }

        void clear() {
            messages.clear();
        }
    }

    record MailMessage(
            String to,
            String subject,
            String body
    ) {
    }

    private record TestPasswordReset(
            User user,
            AuthToken passwordResetToken
    ) {
    }
}
