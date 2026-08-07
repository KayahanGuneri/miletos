package com.miletos.features.workflow.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class WorkflowNameAlreadyExistsException extends MiletosException {

    public WorkflowNameAlreadyExistsException() {
        super(ErrorCode.WORKFLOW_NAME_ALREADY_EXISTS, HttpStatus.CONFLICT);
    }
}
