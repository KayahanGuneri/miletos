package com.miletos.security;

import static org.hamcrest.Matchers.blankOrNullString;
import static org.hamcrest.Matchers.hasItem;
import static org.hamcrest.Matchers.not;
import static org.hamcrest.Matchers.nullValue;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
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
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.test.web.servlet.MockMvc;

@SpringBootTest(properties = {
        "miletos.security.jwt.secret=test-security-secret-key-for-miletos-auth-flow-32-bytes-minimum",
        "miletos.security.jwt.issuer=https://miletos-security-test.local",
        "miletos.security.jwt.access-token-minutes=30"
})
@AutoConfigureMockMvc
class SecurityIntegrationTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL =
            "security-superadmin@miletos.local";

    private static final String SUPERADMIN_PASSWORD =
            "Security123!";

    private static final String COMPANY_ADMIN_EMAIL =
            "security-company-admin@miletos.local";

    private static final String COMPANY_ADMIN_PASSWORD =
            "CompanyAdmin123!";

    private static final String PRINCIPAL_USER_EMAIL =
            "security-principal-user@miletos.local";

    private static final String PRINCIPAL_USER_PASSWORD =
            "PrincipalUser123!";

    private static final String SUPERADMIN_FORBIDDEN_USER_EMAIL =
            "security-superadmin-forbidden-user@miletos.local";

    private static final String SUPERADMIN_FORBIDDEN_USER_PASSWORD =
            "SuperadminForbidden123!";

    private static final String COMPANY_ADMIN_FORBIDDEN_USER_EMAIL =
            "security-company-admin-forbidden-user@miletos.local";

    private static final String COMPANY_ADMIN_FORBIDDEN_USER_PASSWORD =
            "CompanyAdminForbidden123!";

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private PasswordEncoder passwordEncoder;

    @BeforeEach
    void createRequiredSuperadmin() {
        if (userRepository.findByEmail(SUPERADMIN_EMAIL).isPresent()) {
            return;
        }

        userRepository.saveAndFlush(new User(
                null,
                null,
                SUPERADMIN_EMAIL,
                passwordEncoder.encode(SUPERADMIN_PASSWORD),
                "Security",
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
    void loginEndpointReturnsBearerToken() throws Exception {
        mockMvc.perform(post("/api/auth/login")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "email": "security-superadmin@miletos.local",
                                  "password": "Security123!"
                                }
                                """))
                .andExpect(status().isOk())
                .andExpect(
                        jsonPath(
                                "$.accessToken",
                                not(blankOrNullString())
                        )
                )
                .andExpect(
                        jsonPath("$.tokenType")
                                .value("Bearer")
                )
                .andExpect(
                        jsonPath("$.user.email")
                                .value(SUPERADMIN_EMAIL)
                )
                .andExpect(
                        jsonPath("$.user.role")
                                .value(nullValue())
                )
                .andExpect(
                        jsonPath("$.user.superAdmin")
                                .value(true)
                );
    }

    @Test
    void protectedEndpointRejectsMissingBearerToken()
            throws Exception {
        mockMvc.perform(post("/api/companies")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "name": "Unauthorized Security Tenant"
                                }
                                """))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("UNAUTHENTICATED"));
    }

    @Test
    void workflowRuntimeEndpointRejectsMissingBearerToken()
            throws Exception {
        mockMvc.perform(get("/api/v1/executions"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("UNAUTHENTICATED"));
    }

    @Test
    void authenticatedUserWithoutCompanyCannotUseRuntimeGateway()
            throws Exception {
        String accessToken =
                loginAndExtractAccessToken(
                        SUPERADMIN_EMAIL,
                        SUPERADMIN_PASSWORD
                );

        mockMvc.perform(get("/api/v1/executions")
                        .header(
                                "Authorization",
                                "Bearer " + accessToken
                        ))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code")
                        .value("COMPANY_CONTEXT_REQUIRED"));
    }

    @Test
    void protectedEndpointRejectsMalformedBearerTokenWithStableJson() throws Exception {
        mockMvc.perform(get("/api/users/me")
                        .header("Authorization", "Bearer not-a-jwt"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("MALFORMED_TOKEN"))
                .andExpect(jsonPath("$.path").value("/api/users/me"));
    }

    @Test
    void superadminPermissionAcceptsValidBearerToken()
            throws Exception {
        String accessToken =
                loginAndExtractAccessToken(
                        SUPERADMIN_EMAIL,
                        SUPERADMIN_PASSWORD
                );

        mockMvc.perform(post("/api/companies")
                        .header(
                                "Authorization",
                                "Bearer " + accessToken
                        )
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "name": "Authorized Security Tenant"
                                }
                                """))
                .andExpect(status().isCreated())
                .andExpect(
                        jsonPath("$.name")
                                .value(
                                        "Authorized Security Tenant"
                                )
                )
                .andExpect(
                        jsonPath("$.status")
                                .value("ACTIVE")
                );
    }

    @Test
    void authenticatedEndpointReceivesJwtSubjectAsActorEmail()
            throws Exception {
        Company company =
                createCompany(
                        "Security Principal Tenant"
                );

        createActiveUser(
                company,
                PRINCIPAL_USER_EMAIL,
                PRINCIPAL_USER_PASSWORD,
                UserRole.USER
        );

        String accessToken =
                loginAndExtractAccessToken(
                        PRINCIPAL_USER_EMAIL,
                        PRINCIPAL_USER_PASSWORD
                );

        mockMvc.perform(get("/api/users/me")
                        .header(
                                "Authorization",
                                "Bearer " + accessToken
                        ))
                .andExpect(status().isOk())
                .andExpect(
                        jsonPath("$.email")
                                .value(PRINCIPAL_USER_EMAIL)
                )
                .andExpect(
                        jsonPath("$.role")
                                .value("USER")
                )
                .andExpect(
                        jsonPath("$.superAdmin")
                                .value(false)
                );
    }

    @Test
    void regularUserIsForbiddenFromSuperadminEndpoint()
            throws Exception {
        Company company =
                createCompany(
                        "Security Regular Permission Tenant"
                );

        createActiveUser(
                company,
                SUPERADMIN_FORBIDDEN_USER_EMAIL,
                SUPERADMIN_FORBIDDEN_USER_PASSWORD,
                UserRole.USER
        );

        String accessToken =
                loginAndExtractAccessToken(
                        SUPERADMIN_FORBIDDEN_USER_EMAIL,
                        SUPERADMIN_FORBIDDEN_USER_PASSWORD
                );

        mockMvc.perform(post("/api/companies")
                        .header(
                                "Authorization",
                                "Bearer " + accessToken
                        )
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "name": "Forbidden Security Tenant"
                                }
                                """))
                .andExpect(status().isForbidden())
                .andExpect(
                        jsonPath("$.code")
                                .value("FORBIDDEN")
                );
    }

    @Test
    void companyAdminCanAccessOwnCompanyUsers()
            throws Exception {
        Company company =
                createCompany(
                        "Security Company Admin Tenant"
                );

        createActiveUser(
                company,
                COMPANY_ADMIN_EMAIL,
                COMPANY_ADMIN_PASSWORD,
                UserRole.ADMIN
        );

        createActiveUser(
                company,
                "security-company-member@miletos.local",
                "CompanyMember123!",
                UserRole.USER
        );

        String accessToken =
                loginAndExtractAccessToken(
                        COMPANY_ADMIN_EMAIL,
                        COMPANY_ADMIN_PASSWORD
                );

        mockMvc.perform(
                        get(
                                "/api/companies/{companyId}/users",
                                company.getId()
                        )
                                .header(
                                        "Authorization",
                                        "Bearer " + accessToken
                                )
                )
                .andExpect(status().isOk())
                .andExpect(
                        jsonPath(
                                "$[*].email",
                                hasItem(COMPANY_ADMIN_EMAIL)
                        )
                )
                .andExpect(
                        jsonPath(
                                "$[*].email",
                                hasItem(
                                        "security-company-member@miletos.local"
                                )
                        )
                );
    }

    @Test
    void regularUserIsForbiddenFromCompanyAdminEndpoint()
            throws Exception {
        Company company =
                createCompany(
                        "Security Company Permission Tenant"
                );

        createActiveUser(
                company,
                COMPANY_ADMIN_FORBIDDEN_USER_EMAIL,
                COMPANY_ADMIN_FORBIDDEN_USER_PASSWORD,
                UserRole.USER
        );

        String accessToken =
                loginAndExtractAccessToken(
                        COMPANY_ADMIN_FORBIDDEN_USER_EMAIL,
                        COMPANY_ADMIN_FORBIDDEN_USER_PASSWORD
                );

        mockMvc.perform(
                        get(
                                "/api/companies/{companyId}/users",
                                company.getId()
                        )
                                .header(
                                        "Authorization",
                                        "Bearer " + accessToken
                                )
                )
                .andExpect(status().isForbidden())
                .andExpect(
                        jsonPath("$.code")
                                .value("FORBIDDEN")
                );
    }

    @Test
    void protectedEndpointRejectsSupersededBearerToken()
            throws Exception {
        String oldAccessToken =
                loginAndExtractAccessToken(
                        SUPERADMIN_EMAIL,
                        SUPERADMIN_PASSWORD
                );

        loginAndExtractAccessToken(
                SUPERADMIN_EMAIL,
                SUPERADMIN_PASSWORD
        );

        mockMvc.perform(post("/api/companies")
                        .header(
                                "Authorization",
                                "Bearer " + oldAccessToken
                        )
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "name": "Rejected Superseded Token Tenant"
                                }
                                """))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value("INACTIVE_SESSION"));
    }

    private Company createCompany(String name) {
        return companyRepository.saveAndFlush(
                new Company(
                        name,
                        CompanyStatus.ACTIVE
                )
        );
    }

    private User createActiveUser(
            Company company,
            String email,
            String rawPassword,
            UserRole role
    ) {
        return userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        email,
                        passwordEncoder.encode(rawPassword),
                        "Security",
                        "User",
                        role,
                        false,
                        UserStatus.ACTIVE,
                        OnboardingStatus.COMPLETED,
                        null,
                        null,
                        null,
                        null
                )
        );
    }

    private String loginAndExtractAccessToken(
            String email,
            String password
    ) throws Exception {
        String requestBody =
                objectMapper.writeValueAsString(
                        new LoginRequestBody(
                                email,
                                password
                        )
                );

        String responseBody =
                mockMvc.perform(post("/api/auth/login")
                                .contentType(
                                        MediaType.APPLICATION_JSON
                                )
                                .content(requestBody))
                        .andExpect(status().isOk())
                        .andReturn()
                        .getResponse()
                        .getContentAsString();

        JsonNode responseJson =
                objectMapper.readTree(responseBody);

        return responseJson
                .get("accessToken")
                .asText();
    }

    private record LoginRequestBody(
            String email,
            String password
    ) {
    }
}
