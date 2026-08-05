package com.miletos.common.exception;

import java.util.Map;
import java.util.Objects;

import org.springframework.http.HttpStatus;

import lombok.Getter;

@Getter
public abstract class MiletosException extends RuntimeException {

        private final ErrorCode code;
        private final HttpStatus httpStatus;
        private final Map<String, String> fieldErrors;

        protected MiletosException(
                        ErrorCode code,
                        HttpStatus httpStatus) {
                this(code, httpStatus, Map.of(), null);
        }

        protected MiletosException(
                        ErrorCode code,
                        HttpStatus httpStatus,
                        Throwable cause) {
                this(code, httpStatus, Map.of(), cause);
        }

        protected MiletosException(
                        ErrorCode code,
                        HttpStatus httpStatus,
                        Map<String, String> fieldErrors,
                        Throwable cause) {
                super(
                                Objects.requireNonNull(code, "code").name(),
                                cause);
                this.code = code;
                this.httpStatus = Objects.requireNonNull(
                                httpStatus,
                                "httpStatus");
                this.fieldErrors = fieldErrors == null
                                ? Map.of()
                                : Map.copyOf(fieldErrors);
        }
}
