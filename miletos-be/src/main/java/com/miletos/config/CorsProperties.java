package com.miletos.config;

import java.util.List;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.validation.annotation.Validated;

import jakarta.validation.constraints.AssertTrue;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotEmpty;
import jakarta.validation.constraints.Pattern;
import jakarta.validation.constraints.Positive;

@Validated
@ConfigurationProperties(prefix = "miletos.security.cors")
public record CorsProperties(
        @NotEmpty List<@Pattern(regexp = "\\*|https?://[A-Za-z0-9.-]+(?::\\d+)?") String> allowedOrigins,
        @NotEmpty List<@Pattern(regexp = "GET|POST|PUT|PATCH|DELETE|OPTIONS") String> allowedMethods,
        @NotEmpty List<@NotBlank String> allowedHeaders,
        List<@NotBlank String> exposedHeaders,
        boolean allowCredentials,
        @Positive long maxAge) {

    @AssertTrue(message = "Wildcard CORS origins are not allowed when credentials are enabled")
    public boolean isCredentialConfigurationSafe() {
        return !allowCredentials || !allowedOrigins.contains("*");
    }
}
