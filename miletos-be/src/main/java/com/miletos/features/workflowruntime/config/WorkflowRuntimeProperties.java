package com.miletos.features.workflowruntime.config;

import java.nio.charset.StandardCharsets;
import java.time.Duration;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.validation.annotation.Validated;

import jakarta.validation.constraints.AssertTrue;
import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;

@Validated
@ConfigurationProperties(prefix = "miletos.runtime")
public record WorkflowRuntimeProperties(
        @NotBlank String grpcHost,
        @Min(1) @Max(65535) int grpcPort,
        @NotBlank String internalServiceToken,
        @NotNull Duration requestTimeout) {

    private static final int MINIMUM_TOKEN_BYTES = 32;

    @AssertTrue(message = "Runtime internal service token must contain at least 32 bytes and no whitespace")
    public boolean isInternalServiceTokenValid() {
        if (internalServiceToken == null
                || internalServiceToken.getBytes(StandardCharsets.UTF_8).length
                        < MINIMUM_TOKEN_BYTES) {
            return false;
        }
        return internalServiceToken.codePoints().noneMatch(Character::isWhitespace);
    }

    @AssertTrue(message = "Runtime request timeout must be greater than zero")
    public boolean isRequestTimeoutValid() {
        return requestTimeout != null
                && !requestTimeout.isZero()
                && !requestTimeout.isNegative();
    }
}
