package com.miletos.features.workflow.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class WorkflowNotFoundException extends MiletosException {

    public WorkflowNotFoundException() {
        super(ErrorCode.WORKFLOW_NOT_FOUND, HttpStatus.NOT_FOUND);
    }
}
