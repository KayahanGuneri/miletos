package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class ProfilePhotoNotFoundException
        extends MiletosException {

    public ProfilePhotoNotFoundException() {
        super(
                ErrorCode.PROFILE_PHOTO_NOT_FOUND,
                HttpStatus.NOT_FOUND
        );
    }
}
