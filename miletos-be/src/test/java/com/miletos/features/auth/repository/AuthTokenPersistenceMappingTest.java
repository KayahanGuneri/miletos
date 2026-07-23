package com.miletos.features.auth.repository;

import static org.assertj.core.api.Assertions.assertThat;

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
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.jdbc.AutoConfigureTestDatabase;
import org.springframework.boot.test.autoconfigure.orm.jpa.DataJpaTest;
import org.springframework.jdbc.core.JdbcTemplate;

@DataJpaTest
@AutoConfigureTestDatabase(replace = AutoConfigureTestDatabase.Replace.NONE)
class AuthTokenPersistenceMappingTest extends PostgreSqlContainerSupport {

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private AuthTokenRepository authTokenRepository;

    @Autowired
    private JdbcTemplate jdbcTemplate;

    @Test
    void persistsInviteAndPasswordResetTokensInOneTable() {
        Company company = companyRepository.saveAndFlush(
                new Company(
                        "Generic Auth Token Tenant",
                        CompanyStatus.ACTIVE
                )
        );

        User superadmin = new User(
                null,
                null,
                "token-superadmin@miletos.local",
                "$2a$12$superadmin-password-hash",
                "Token",
                "Superadmin",
                null,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.NOT_REQUIRED,
                null,
                null,
                null,
                null
        );
        superadmin.setSuperAdmin(true);
        superadmin = userRepository.saveAndFlush(superadmin);

        User invitedUser = userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        "token-invited-user@miletos.local",
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
                )
        );

        User activeUser = userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        "token-active-user@miletos.local",
                        "$2a$12$active-password-hash",
                        "Active",
                        "User",
                        UserRole.USER,
                        false,
                        UserStatus.ACTIVE,
                        OnboardingStatus.COMPLETED,
                        null,
                        null,
                        null,
                        null
                )
        );

        AuthToken inviteToken = authTokenRepository.saveAndFlush(
                AuthToken.invite(
                        company,
                        invitedUser,
                        superadmin,
                        "generic-invite-token-hash",
                        Instant.now().plus(24, ChronoUnit.HOURS)
                )
        );

        AuthToken passwordResetToken = authTokenRepository.saveAndFlush(
                AuthToken.passwordReset(
                        activeUser,
                        "generic-password-reset-token-hash",
                        Instant.now().plus(30, ChronoUnit.MINUTES)
                )
        );

        assertThat(inviteToken.getId()).isNotNull();
        assertThat(inviteToken.getType())
                .isEqualTo(AuthTokenType.INVITE);
        assertThat(inviteToken.getCompany().getId())
                .isEqualTo(company.getId());
        assertThat(inviteToken.getUser().getId())
                .isEqualTo(invitedUser.getId());
        assertThat(inviteToken.getCreatedByUser().getId())
                .isEqualTo(superadmin.getId());

        assertThat(passwordResetToken.getId()).isNotNull();
        assertThat(passwordResetToken.getType())
                .isEqualTo(AuthTokenType.PASSWORD_RESET);
        assertThat(passwordResetToken.getCompany()).isNull();
        assertThat(passwordResetToken.getUser().getId())
                .isEqualTo(activeUser.getId());
        assertThat(passwordResetToken.getCreatedByUser()).isNull();

        assertThat(
                authTokenRepository.findByTokenHashAndType(
                        inviteToken.getTokenHash(),
                        AuthTokenType.INVITE
                )
        ).contains(inviteToken);

        assertThat(
                authTokenRepository.findByTokenHashAndType(
                        passwordResetToken.getTokenHash(),
                        AuthTokenType.PASSWORD_RESET
                )
        ).contains(passwordResetToken);
    }

    @Test
    void createsExpectedAuthTokenDatabaseContract() {
        Integer authTokenTableCount = jdbcTemplate.queryForObject(
                """
                SELECT COUNT(*)
                FROM information_schema.tables
                WHERE table_schema = 'public'
                  AND table_name = 'auth_tokens'
                """,
                Integer.class
        );

        Integer bigintIdentityPrimaryKeyCount =
                jdbcTemplate.queryForObject(
                        """
                        SELECT COUNT(*)
                        FROM information_schema.columns
                        WHERE table_schema = 'public'
                          AND table_name = 'auth_tokens'
                          AND column_name = 'id'
                          AND data_type = 'bigint'
                          AND is_identity = 'YES'
                        """,
                        Integer.class
                );

        Integer tokenTypeColumnCount = jdbcTemplate.queryForObject(
                """
                SELECT COUNT(*)
                FROM information_schema.columns
                WHERE table_schema = 'public'
                  AND table_name = 'auth_tokens'
                  AND column_name = 'type'
                  AND data_type = 'character varying'
                  AND is_nullable = 'NO'
                """,
                Integer.class
        );

        assertThat(authTokenTableCount).isEqualTo(1);
        assertThat(bigintIdentityPrimaryKeyCount).isEqualTo(1);
        assertThat(tokenTypeColumnCount).isEqualTo(1);
    }
}
