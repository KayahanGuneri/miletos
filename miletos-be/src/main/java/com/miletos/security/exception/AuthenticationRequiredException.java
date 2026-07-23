package com.miletos.security.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class AuthenticationRequiredException
        extends MiletosException {

    public AuthenticationRequiredException() {
        super(
                ErrorCode.UNAUTHENTICATED,
                HttpStatus.UNAUTHORIZED
        );
    }
}
