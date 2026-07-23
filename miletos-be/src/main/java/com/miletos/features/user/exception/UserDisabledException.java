package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class UserDisabledException
        extends MiletosException {

    public UserDisabledException() {
        super(
                ErrorCode.USER_DISABLED,
                HttpStatus.FORBIDDEN
        );
    }
}
