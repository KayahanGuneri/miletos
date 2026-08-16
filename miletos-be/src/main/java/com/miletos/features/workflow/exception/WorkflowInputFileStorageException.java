package com.miletos.features.workflow.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class WorkflowInputFileStorageException extends MiletosException {

  public WorkflowInputFileStorageException() {
    super(ErrorCode.WORKFLOW_INPUT_FILE_STORAGE_FAILED, HttpStatus.INTERNAL_SERVER_ERROR);
  }

  public WorkflowInputFileStorageException(Throwable cause) {
    super(ErrorCode.WORKFLOW_INPUT_FILE_STORAGE_FAILED, HttpStatus.INTERNAL_SERVER_ERROR, cause);
  }
}
