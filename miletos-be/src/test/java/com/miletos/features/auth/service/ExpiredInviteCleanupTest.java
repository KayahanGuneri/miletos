package com.miletos.features.auth.service;


import static org.assertj.core.api.Assertions.assertThat;

import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import java.time.Clock;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest(properties = {
        "miletos.security.invite-cleanup.initial-delay-ms=300000",
        "miletos.security.invite-cleanup.fixed-delay-ms=300000"
})
@Transactional
class ExpiredInviteCleanupTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL = "expired-invite-cleanup-superadmin@miletos.local";

    @Autowired
    private ExpiredInviteCleanup expiredInviteCleanup;

    @Autowired
    private TokenHasher tokenHasher;

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
                "Cleanup",
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
    void hardDeletesExpiredPendingInviteUsers() {
        TestInvite expiredInvite = createInvite("cleanup-expired-token", Instant.now(clock).minus(1, ChronoUnit.MINUTES));
        TestInvite activeInvite = createInvite("cleanup-active-token", Instant.now(clock).plus(24, ChronoUnit.HOURS));

        expiredInviteCleanup.deleteExpiredPendingInviteUsers();

        assertThat(authTokenRepository.findById(expiredInvite.inviteToken().getId())).isEmpty();
        assertThat(userRepository.findById(expiredInvite.invitedUser().getId())).isEmpty();

        assertThat(authTokenRepository.findById(activeInvite.inviteToken().getId())).isPresent();
        assertThat(userRepository.findById(activeInvite.invitedUser().getId())).isPresent();
    }

    private TestInvite createInvite(String rawToken, Instant expiresAt) {
        Company company = companyRepository.saveAndFlush(new Company(
                "Expired Invite Cleanup Tenant " + rawToken,
                CompanyStatus.ACTIVE
        ));

        User superadmin = userRepository.findByEmail(SUPERADMIN_EMAIL).orElseThrow();

        User invitedUser = userRepository.saveAndFlush(new User(
                null,
                company,
                rawToken + "@miletos.local",
                null,
                "Invited",
                "User",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        ));

        AuthToken inviteToken = authTokenRepository.saveAndFlush(AuthToken.invite(
                company,
                invitedUser,
                superadmin,
                tokenHasher.hash(rawToken),
                expiresAt
        ));

        return new TestInvite(invitedUser, inviteToken);
    }

    private record TestInvite(
            User invitedUser,
            AuthToken inviteToken
    ) {
    }
}
