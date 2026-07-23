package com.miletos.config;

import static org.assertj.core.api.Assertions.assertThat;

import jakarta.validation.Validation;
import jakarta.validation.Validator;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;
import org.springframework.boot.context.properties.bind.Bindable;
import org.springframework.boot.context.properties.bind.Binder;
import org.springframework.boot.context.properties.source.MapConfigurationPropertySource;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.web.cors.CorsConfiguration;
import com.miletos.security.SecurityConfig;

class CorsPropertiesTest {

    private final Validator validator = Validation.buildDefaultValidatorFactory().getValidator();

    @Test
    void bindsCommaSeparatedOriginsAndPolicyFields() {
        MapConfigurationPropertySource source = new MapConfigurationPropertySource(Map.of(
                "miletos.security.cors.allowed-origins", "http://localhost:3000,http://127.0.0.1:3000",
                "miletos.security.cors.allowed-methods", "GET,POST,OPTIONS",
                "miletos.security.cors.allowed-headers", "Authorization,Content-Type",
                "miletos.security.cors.exposed-headers", "Location",
                "miletos.security.cors.allow-credentials", "false",
                "miletos.security.cors.max-age", "3600"
        ));

        CorsProperties properties = new Binder(source)
                .bind("miletos.security.cors", Bindable.of(CorsProperties.class))
                .orElseThrow(() -> new AssertionError("CORS properties were not bound"));

        assertThat(properties.allowedOrigins())
                .containsExactly("http://localhost:3000", "http://127.0.0.1:3000");
        assertThat(properties.allowedMethods()).containsExactly("GET", "POST", "OPTIONS");
        assertThat(properties.allowedHeaders()).containsExactly("Authorization", "Content-Type");
        assertThat(properties.exposedHeaders()).containsExactly("Location");
        assertThat(properties.allowCredentials()).isFalse();
        assertThat(properties.maxAge()).isEqualTo(3600L);
        assertThat(validator.validate(properties)).isEmpty();
    }

    @Test
    void rejectsWildcardOriginWhenCredentialsAreEnabled() {
        CorsProperties properties = properties(List.of("*"), true);

        assertThat(validator.validate(properties))
                .anyMatch(violation -> violation.getPropertyPath().toString()
                        .equals("credentialConfigurationSafe"));
    }

    @Test
    void rejectsEmptyAndMalformedOrigins() {
        assertThat(validator.validate(properties(List.of(), false))).isNotEmpty();
        assertThat(validator.validate(properties(List.of("localhost:3000"), false))).isNotEmpty();
    }

    @Test
    void securityConfigurationUsesEveryConfiguredPolicyField() {
        CorsProperties properties = new CorsProperties(
                List.of("https://app.miletos.test"),
                List.of("GET", "OPTIONS"),
                List.of("Authorization"),
                List.of("Location"),
                true,
                900L
        );
        SecurityConfig securityConfig = new SecurityConfig(null, null, properties);

        CorsConfiguration configuration = securityConfig
                .corsConfigurationSource()
                .getCorsConfiguration(new MockHttpServletRequest("GET", "/api/profile"));

        assertThat(configuration).isNotNull();
        assertThat(configuration.getAllowedOrigins()).containsExactly("https://app.miletos.test");
        assertThat(configuration.getAllowedMethods()).containsExactly("GET", "OPTIONS");
        assertThat(configuration.getAllowedHeaders()).containsExactly("Authorization");
        assertThat(configuration.getExposedHeaders()).containsExactly("Location");
        assertThat(configuration.getAllowCredentials()).isTrue();
        assertThat(configuration.getMaxAge()).isEqualTo(900L);
    }

    private CorsProperties properties(List<String> origins, boolean allowCredentials) {
        return new CorsProperties(
                origins,
                List.of("GET", "OPTIONS"),
                List.of("Authorization"),
                List.of("Location"),
                allowCredentials,
                3600L
        );
    }
}
