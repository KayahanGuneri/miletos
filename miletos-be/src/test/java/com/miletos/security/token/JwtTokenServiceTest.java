package com.miletos.security.token;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import com.miletos.config.JwtProperties;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.security.oauth2.jwt.JwtEncoder;
import org.springframework.security.oauth2.jwt.JwtEncoderParameters;

class JwtTokenServiceTest {

    @Test
    void emitsOnlyStandardIdentityAndJtiClaims() {
        JwtEncoder encoder = mock(JwtEncoder.class);
        Jwt encodedJwt = mock(Jwt.class);
        when(encodedJwt.getTokenValue()).thenReturn("encoded-token");
        when(encoder.encode(any())).thenReturn(encodedJwt);
        Clock clock = Clock.fixed(Instant.parse("2026-07-18T08:00:00Z"), ZoneOffset.UTC);
        JwtTokenService service = new JwtTokenService(
                encoder,
                new JwtProperties("unused-in-unit-test", "https://miletos.test", 30),
                clock
        );
        User user = new User(
                null,
                null,
                " Actor@Miletos.Local ",
                "hash",
                "Active",
                "Actor",
                UserRole.ADMIN,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        );
        user.setId(42L);
        user.setSuperAdmin(true);

        service.generateAccessToken(user, "session-jti");

        ArgumentCaptor<JwtEncoderParameters> parameters =
                ArgumentCaptor.forClass(JwtEncoderParameters.class);
        org.mockito.Mockito.verify(encoder).encode(parameters.capture());
        assertThat(parameters.getValue().getClaims().getClaims())
                .containsOnlyKeys("iss", "sub", "iat", "exp", "jti")
                .containsEntry("sub", "actor@miletos.local")
                .containsEntry("jti", "session-jti");
    }
}
