package com.miletos.features.auth.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.auth.command.ChangePasswordCommand;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest(properties = {
        "miletos.security.jwt.secret="
                + "test-change-secret-key-for-miletos-auth-flow-"
                + "32-bytes-minimum",
        "miletos.security.jwt.issuer="
                + "https://miletos-change-test.local",
        "miletos.security.jwt.access-token-minutes=30"
})
@Transactional
class AuthServiceChangePasswordTest
        extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL =
            "change-password-superadmin@miletos.local";

    @Autowired
    private AuthService authService;

    @Autowired
    private PasswordEncoder passwordEncoder;

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @BeforeEach
    void createRequiredSuperadmin() {
        if (userRepository.findByEmail(SUPERADMIN_EMAIL).isPresent()) {
            return;
        }

        userRepository.saveAndFlush(new User(
                null,
                null,
                SUPERADMIN_EMAIL,
                passwordEncoder.encode("Change123!"),
                "Change",
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
    void changesPasswordForActiveUser() {
        authService.changePassword(
                new ChangePasswordCommand(
                        SUPERADMIN_EMAIL,
                        "Change123!",
                        "NewChangePassword123!"
                )
        );

        User updatedUser = userRepository
                .findByEmail(SUPERADMIN_EMAIL)
                .orElseThrow();

        assertThat(
                passwordEncoder.matches(
                        "NewChangePassword123!",
                        updatedUser.getPasswordHash()
                )
        ).isTrue();

        assertThat(
                passwordEncoder.matches(
                        "Change123!",
                        updatedUser.getPasswordHash()
                )
        ).isFalse();
    }

    @Test
    void rejectsInvalidCurrentPassword() {
        assertThatThrownBy(() ->
                authService.changePassword(
                        new ChangePasswordCommand(
                                SUPERADMIN_EMAIL,
                                "WrongPassword123!",
                                "NewChangePassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.INVALID_CREDENTIALS
                        )
                );
    }

    @Test
    void rejectsUnknownActor() {
        assertThatThrownBy(() ->
                authService.changePassword(
                        new ChangePasswordCommand(
                                "unknown-change@miletos.local",
                                "Change123!",
                                "NewChangePassword123!"
                        )
                )
        )
                .isInstanceOfSatisfying(
                        MiletosException.class,
                        exception -> assertThat(
                                exception.getCode()
                        ).isEqualTo(
                                ErrorCode.AUTHENTICATED_USER_NOT_FOUND
                        )
                );
    }

    @Test
    void rejectsPendingUser() {
        Company company = companyRepository.saveAndFlush(
                new Company(
                        "Change Pending Tenant",
                        CompanyStatus.ACTIVE
                )
        );

        userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        "pending-change@miletos.local",
                        null,
                        "Pending",
                        "Change",
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

        assertThatThrownBy(() ->
                authService.changePassword(
                        new ChangePasswordCommand(
                                "pending-change@miletos.local",
                                "CurrentPassword123!",
                                "NewChangePassword123!"
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

    @Test
    void rejectsDisabledUser() {
        Company company = companyRepository.saveAndFlush(
                new Company(
                        "Change Disabled Tenant",
                        CompanyStatus.ACTIVE
                )
        );

        userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        "disabled-change@miletos.local",
                        passwordEncoder.encode(
                                "CurrentPassword123!"
                        ),
                        "Disabled",
                        "Change",
                        UserRole.USER,
                        false,
                        UserStatus.DISABLED,
                        OnboardingStatus.COMPLETED,
                        null,
                        null,
                        null,
                        null
                )
        );

        assertThatThrownBy(() ->
                authService.changePassword(
                        new ChangePasswordCommand(
                                "disabled-change@miletos.local",
                                "CurrentPassword123!",
                                "NewChangePassword123!"
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
}
