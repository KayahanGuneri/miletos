package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class PasswordResetTokenAlreadyUsedException
        extends MiletosException {

    public PasswordResetTokenAlreadyUsedException() {
        super(
                ErrorCode.PASSWORD_RESET_TOKEN_ALREADY_USED,
                HttpStatus.CONFLICT);
    }
}
