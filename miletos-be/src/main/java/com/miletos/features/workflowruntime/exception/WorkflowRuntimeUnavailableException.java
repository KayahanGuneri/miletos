package com.miletos.features.workflowruntime.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public class WorkflowRuntimeUnavailableException extends MiletosException {

    public WorkflowRuntimeUnavailableException(Throwable cause) {
        super(
                ErrorCode.RUNTIME_UPSTREAM_UNAVAILABLE,
                HttpStatus.BAD_GATEWAY,
                cause);
    }
}
