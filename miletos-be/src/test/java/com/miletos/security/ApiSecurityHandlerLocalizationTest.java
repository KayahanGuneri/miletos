package com.miletos.security;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.miletos.common.response.ApiErrorResponseFactory;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.Locale;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;
import org.springframework.security.access.AccessDeniedException;
import org.springframework.security.authentication.InsufficientAuthenticationException;
import org.springframework.security.oauth2.server.resource.BearerTokenError;
import org.springframework.security.oauth2.core.OAuth2AuthenticationException;
import org.springframework.http.HttpStatus;

class ApiSecurityHandlerLocalizationTest {

    private ObjectMapper objectMapper;
    private ApiAuthenticationEntryPoint authenticationEntryPoint;
    private ApiAccessDeniedHandler accessDeniedHandler;

    @BeforeEach
    void setUp() {
        objectMapper =
                new ObjectMapper()
                        .findAndRegisterModules();

        Clock clock =
                Clock.fixed(
                        Instant.parse(
                                "2026-07-11T20:00:00Z"
                        ),
                        ZoneOffset.UTC
                );

        ApiErrorResponseFactory responseFactory =
                new ApiErrorResponseFactory(clock);

        authenticationEntryPoint =
                new ApiAuthenticationEntryPoint(
                        objectMapper,
                        responseFactory
                );

        accessDeniedHandler =
                new ApiAccessDeniedHandler(
                        objectMapper,
                        responseFactory
                );
    }

    @Test
    void returnsStableUnauthorizedContractRegardlessOfLocale()
            throws Exception {
        MockHttpServletRequest request =
                createRequest(
                        Locale.forLanguageTag("tr-TR")
                );

        MockHttpServletResponse response =
                new MockHttpServletResponse();

        authenticationEntryPoint.commence(
                request,
                response,
                new InsufficientAuthenticationException(
                        "Authentication required"
                )
        );

        JsonNode responseBody =
                objectMapper.readTree(
                        response.getContentAsString()
                );

        assertThat(response.getStatus())
                .isEqualTo(401);

        assertThat(response.getContentType())
                .startsWith("application/json");

        assertThat(responseBody.get("code").asText())
                .isEqualTo("UNAUTHENTICATED");

        assertThat(responseBody.get("message").asText())
                .isEqualTo("UNAUTHENTICATED");

        assertThat(responseBody.get("path").asText())
                .isEqualTo("/api/protected");
    }

    @Test
    void returnsStableForbiddenContractRegardlessOfLocale()
            throws Exception {
        MockHttpServletRequest request =
                createRequest(Locale.ENGLISH);

        MockHttpServletResponse response =
                new MockHttpServletResponse();

        accessDeniedHandler.handle(
                request,
                response,
                new AccessDeniedException("Forbidden")
        );

        JsonNode responseBody =
                objectMapper.readTree(
                        response.getContentAsString()
                );

        assertThat(response.getStatus())
                .isEqualTo(403);

        assertThat(responseBody.get("code").asText())
                .isEqualTo("FORBIDDEN");

        assertThat(responseBody.get("message").asText())
                .isEqualTo("FORBIDDEN");

        assertThat(responseBody.get("path").asText())
                .isEqualTo("/api/protected");
    }

    @Test
    void classifiesStableBearerTokenErrors() {
        assertThat(resolveBearerError("invalid_token", "Jwt expired at 2026-07-18"))
                .isEqualTo("EXPIRED_TOKEN");
        assertThat(resolveBearerError("invalid_token", "Bearer token is malformed"))
                .isEqualTo("MALFORMED_TOKEN");
        assertThat(resolveBearerError("inactive_session", "JWT session is not active"))
                .isEqualTo("INACTIVE_SESSION");
        assertThat(resolveBearerError("invalid_token", "JWT signature is invalid"))
                .isEqualTo("INVALID_TOKEN");
    }

    private String resolveBearerError(String code, String description) {
        BearerTokenError error = new BearerTokenError(
                code,
                HttpStatus.UNAUTHORIZED,
                description,
                null
        );
        return authenticationEntryPoint.resolveErrorCode(
                new OAuth2AuthenticationException(error)
        ).name();
    }

    private MockHttpServletRequest createRequest(
            Locale locale
    ) {
        MockHttpServletRequest request =
                new MockHttpServletRequest();

        request.setRequestURI("/api/protected");
        request.addPreferredLocale(locale);

        return request;
    }
}
