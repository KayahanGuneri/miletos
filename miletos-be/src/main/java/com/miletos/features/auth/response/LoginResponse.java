package com.miletos.features.auth.response;

import java.time.Instant;

public record LoginResponse(
        String accessToken,
        TokenType tokenType,
        Instant expiresAt,
        AuthenticatedUserResponse user
) {
}
