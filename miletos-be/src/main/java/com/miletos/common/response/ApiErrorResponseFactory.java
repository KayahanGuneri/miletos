package com.miletos.common.response;

import java.time.Clock;
import java.time.Instant;
import java.util.Map;

import org.springframework.stereotype.Component;

import com.miletos.common.exception.ErrorCode;

@Component
public class ApiErrorResponseFactory {

    private final Clock clock;

    public ApiErrorResponseFactory(Clock clock) {
        this.clock = clock;
    }

    public ApiErrorResponse create(ErrorCode code, String message, String path) {
        return create(code, message, path, Map.of());
    }

    public ApiErrorResponse create(
            ErrorCode code,
            String message,
            String path,
            Map<String, String> fieldErrors) {
        return new ApiErrorResponse(
                code,
                message,
                path,
                Instant.now(clock),
                fieldErrors == null ? Map.of() : fieldErrors);
    }
}