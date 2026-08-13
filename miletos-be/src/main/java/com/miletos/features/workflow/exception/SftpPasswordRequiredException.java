package com.miletos.features.workflow.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class SftpPasswordRequiredException extends MiletosException {

  public SftpPasswordRequiredException() {
    super(ErrorCode.SFTP_PASSWORD_REQUIRED, HttpStatus.BAD_REQUEST);
  }
}
