package com.miletos.features.company.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class CompanyDisabledException
        extends MiletosException {

    public CompanyDisabledException() {
        super(
                ErrorCode.COMPANY_DISABLED,
                HttpStatus.BAD_REQUEST);
    }
}
