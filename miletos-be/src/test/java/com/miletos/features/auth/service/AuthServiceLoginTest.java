package com.miletos.features.auth.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import com.miletos.features.auth.model.LoginResult;
import com.miletos.features.auth.command.LoginCommand;
import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.repository.UserRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.JwtException;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest(properties = {
        "miletos.security.jwt.secret=test-jwt-secret-key-for-miletos-auth-flow-32-bytes-minimum",
        "miletos.security.jwt.issuer=https://miletos-test.local",
        "miletos.security.jwt.access-token-minutes=30"
})
@Transactional
class AuthServiceLoginTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL = "jwt-superadmin@miletos.local";

    @Autowired
    private AuthService authService;

    @Autowired
    private PasswordEncoder passwordEncoder;

    @Autowired
    private JwtDecoder jwtDecoder;

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
                passwordEncoder.encode("Jwt123456!"),
                "Jwt",
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
    void logsInActiveUserAndReturnsJwtToken() {
        LoginResult result = authService.login(new LoginCommand(SUPERADMIN_EMAIL, "Jwt123456!"));

        Jwt jwt = jwtDecoder.decode(result.accessToken().value());

        assertThat(result.user().getEmail()).isEqualTo(SUPERADMIN_EMAIL);
        assertThat(result.user().isSuperAdmin()).isTrue();
        assertThat(result.user().getRole()).isNull();
        assertThat(result.accessToken().value()).isNotBlank();
        assertThat(result.accessToken().expiresAt()).isNotNull();

        assertThat(jwt.getSubject()).isEqualTo(SUPERADMIN_EMAIL);
        assertThat(jwt.getIssuer().toString()).isEqualTo("https://miletos-test.local");
        assertThat(jwt.getId()).isNotBlank();
        assertThat(jwt.getClaims()).containsOnlyKeys("iss", "sub", "iat", "exp", "jti");
    }

    @Test
    void secondLoginInvalidatesPreviousAccessToken() {
        LoginResult firstLogin = authService.login(new LoginCommand(SUPERADMIN_EMAIL, "Jwt123456!"));
        LoginResult secondLogin = authService.login(new LoginCommand(SUPERADMIN_EMAIL, "Jwt123456!"));

        Jwt secondJwt = jwtDecoder.decode(secondLogin.accessToken().value());

        assertThat(secondJwt.getId()).isNotBlank();
        assertThatThrownBy(() -> jwtDecoder.decode(firstLogin.accessToken().value()))
                .isInstanceOf(JwtException.class);
    }

    @Test
    void rejectsInvalidPassword() {
        assertThatThrownBy(() -> authService.login(new LoginCommand(SUPERADMIN_EMAIL, "WrongPassword123!")))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.INVALID_CREDENTIALS)
                );
    }

    @Test
    void rejectsUnknownEmail() {
        assertThatThrownBy(() -> authService.login(new LoginCommand("unknown@miletos.local", "Jwt123456!")))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.INVALID_CREDENTIALS)
                );
    }

    @Test
    void rejectsUserWaitingForPasswordCompletion() {
        Company company = companyRepository.saveAndFlush(new Company("Jwt Pending Tenant", CompanyStatus.ACTIVE));

        userRepository.saveAndFlush(new User(
                null,
                company,
                "pending-login@miletos.local",
                null,
                "Pending",
                "Login",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        ));

        assertThatThrownBy(() -> authService.login(new LoginCommand("pending-login@miletos.local", "Password123!")))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.PASSWORD_SETUP_REQUIRED)
                );
    }

    @Test
    void rejectsDisabledUser() {
        Company company = companyRepository.saveAndFlush(new Company("Jwt Disabled Tenant", CompanyStatus.ACTIVE));

        userRepository.saveAndFlush(new User(
                null,
                company,
                "disabled-login@miletos.local",
                passwordEncoder.encode("Password123!"),
                "Disabled",
                "Login",
                UserRole.USER,
                false,
                UserStatus.DISABLED,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        ));

        assertThatThrownBy(() -> authService.login(new LoginCommand("disabled-login@miletos.local", "Password123!")))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.USER_DISABLED)
                );
    }
}
