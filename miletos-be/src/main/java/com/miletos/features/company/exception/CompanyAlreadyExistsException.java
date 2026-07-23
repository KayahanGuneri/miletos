package com.miletos.features.company.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class CompanyAlreadyExistsException
        extends MiletosException {

    public CompanyAlreadyExistsException() {
        super(
                ErrorCode.COMPANY_ALREADY_EXISTS,
                HttpStatus.CONFLICT);
    }
}
