package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InviteTokenAlreadyUsedException
        extends MiletosException {

    public InviteTokenAlreadyUsedException() {
        super(
                ErrorCode.INVITE_TOKEN_ALREADY_USED,
                HttpStatus.CONFLICT);
    }
}
