package com.miletos.security.authorization;

import com.miletos.features.user.repository.entity.User;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.security.exception.AuthenticationRequiredException;
import com.miletos.security.exception.OperationNotAuthorizedException;
import lombok.RequiredArgsConstructor;
import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.annotation.Around;
import org.aspectj.lang.annotation.Aspect;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;

@Aspect
@Component
@RequiredArgsConstructor
public class AuthorizationAspect {

    private final AuthenticatedActorResolver authenticatedActorResolver;
    private final AuthorizationEvaluator authorizationEvaluator;

    @Around("@annotation(authorize)")
    public Object authorize(
            ProceedingJoinPoint joinPoint,
            Authorize authorize
    ) throws Throwable {
        Authentication authentication =
                SecurityContextHolder
                        .getContext()
                        .getAuthentication();

        if (
            authentication == null
                    || authentication.getName() == null
                    || authentication.getName().isBlank()
        ) {
            throw new AuthenticationRequiredException();
        }

        User user =
                authenticatedActorResolver.resolve(
                        authentication.getName()
                );

        if (
            !authorizationEvaluator.hasAnyRole(
                    user,
                    authorize.value()
            )
        ) {
            throw new OperationNotAuthorizedException();
        }

        return joinPoint.proceed();
    }

}
