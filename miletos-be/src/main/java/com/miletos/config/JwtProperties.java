package com.miletos.config;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "miletos.security.jwt")
public record JwtProperties(
                String secret,
                String issuer,
                long accessTokenMinutes) {
}