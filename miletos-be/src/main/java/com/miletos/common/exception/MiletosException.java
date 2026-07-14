package com.miletos.common.exception;

import java.util.Objects;

import org.springframework.http.HttpStatus;

import lombok.Getter;

@Getter
public abstract class MiletosException extends RuntimeException {

        private final ErrorCode code;
        private final HttpStatus httpStatus;

        protected MiletosException(
                        ErrorCode code,
                        HttpStatus httpStatus) {
                super(Objects.requireNonNull(code, "code").name());
                this.code = code;
                this.httpStatus = Objects.requireNonNull(
                                httpStatus,
                                "httpStatus");
        }

        protected MiletosException(
                        ErrorCode code,
                        HttpStatus httpStatus,
                        Throwable cause) {
                super(
                                Objects.requireNonNull(code, "code").name(),
                                cause);
                this.code = code;
                this.httpStatus = Objects.requireNonNull(
                                httpStatus,
                                "httpStatus");
        }
}
