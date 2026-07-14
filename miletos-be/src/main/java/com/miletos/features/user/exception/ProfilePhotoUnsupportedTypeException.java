package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class ProfilePhotoUnsupportedTypeException
        extends MiletosException {

    public ProfilePhotoUnsupportedTypeException() {
        super(
                ErrorCode.PROFILE_PHOTO_UNSUPPORTED_TYPE,
                HttpStatus.BAD_REQUEST
        );
    }
}
