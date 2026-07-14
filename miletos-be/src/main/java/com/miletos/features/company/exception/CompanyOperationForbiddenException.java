package com.miletos.features.company.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class CompanyOperationForbiddenException
        extends MiletosException {

    public CompanyOperationForbiddenException() {
        super(
                ErrorCode.FORBIDDEN,
                HttpStatus.FORBIDDEN);
    }
}
