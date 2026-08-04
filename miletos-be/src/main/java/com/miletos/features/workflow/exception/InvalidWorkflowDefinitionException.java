package com.miletos.features.workflow.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InvalidWorkflowDefinitionException extends MiletosException {

    public InvalidWorkflowDefinitionException() {
        super(ErrorCode.INVALID_WORKFLOW_DEFINITION, HttpStatus.BAD_REQUEST);
    }
}
