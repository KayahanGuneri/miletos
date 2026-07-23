package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InvalidCredentialsException
        extends MiletosException {

    public InvalidCredentialsException() {
        super(
                ErrorCode.INVALID_CREDENTIALS,
                HttpStatus.UNAUTHORIZED);
    }
}
