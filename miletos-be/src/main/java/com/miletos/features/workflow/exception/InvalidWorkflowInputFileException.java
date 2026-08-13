package com.miletos.features.workflow.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class InvalidWorkflowInputFileException extends MiletosException {

  public InvalidWorkflowInputFileException() {
    super(ErrorCode.WORKFLOW_INPUT_FILE_INVALID, HttpStatus.BAD_REQUEST);
  }
}
