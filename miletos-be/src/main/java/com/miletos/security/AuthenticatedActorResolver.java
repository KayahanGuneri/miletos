package com.miletos.security;

import org.springframework.stereotype.Component;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.context.SecurityContextHolder;

import com.miletos.common.util.EmailNormalizer;
import com.miletos.features.user.exception.AuthenticatedUserNotFoundException;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.User;
import com.miletos.security.exception.AuthenticationRequiredException;

import lombok.RequiredArgsConstructor;

@Component
@RequiredArgsConstructor
public class AuthenticatedActorResolver {

    private final UserRepository userRepository;

    public User resolveCurrent() {
        Authentication authentication = SecurityContextHolder.getContext().getAuthentication();
        if (authentication == null
                || authentication.getName() == null
                || authentication.getName().isBlank()) {
            throw new AuthenticationRequiredException();
        }

        return resolve(authentication.getName());
    }

    public User resolve(String actorEmail) {
        return userRepository
                .findByEmail(EmailNormalizer.normalize(actorEmail))
                .orElseThrow(AuthenticatedUserNotFoundException::new);
    }
}
