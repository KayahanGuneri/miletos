package com.miletos.features.user.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class ProfilePhotoInvalidContentException
        extends MiletosException {

    public ProfilePhotoInvalidContentException() {
        super(
                ErrorCode.PROFILE_PHOTO_INVALID_CONTENT,
                HttpStatus.BAD_REQUEST);
    }

    public ProfilePhotoInvalidContentException(Throwable cause) {
        super(
                ErrorCode.PROFILE_PHOTO_INVALID_CONTENT,
                HttpStatus.BAD_REQUEST,
                cause);
    }
}
