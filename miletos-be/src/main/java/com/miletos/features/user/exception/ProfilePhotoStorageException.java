package com.miletos.features.user.exception;

import com.miletos.common.exception.ErrorCode;
import com.miletos.common.exception.MiletosException;
import org.springframework.http.HttpStatus;

public final class ProfilePhotoStorageException
        extends MiletosException {

    public ProfilePhotoStorageException() {
        super(
                ErrorCode.PROFILE_PHOTO_STORAGE_FAILED,
                HttpStatus.INTERNAL_SERVER_ERROR
        );
    }
    public ProfilePhotoStorageException(Throwable cause) {
        super(
                ErrorCode.PROFILE_PHOTO_STORAGE_FAILED,
                HttpStatus.INTERNAL_SERVER_ERROR,
                cause
        );
    }
}
