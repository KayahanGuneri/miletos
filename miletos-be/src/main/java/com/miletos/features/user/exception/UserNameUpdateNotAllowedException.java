package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class UserNameUpdateNotAllowedException
        extends MiletosException {

    public UserNameUpdateNotAllowedException() {
        super(
                ErrorCode.USER_NAME_UPDATE_NOT_ALLOWED,
                HttpStatus.FORBIDDEN
        );
    }
}
