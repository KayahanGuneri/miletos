package com.miletos.features.workflowruntime.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public class WorkflowRuntimeAuthenticationException extends MiletosException {

    public WorkflowRuntimeAuthenticationException() {
        super(
                ErrorCode.RUNTIME_AUTHENTICATION_FAILED,
                HttpStatus.BAD_GATEWAY);
    }
}
