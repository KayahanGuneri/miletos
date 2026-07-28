package com.miletos.features.workflowruntime.config;

import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Duration;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.validation.annotation.Validated;

import jakarta.validation.constraints.AssertTrue;
import jakarta.validation.constraints.NotNull;

@Validated
@ConfigurationProperties(prefix = "miletos.runtime")
public record WorkflowRuntimeProperties(
        @NotNull URI baseUrl,
        String internalServiceToken,
        @NotNull Duration connectTimeout,
        @NotNull Duration readTimeout) {

    private static final int MINIMUM_TOKEN_BYTES = 32;

    @AssertTrue(message = "Runtime base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
    public boolean isBaseUrlValid() {
        if (baseUrl == null
                || !baseUrl.isAbsolute()
                || baseUrl.getHost() == null
                || baseUrl.getUserInfo() != null
                || baseUrl.getQuery() != null
                || baseUrl.getFragment() != null) {
            return false;
        }

        return "http".equalsIgnoreCase(baseUrl.getScheme())
                || "https".equalsIgnoreCase(baseUrl.getScheme());
    }

    @AssertTrue(message = "Runtime internal service token must contain at least 32 bytes and no whitespace")
    public boolean isInternalServiceTokenValid() {
        if (!isConfigured()) {
            return true;
        }

        if (internalServiceToken.getBytes(StandardCharsets.UTF_8).length < MINIMUM_TOKEN_BYTES) {
            return false;
        }

        return internalServiceToken.codePoints().noneMatch(Character::isWhitespace);
    }

    @AssertTrue(message = "Runtime timeouts must be greater than zero")
    public boolean areTimeoutsValid() {
        return isPositive(connectTimeout) && isPositive(readTimeout);
    }

    public String normalizedBaseUrl() {
        String value = baseUrl.toString();

        while (value.endsWith("/")) {
            value = value.substring(0, value.length() - 1);
        }

        return value;
    }

    public boolean isConfigured() {
        return internalServiceToken != null
                && !internalServiceToken.isEmpty();
    }

    private boolean isPositive(Duration duration) {
        return duration != null && !duration.isZero() && !duration.isNegative();
    }
}
