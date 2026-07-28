package com.miletos.features.workflowruntime.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public class WorkflowRuntimeNotConfiguredException extends MiletosException {

    public WorkflowRuntimeNotConfiguredException() {
        super(
                ErrorCode.RUNTIME_BRIDGE_NOT_CONFIGURED,
                HttpStatus.SERVICE_UNAVAILABLE);
    }
}
