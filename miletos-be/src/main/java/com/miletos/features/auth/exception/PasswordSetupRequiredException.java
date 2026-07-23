package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class PasswordSetupRequiredException
        extends MiletosException {

    public PasswordSetupRequiredException() {
        super(
                ErrorCode.PASSWORD_SETUP_REQUIRED,
                HttpStatus.FORBIDDEN);
    }
}
