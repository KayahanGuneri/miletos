package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class CompanyUserListNotAllowedException
        extends MiletosException {

    public CompanyUserListNotAllowedException() {
        super(
                ErrorCode.FORBIDDEN,
                HttpStatus.FORBIDDEN
        );
    }
}
