package com.miletos.features.workflowruntime.config;

import static org.assertj.core.api.Assertions.assertThat;

import java.net.URI;
import java.time.Duration;
import java.util.Set;

import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import jakarta.validation.ConstraintViolation;
import jakarta.validation.Validation;
import jakarta.validation.Validator;

class WorkflowRuntimePropertiesTest {

    private static Validator validator;

    @BeforeAll
    static void createValidator() {
        validator = Validation
                .buildDefaultValidatorFactory()
                .getValidator();
    }

    @Test
    void acceptsValidConfigurationAndNormalizesTrailingSlashes() {
        WorkflowRuntimeProperties properties =
                new WorkflowRuntimeProperties(
                        URI.create("https://runtime.internal/base///"),
                        "runtime-internal-service-token-32-bytes-minimum",
                        Duration.ofSeconds(3),
                        Duration.ofSeconds(35));

        assertThat(validator.validate(properties)).isEmpty();
        assertThat(properties.normalizedBaseUrl())
                .isEqualTo("https://runtime.internal/base");
    }

    @Test
    void rejectsShortInternalServiceToken() {
        assertThat(validationMessages(propertiesWithToken("too-short")))
                .contains(
                        "Runtime internal service token must contain at least 32 bytes and no whitespace");
    }

    @Test
    void permitsMissingTokenForControlledServiceUnavailableResponse() {
        WorkflowRuntimeProperties properties = propertiesWithToken("");

        assertThat(validator.validate(properties)).isEmpty();
        assertThat(properties.isConfigured()).isFalse();
    }

    @Test
    void rejectsWhitespaceInInternalServiceToken() {
        assertThat(validationMessages(propertiesWithToken(
                "runtime-internal-service-token-with whitespace")))
                .contains(
                        "Runtime internal service token must contain at least 32 bytes and no whitespace");
    }

    @Test
    void rejectsUnsafeOrInvalidRuntimeUrl() {
        WorkflowRuntimeProperties properties =
                new WorkflowRuntimeProperties(
                        URI.create("ftp://user:password@runtime.internal/base?secret=value"),
                        "runtime-internal-service-token-32-bytes-minimum",
                        Duration.ofSeconds(3),
                        Duration.ofSeconds(35));

        assertThat(validationMessages(properties))
                .contains(
                        "Runtime base URL must be an absolute HTTP(S) URL without credentials, query, or fragment");
    }

    private WorkflowRuntimeProperties propertiesWithToken(String token) {
        return new WorkflowRuntimeProperties(
                URI.create("http://runtime.internal"),
                token,
                Duration.ofSeconds(3),
                Duration.ofSeconds(35));
    }

    private Set<String> validationMessages(
            WorkflowRuntimeProperties properties) {
        return validator
                .validate(properties)
                .stream()
                .map(ConstraintViolation::getMessage)
                .collect(java.util.stream.Collectors.toSet());
    }
}
