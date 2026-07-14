package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InviteTokenExpiredException
        extends MiletosException {

    public InviteTokenExpiredException() {
        super(
                ErrorCode.INVITE_TOKEN_EXPIRED,
                HttpStatus.GONE);
    }
}
