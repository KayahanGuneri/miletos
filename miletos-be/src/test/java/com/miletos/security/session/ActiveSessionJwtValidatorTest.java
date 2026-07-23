package com.miletos.security.session;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.security.authorization.AuthorizationEvaluator;
import java.time.Instant;
import java.util.Map;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.security.oauth2.jwt.Jwt;

class ActiveSessionJwtValidatorTest {

    private ActiveSessionService activeSessionService;
    private AuthenticatedActorResolver actorResolver;
    private ActiveSessionJwtValidator validator;

    @BeforeEach
    void setUp() {
        activeSessionService = mock(ActiveSessionService.class);
        actorResolver = mock(AuthenticatedActorResolver.class);
        validator = new ActiveSessionJwtValidator(
                activeSessionService,
                actorResolver,
                new AuthorizationEvaluator()
        );
    }

    @Test
    void acceptsJtiForCurrentActiveUserSession() {
        User user = activeUser();
        when(actorResolver.resolve(user.getEmail())).thenReturn(user);
        when(activeSessionService.isSessionActive(12L, "session-1")).thenReturn(true);

        assertThat(validator.validate(jwt(user.getEmail(), "session-1")).hasErrors()).isFalse();
    }

    @Test
    void rejectsInactiveBackendUserEvenWhenSessionExists() {
        User user = activeUser();
        user.setStatus(UserStatus.DISABLED);
        when(actorResolver.resolve(user.getEmail())).thenReturn(user);
        when(activeSessionService.isSessionActive(12L, "session-1")).thenReturn(true);

        assertThat(validator.validate(jwt(user.getEmail(), "session-1")).hasErrors()).isTrue();
    }

    @Test
    void rejectsUnknownJti() {
        User user = activeUser();
        when(actorResolver.resolve(user.getEmail())).thenReturn(user);

        assertThat(validator.validate(jwt(user.getEmail(), "unknown")).hasErrors()).isTrue();
    }

    private User activeUser() {
        User user = new User(
                null,
                null,
                "actor@miletos.local",
                "hash",
                "Active",
                "Actor",
                UserRole.USER,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        );
        user.setId(12L);
        return user;
    }

    private Jwt jwt(String subject, String jti) {
        Instant now = Instant.parse("2026-07-18T08:00:00Z");
        return new Jwt(
                "token",
                now,
                now.plusSeconds(300),
                Map.of("alg", "HS256"),
                Map.of("sub", subject, "jti", jti)
        );
    }
}
