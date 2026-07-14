package com.miletos.security.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class OperationNotAuthorizedException
        extends MiletosException {

    public OperationNotAuthorizedException() {
        super(
                ErrorCode.FORBIDDEN,
                HttpStatus.FORBIDDEN
        );
    }
}
