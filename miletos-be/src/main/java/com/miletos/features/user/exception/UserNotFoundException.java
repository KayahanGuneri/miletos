package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class UserNotFoundException
        extends MiletosException {

    public UserNotFoundException() {
        super(
                ErrorCode.USER_NOT_FOUND,
                HttpStatus.NOT_FOUND
        );
    }
}
