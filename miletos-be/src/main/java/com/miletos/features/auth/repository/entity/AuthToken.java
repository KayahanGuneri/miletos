package com.miletos.features.auth.repository.entity;

import java.time.Instant;

import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.user.repository.entity.User;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.FetchType;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.ManyToOne;
import jakarta.persistence.PrePersist;
import jakarta.persistence.Table;
import lombok.AccessLevel;
import lombok.Getter;
import lombok.NoArgsConstructor;
import lombok.Setter;

@Entity
@Table(name = "auth_tokens")
@Getter
@Setter
@NoArgsConstructor(access = AccessLevel.PROTECTED)
public class AuthToken {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Enumerated(EnumType.STRING)
    @Column(name = "type", nullable = false, length = 40)
    private AuthTokenType type;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "company_id")
    private Company company;

    @ManyToOne(fetch = FetchType.LAZY, optional = false)
    @JoinColumn(name = "user_id", nullable = false)
    private User user;

    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "created_by_user_id")
    private User createdByUser;

    @Column(name = "token_hash", nullable = false, length = 128)
    private String tokenHash;

    @Column(name = "expires_at", nullable = false)
    private Instant expiresAt;

    @Column(name = "used_at")
    private Instant usedAt;

    @Column(name = "created_at", nullable = false, updatable = false)
    private Instant createdAt;

    private AuthToken(
            AuthTokenType type,
            Company company,
            User user,
            User createdByUser,
            String tokenHash,
            Instant expiresAt) {
        this.type = type;
        this.company = company;
        this.user = user;
        this.createdByUser = createdByUser;
        this.tokenHash = tokenHash;
        this.expiresAt = expiresAt;
    }

    public static AuthToken invite(
            Company company,
            User user,
            User createdByUser,
            String tokenHash,
            Instant expiresAt) {
        return new AuthToken(
                AuthTokenType.INVITE,
                company,
                user,
                createdByUser,
                tokenHash,
                expiresAt);
    }

    public static AuthToken passwordReset(
            User user,
            String tokenHash,
            Instant expiresAt) {
        return new AuthToken(
                AuthTokenType.PASSWORD_RESET,
                null,
                user,
                null,
                tokenHash,
                expiresAt);
    }

    @PrePersist
    void prePersist() {
        if (createdAt == null) {
            createdAt = Instant.now();
        }
    }
}