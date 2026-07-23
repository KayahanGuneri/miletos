package com.miletos.features.company.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class CompanyNotFoundException
        extends MiletosException {

    public CompanyNotFoundException() {
        super(
                ErrorCode.COMPANY_NOT_FOUND,
                HttpStatus.NOT_FOUND);
    }
}
