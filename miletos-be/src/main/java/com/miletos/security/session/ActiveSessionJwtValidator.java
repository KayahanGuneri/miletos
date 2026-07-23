package com.miletos.security.session;

import lombok.RequiredArgsConstructor;
import org.springframework.security.oauth2.core.OAuth2Error;
import org.springframework.security.oauth2.core.OAuth2TokenValidator;
import org.springframework.security.oauth2.core.OAuth2TokenValidatorResult;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.stereotype.Component;
import org.springframework.util.StringUtils;
import com.miletos.features.user.repository.entity.User;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.security.authorization.AuthorizationEvaluator;

@Component
@RequiredArgsConstructor
public class ActiveSessionJwtValidator implements OAuth2TokenValidator<Jwt> {

    private static final OAuth2Error INVALID_SESSION_ERROR = new OAuth2Error(
            "inactive_session",
            "JWT session is not active",
            null
    );

    private final ActiveSessionService activeSessionService;
    private final AuthenticatedActorResolver authenticatedActorResolver;
    private final AuthorizationEvaluator authorizationEvaluator;

    @Override
    public OAuth2TokenValidatorResult validate(Jwt token) {
        String subject = token.getSubject();
        String sessionId = token.getId();

        if (!StringUtils.hasText(subject) || !StringUtils.hasText(sessionId)) {
            return OAuth2TokenValidatorResult.failure(INVALID_SESSION_ERROR);
        }

        User user;

        try {
            user = authenticatedActorResolver.resolve(subject);
        } catch (RuntimeException exception) {
            return OAuth2TokenValidatorResult.failure(INVALID_SESSION_ERROR);
        }

        if (!authorizationEvaluator.isActive(user)
                || !activeSessionService.isSessionActive(user.getId(), sessionId)) {
            return OAuth2TokenValidatorResult.failure(INVALID_SESSION_ERROR);
        }

        return OAuth2TokenValidatorResult.success();
    }
}
