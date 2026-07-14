package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class ProfilePhotoTooLargeException
        extends MiletosException {

    public ProfilePhotoTooLargeException() {
        super(
                ErrorCode.PROFILE_PHOTO_TOO_LARGE,
                HttpStatus.PAYLOAD_TOO_LARGE
        );
    }
}
