package com.miletos.common.exception;

import static org.assertj.core.api.Assertions.assertThat;

import com.miletos.common.response.ApiErrorResponse;
import com.miletos.common.response.ApiErrorResponseFactory;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.mock.web.MockHttpServletRequest;

class MiletosExceptionHandlerTest {

    @Test
    void returnsStableCodeWithoutInternalMessage() {
        Clock clock =
                Clock.fixed(
                        Instant.parse(
                                "2026-07-15T10:00:00Z"
                        ),
                        ZoneOffset.UTC
                );

        GlobalExceptionHandler handler =
                new GlobalExceptionHandler(
                        new ApiErrorResponseFactory(clock)
                );

        MockHttpServletRequest request =
                new MockHttpServletRequest();

        request.setRequestURI("/api/test");

        ResponseEntity<ApiErrorResponse> response =
                handler.handleMiletosException(
                        new TestMiletosException(),
                        request
                );

        assertThat(response.getStatusCode())
                .isEqualTo(HttpStatus.NOT_FOUND);

        assertThat(response.getBody())
                .isNotNull();

        assertThat(response.getBody().code())
                .isEqualTo(ErrorCode.USER_NOT_FOUND);

        assertThat(response.getBody().message())
                .isEqualTo("USER_NOT_FOUND");

        assertThat(response.getBody().message())
                .doesNotContain(
                        "sensitive internal detail"
                );

        assertThat(response.getBody().path())
                .isEqualTo("/api/test");
    }

    private static final class TestMiletosException
            extends MiletosException {

        private TestMiletosException() {
            super(
                    ErrorCode.USER_NOT_FOUND,
                    HttpStatus.NOT_FOUND
            );
        }

        @Override
        public String getMessage() {
            return "sensitive internal detail";
        }
    }
}
