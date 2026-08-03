package com.miletos.features.workflowruntime.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public class CompanyContextRequiredException extends MiletosException {

    public CompanyContextRequiredException() {
        super(ErrorCode.COMPANY_CONTEXT_REQUIRED, HttpStatus.FORBIDDEN);
    }
}
