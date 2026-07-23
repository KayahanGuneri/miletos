package com.miletos.features.auth;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Instant;

import org.junit.jupiter.api.Test;
import org.mapstruct.factory.Mappers;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.miletos.features.auth.model.JwtToken;
import com.miletos.features.auth.model.LoginResult;
import com.miletos.features.auth.request.CompletePasswordRequest;
import com.miletos.features.auth.request.ForgotPasswordRequest;
import com.miletos.features.auth.request.LoginRequest;
import com.miletos.features.auth.request.ResetPasswordRequest;
import com.miletos.features.auth.response.TokenType;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;

class AuthMapperTest {

    private final AuthMapper mapper = Mappers.getMapper(AuthMapper.class);

    @Test
    void trimsLoginForgotPasswordAndPasswordTokenInputs() {
        var login = mapper.toLoginCommand(new LoginRequest(" user@example.com ", "Password123!"));
        var forgot = mapper.toForgotPasswordCommand(new ForgotPasswordRequest(" user@example.com "));
        var complete = mapper.toCompletePasswordCommand(new CompletePasswordRequest(" token ", "Password123!"));
        var reset = mapper.toResetPasswordCommand(new ResetPasswordRequest(" token ", "Password123!"));

        assertThat(login.email()).isEqualTo("user@example.com");
        assertThat(forgot.email()).isEqualTo("user@example.com");
        assertThat(complete.token()).isEqualTo("token");
        assertThat(reset.token()).isEqualTo("token");
    }

    @Test
    void mapsNestedLoginResponseAndCanonicalEnums() {
        Company company = new Company("Acme", CompanyStatus.ACTIVE);
        company.setId(41L);
        User user = new User(
                null,
                company,
                "user@example.com",
                "hash",
                "Ada",
                "Lovelace",
                UserRole.ADMIN,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null);
        user.setId(7L);

        Instant expiresAt = Instant.parse("2026-07-22T12:00:00Z");
        var response = mapper.toLoginResponse(
                new LoginResult(user, new JwtToken("access-token", expiresAt)));

        assertThat(response.accessToken()).isEqualTo("access-token");
        assertThat(response.tokenType()).isEqualTo(TokenType.BEARER);
        assertThat(response.expiresAt()).isEqualTo(expiresAt);
        assertThat(response.user().companyId()).isEqualTo(41L);
        assertThat(response.user().role()).isEqualTo(UserRole.ADMIN);
        assertThat(response.user().status()).isEqualTo(UserStatus.ACTIVE);
        assertThat(response.user().onboardingStatus()).isEqualTo(OnboardingStatus.COMPLETED);
    }

    @Test
    void mapsNullCompanyForSuperadmin() {
        User user = new User(
                null,
                null,
                "superadmin@example.com",
                "hash",
                "System",
                "Admin",
                null,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.NOT_REQUIRED,
                null,
                null,
                null,
                null);
        user.setId(1L);
        user.setSuperAdmin(true);

        var response = mapper.toAuthenticatedUserResponse(user);

        assertThat(response.companyId()).isNull();
        assertThat(response.role()).isNull();
        assertThat(response.superAdmin()).isTrue();
    }

    @Test
    void serializesBearerTokenTypeWithCompatibleJson() throws JsonProcessingException {
        assertThat(new ObjectMapper().writeValueAsString(TokenType.BEARER)).isEqualTo("\"Bearer\"");
    }
}
