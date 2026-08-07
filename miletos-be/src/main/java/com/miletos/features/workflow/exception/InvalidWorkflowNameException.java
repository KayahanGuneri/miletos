package com.miletos.features.workflow.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InvalidWorkflowNameException extends MiletosException {

    public InvalidWorkflowNameException() {
        super(ErrorCode.INVALID_WORKFLOW_NAME, HttpStatus.BAD_REQUEST);
    }
}
