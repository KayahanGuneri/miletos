package com.miletos.features.user.service.output;

public record ProfilePhotoContent(
        byte[] content,
        String contentType
) {
}