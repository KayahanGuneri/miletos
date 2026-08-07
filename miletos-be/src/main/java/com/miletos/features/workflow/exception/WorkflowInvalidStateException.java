package com.miletos.features.workflow.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class WorkflowInvalidStateException extends MiletosException {

    public WorkflowInvalidStateException() {
        super(ErrorCode.WORKFLOW_INVALID_STATE, HttpStatus.CONFLICT);
    }
}
