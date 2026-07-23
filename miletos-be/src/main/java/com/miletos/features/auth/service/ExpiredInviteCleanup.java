package com.miletos.features.auth.service;

import java.time.Clock;
import java.time.Instant;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.auth.repository.entity.AuthTokenType;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserStatus;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class ExpiredInviteCleanup {

    private final AuthTokenRepository authTokenRepository;
    private final UserRepository userRepository;
    private final Clock clock;

    @Scheduled(initialDelayString = "${miletos.security.invite-cleanup.initial-delay-ms:60000}", fixedDelayString = "${miletos.security.invite-cleanup.fixed-delay-ms:60000}")
    @Transactional
    public void deleteExpiredPendingInviteUsers() {
        Instant now = Instant.now(clock);
        List<AuthToken> expiredInviteTokens = authTokenRepository
                .findAllByTypeAndUsedAtIsNullAndExpiresAtBefore(
                        AuthTokenType.INVITE,
                        now);
        Set<Long> processedUserIds = new HashSet<>();

        for (AuthToken inviteToken : expiredInviteTokens) {
            Long userId = inviteToken.getUser().getId();

            if (processedUserIds.add(userId)) {
                hardDeletePendingUserForExpiredInvite(inviteToken);
            }
        }
    }

    public void hardDeletePendingUserForExpiredInvite(AuthToken inviteToken) {
        User user = inviteToken.getUser();

        List<AuthToken> userInviteTokens = authTokenRepository.findAllByTypeAndUser_Id(
                AuthTokenType.INVITE,
                user.getId());
        authTokenRepository.deleteAll(userInviteTokens);
        authTokenRepository.flush();

        if (isPendingInviteUser(user)) {
            userRepository.delete(user);
            userRepository.flush();
        }
    }

    private boolean isPendingInviteUser(User user) {
        return user.getStatus() == UserStatus.PENDING
                && user.getOnboardingStatus() == OnboardingStatus.PASSWORD_SETUP_REQUIRED;
    }
}
