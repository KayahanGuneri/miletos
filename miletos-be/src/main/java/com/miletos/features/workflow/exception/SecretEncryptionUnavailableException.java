package com.miletos.features.workflow.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class SecretEncryptionUnavailableException extends MiletosException {

  public SecretEncryptionUnavailableException() {
    super(ErrorCode.SECRET_ENCRYPTION_UNAVAILABLE, HttpStatus.SERVICE_UNAVAILABLE);
  }
}
