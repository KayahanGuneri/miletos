package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InviteTokenNotFoundException
        extends MiletosException {

    public InviteTokenNotFoundException() {
        super(
                ErrorCode.INVITE_TOKEN_NOT_FOUND,
                HttpStatus.NOT_FOUND);
    }
}
