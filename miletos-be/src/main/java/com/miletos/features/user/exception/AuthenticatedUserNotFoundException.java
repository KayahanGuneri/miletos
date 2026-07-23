package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class AuthenticatedUserNotFoundException
        extends MiletosException {

    public AuthenticatedUserNotFoundException() {
        super(
                ErrorCode.AUTHENTICATED_USER_NOT_FOUND,
                HttpStatus.UNAUTHORIZED
        );
    }
}
