package com.miletos.security;

import org.springframework.stereotype.Component;

import com.miletos.common.util.EmailNormalizer;
import com.miletos.features.user.exception.AuthenticatedUserNotFoundException;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.User;

import lombok.RequiredArgsConstructor;

@Component
@RequiredArgsConstructor
public class AuthenticatedActorResolver {

    private final UserRepository userRepository;

    public User resolve(String actorEmail) {
        return userRepository
                .findByEmail(EmailNormalizer.normalize(actorEmail))
                .orElseThrow(AuthenticatedUserNotFoundException::new);
    }
}
