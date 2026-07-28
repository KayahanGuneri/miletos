package com.miletos.features.workflowruntime.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public class WorkflowRuntimeTimeoutException extends MiletosException {

    public WorkflowRuntimeTimeoutException(Throwable cause) {
        super(
                ErrorCode.RUNTIME_UPSTREAM_TIMEOUT,
                HttpStatus.GATEWAY_TIMEOUT,
                cause);
    }
}
