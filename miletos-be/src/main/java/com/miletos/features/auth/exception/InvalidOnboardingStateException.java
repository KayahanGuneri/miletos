package com.miletos.features.auth.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class InvalidOnboardingStateException
        extends MiletosException {

    public InvalidOnboardingStateException() {
        super(
                ErrorCode.INVALID_ONBOARDING_STATE,
                HttpStatus.BAD_REQUEST);
    }
}
