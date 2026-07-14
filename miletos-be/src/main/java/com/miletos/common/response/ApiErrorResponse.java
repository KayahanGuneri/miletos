package com.miletos.common.response;

import java.time.Instant;
import java.util.Map;

import com.miletos.common.exception.ErrorCode;

public record ApiErrorResponse(
                ErrorCode code,
                String message,
                String path,
                Instant timestamp,
                Map<String, String> fieldErrors) {
}