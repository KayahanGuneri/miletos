package com.miletos.features.workflowruntime.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import java.util.Map;
import org.springframework.http.HttpStatus;

public final class WorkflowRuntimeDomainException extends MiletosException {

  public WorkflowRuntimeDomainException(ErrorCode code, HttpStatus status) {
    super(code, status);
  }

  public WorkflowRuntimeDomainException(ErrorCode code, HttpStatus status, Throwable cause) {
    super(code, status, cause);
  }

  public WorkflowRuntimeDomainException(
      ErrorCode code, HttpStatus status, Map<String, String> validationIssues, Throwable cause) {
    super(code, status, validationIssues, cause);
  }
}
