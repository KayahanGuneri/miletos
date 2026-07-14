package com.miletos.features.auth.repository;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

import org.springframework.data.jpa.repository.JpaRepository;

import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.auth.repository.entity.AuthTokenType;

public interface AuthTokenRepository extends JpaRepository<AuthToken, Long> {

        Optional<AuthToken> findByTokenHashAndType(
                        String tokenHash,
                        AuthTokenType type);

        List<AuthToken> findAllByTypeAndCompany_Id(
                        AuthTokenType type,
                        Long companyId);

        List<AuthToken> findAllByTypeAndUser_Id(
                        AuthTokenType type,
                        Long userId);

        List<AuthToken> findAllByTypeAndCreatedByUser_Id(
                        AuthTokenType type,
                        Long createdByUserId);

        List<AuthToken> findAllByTypeAndUsedAtIsNullAndExpiresAtBefore(
                        AuthTokenType type,
                        Instant now);

        List<AuthToken> findAllByUser_Id(Long userId);
}