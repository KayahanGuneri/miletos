package com.miletos.config;

import java.nio.charset.StandardCharsets;

import javax.crypto.SecretKey;
import javax.crypto.spec.SecretKeySpec;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.oauth2.core.DelegatingOAuth2TokenValidator;
import org.springframework.security.oauth2.core.OAuth2TokenValidator;
import org.springframework.security.oauth2.jose.jws.MacAlgorithm;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.JwtEncoder;
import org.springframework.security.oauth2.jwt.JwtValidators;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.security.oauth2.jwt.NimbusJwtEncoder;

import com.miletos.security.session.ActiveSessionJwtValidator;
import com.nimbusds.jose.jwk.source.ImmutableSecret;

import lombok.RequiredArgsConstructor;

@Configuration
@RequiredArgsConstructor
public class JwtBeansConfig {

    private static final int MINIMUM_HMAC_SECRET_BYTES = 32;

    private final JwtProperties jwtProperties;
    private final ActiveSessionJwtValidator activeSessionJwtValidator;

    @Bean
    public SecretKey jwtSecretKey() {
        byte[] secretBytes = jwtProperties.secret().getBytes(StandardCharsets.UTF_8);

        if (secretBytes.length < MINIMUM_HMAC_SECRET_BYTES) {
            throw new IllegalStateException("JWT secret must be at least 32 bytes");
        }

        return new SecretKeySpec(secretBytes, "HmacSHA256");
    }

    @Bean
    public JwtEncoder jwtEncoder(SecretKey jwtSecretKey) {
        return new NimbusJwtEncoder(new ImmutableSecret<>(jwtSecretKey));
    }

    @Bean
    public JwtDecoder jwtDecoder(SecretKey jwtSecretKey) {
        NimbusJwtDecoder jwtDecoder = NimbusJwtDecoder
                .withSecretKey(jwtSecretKey)
                .macAlgorithm(MacAlgorithm.HS256)
                .build();

        OAuth2TokenValidator<Jwt> jwtValidator = new DelegatingOAuth2TokenValidator<>(
                JwtValidators.createDefault(),
                activeSessionJwtValidator);

        jwtDecoder.setJwtValidator(jwtValidator);

        return jwtDecoder;
    }
}