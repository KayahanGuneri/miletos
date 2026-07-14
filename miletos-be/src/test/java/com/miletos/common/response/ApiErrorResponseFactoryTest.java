package com.miletos.common.response;

import static org.assertj.core.api.Assertions.assertThat;

import com.miletos.common.exception.ErrorCode;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.Map;
import org.junit.jupiter.api.Test;

class ApiErrorResponseFactoryTest {

    private static final Instant FIXED_TIME = Instant.parse("2026-07-07T15:00:00Z");

    private final ApiErrorResponseFactory factory = new ApiErrorResponseFactory(
            Clock.fixed(FIXED_TIME, ZoneOffset.UTC)
    );

    @Test
    void createsErrorResponseWithoutFieldErrors() {
        ApiErrorResponse response = factory.create(
                ErrorCode.FORBIDDEN,
                "Access is denied",
                "/api/users"
        );

        assertThat(response.code()).isEqualTo(ErrorCode.FORBIDDEN);
        assertThat(response.message()).isEqualTo("Access is denied");
        assertThat(response.path()).isEqualTo("/api/users");
        assertThat(response.timestamp()).isEqualTo(FIXED_TIME);
        assertThat(response.fieldErrors()).isEmpty();
    }

    @Test
    void createsErrorResponseWithFieldErrors() {
        ApiErrorResponse response = factory.create(
                ErrorCode.VALIDATION_FAILED,
                "Request validation failed",
                "/api/auth/login",
                Map.of("email", "must not be blank")
        );

        assertThat(response.code()).isEqualTo(ErrorCode.VALIDATION_FAILED);
        assertThat(response.fieldErrors()).containsEntry("email", "must not be blank");
    }
}