package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InviteRoleNotAllowedException
        extends MiletosException {

    public InviteRoleNotAllowedException() {
        super(
                ErrorCode.INVITE_ROLE_NOT_ALLOWED,
                HttpStatus.FORBIDDEN);
    }
}
