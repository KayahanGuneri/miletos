package com.miletos.features.company.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InvalidCompanyStatusException
        extends MiletosException {

    public InvalidCompanyStatusException() {
        super(
                ErrorCode.INVALID_COMPANY_STATUS,
                HttpStatus.BAD_REQUEST);
    }
}
