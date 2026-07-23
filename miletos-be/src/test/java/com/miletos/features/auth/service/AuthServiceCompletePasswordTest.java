package com.miletos.features.auth.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.auth.command.CompletePasswordCommand;
import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
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
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest
@Transactional
class AuthServiceCompletePasswordTest
        extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL =
            "password-completion-superadmin@miletos.local";

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

    @BeforeEach
    void createRequiredSuperadmin() {
        if (userRepository.findByEmail(SUPERADMIN_EMAIL).isPresent()) {
            return;
        }

        userRepository.saveAndFlush(new User(
                null,
                null,
                SUPERADMIN_EMAIL,
                "unused-test-password-hash",
                "Setup",
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

    @Test
    void completesFirstPasswordAndOnboarding() {
        TestInvite invite = createInvite(
                "valid-setup-token",
                Instant.now(clock).plus(
                        24,
                        ChronoUnit.HOURS
                )
        );

        User updatedUser =
                authService.completePassword(
                        new CompletePasswordCommand(
                                "valid-setup-token",
                                "NewPassword123!"
                        )
                );

        AuthToken updatedInviteToken =
                authTokenRepository
                        .findById(
                                invite.inviteToken().getId()
                        )
                        .orElseThrow();

        assertThat(updatedUser.getId())
                .isEqualTo(
                        invite.invitedUser().getId()
                );

        assertThat(updatedUser.getStatus())
                .isEqualTo(UserStatus.ACTIVE);

        assertThat(updatedUser.getOnboardingStatus())
                .isEqualTo(OnboardingStatus.COMPLETED);

        assertThat(updatedUser.getPasswordHash())
                .isNotEqualTo("NewPassword123!");

        assertThat(
                passwordEncoder.matches(
                        "NewPassword123!",
                        updatedUser.getPasswordHash()
                )
        ).isTrue();

        assertThat(updatedInviteToken.getUsedAt())
                .isNotNull();
    }

    @Test
    void rejectsMissingInviteToken() {
        assertThatThrownBy(() ->
                authService.completePassword(
                        new CompletePasswordCommand(
                                "missing-token",
                                "NewPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.INVITE_TOKEN_NOT_FOUND
                        )
                );
    }

    @Test
    void rejectsExpiredInviteTokenAndHardDeletesPendingUser() {
        TestInvite invite = createInvite(
                "expired-setup-token",
                Instant.now(clock).minus(
                        1,
                        ChronoUnit.MINUTES
                )
        );

        assertThatThrownBy(() ->
                authService.completePassword(
                        new CompletePasswordCommand(
                                "expired-setup-token",
                                "NewPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.INVITE_TOKEN_EXPIRED
                        )
                );

        assertThat(
                authTokenRepository.findById(
                        invite.inviteToken().getId()
                )
        ).isEmpty();

        assertThat(
                userRepository.findById(
                        invite.invitedUser().getId()
                )
        ).isEmpty();
    }

    @Test
    void rejectsAlreadyUsedInviteToken() {
        TestInvite invite = createInvite(
                "used-setup-token",
                Instant.now(clock).plus(
                        24,
                        ChronoUnit.HOURS
                )
        );

        invite.inviteToken().setUsedAt(
                Instant.now(clock)
        );

        authTokenRepository.saveAndFlush(
                invite.inviteToken()
        );

        assertThatThrownBy(() ->
                authService.completePassword(
                        new CompletePasswordCommand(
                                "used-setup-token",
                                "NewPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.INVITE_TOKEN_ALREADY_USED
                        )
                );
    }

    @Test
    void rejectsUserThatIsNotWaitingForPasswordCompletion() {
        TestInvite invite = createInvite(
                "invalid-state-token",
                Instant.now(clock).plus(
                        24,
                        ChronoUnit.HOURS
                )
        );

        invite.invitedUser().setStatus(
                UserStatus.ACTIVE
        );

        invite.invitedUser().setOnboardingStatus(
                OnboardingStatus.COMPLETED
        );

        userRepository.saveAndFlush(
                invite.invitedUser()
        );

        assertThatThrownBy(() ->
                authService.completePassword(
                        new CompletePasswordCommand(
                                "invalid-state-token",
                                "NewPassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.INVALID_ONBOARDING_STATE
                        )
                );
    }

    private TestInvite createInvite(
            String rawToken,
            Instant expiresAt
    ) {
        Company company = companyRepository.saveAndFlush(
                new Company(
                        "Password Completion Tenant "
                                + rawToken,
                        CompanyStatus.ACTIVE
                )
        );

        User superadmin = userRepository
                .findByEmail(SUPERADMIN_EMAIL)
                .orElseThrow();

        User invitedUser = userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        rawToken + "@miletos.local",
                        null,
                        "Invited",
                        "User",
                        UserRole.USER,
                        false,
                        UserStatus.PENDING,
                        OnboardingStatus
                                .PASSWORD_SETUP_REQUIRED,
                        null,
                        null,
                        null,
                        null
                )
        );

        AuthToken inviteToken =
                authTokenRepository.saveAndFlush(
                        AuthToken.invite(
                                company,
                                invitedUser,
                                superadmin,
                                tokenHasher.hash(
                                        rawToken
                                ),
                                expiresAt
                        )
                );

        return new TestInvite(
                invitedUser,
                inviteToken
        );
    }

    private record TestInvite(
            User invitedUser,
            AuthToken inviteToken
    ) {
    }
}
