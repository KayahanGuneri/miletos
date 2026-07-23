package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class EmailAlreadyExistsException
        extends MiletosException {

    public EmailAlreadyExistsException() {
        super(
                ErrorCode.EMAIL_ALREADY_EXISTS,
                HttpStatus.CONFLICT
        );
    }
}
