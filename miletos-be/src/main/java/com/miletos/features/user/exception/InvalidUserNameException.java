package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class InvalidUserNameException
        extends MiletosException {

    public InvalidUserNameException() {
        super(
                ErrorCode.INVALID_USER_NAME,
                HttpStatus.BAD_REQUEST
        );
    }
}
