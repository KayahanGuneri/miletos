package com.miletos.features.auth.model;

import java.time.Instant;

public record JwtToken(
                String value,
                Instant expiresAt) {
}