package com.miletos.features.workflow.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InvalidWorkflowDescriptionException extends MiletosException {

    public InvalidWorkflowDescriptionException() {
        super(ErrorCode.INVALID_WORKFLOW_DESCRIPTION, HttpStatus.BAD_REQUEST);
    }
}
