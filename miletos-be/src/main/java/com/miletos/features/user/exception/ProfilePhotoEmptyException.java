package com.miletos.features.user.exception;

import org.springframework.http.HttpStatus;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;

public final class ProfilePhotoEmptyException
        extends MiletosException {

    public ProfilePhotoEmptyException() {
        super(
                ErrorCode.PROFILE_PHOTO_EMPTY,
                HttpStatus.BAD_REQUEST);
    }
}
