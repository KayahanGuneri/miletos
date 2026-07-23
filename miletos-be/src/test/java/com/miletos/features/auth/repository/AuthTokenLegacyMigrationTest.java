package com.miletos.features.auth.repository;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import java.util.Map;
import org.flywaydb.core.Flyway;
import org.flywaydb.core.api.MigrationVersion;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.datasource.DriverManagerDataSource;
import org.testcontainers.containers.PostgreSQLContainer;

class AuthTokenLegacyMigrationTest {

    private static final PostgreSQLContainer<?> POSTGRESQL =
            new PostgreSQLContainer<>("postgres:16-alpine")
                    .withDatabaseName("miletos_token_migration_test")
                    .withUsername("miletos")
                    .withPassword("miletos");

    @BeforeAll
    static void startContainer() {
        POSTGRESQL.start();
    }

    @AfterAll
    static void stopContainer() {
        POSTGRESQL.stop();
    }

    @Test
    void migratesLegacyTokenRowsAndDropsLegacyTables() {
        Flyway.configure()
                .dataSource(
                        POSTGRESQL.getJdbcUrl(),
                        POSTGRESQL.getUsername(),
                        POSTGRESQL.getPassword()
                )
                .target(MigrationVersion.fromVersion("6"))
                .load()
                .migrate();

        JdbcTemplate jdbcTemplate = new JdbcTemplate(
                new DriverManagerDataSource(
                        POSTGRESQL.getJdbcUrl(),
                        POSTGRESQL.getUsername(),
                        POSTGRESQL.getPassword()
                )
        );

        Long companyId = jdbcTemplate.queryForObject(
                """
                INSERT INTO companies (
                    name,
                    status
                )
                VALUES (
                    'Legacy Token Migration Tenant',
                    'ACTIVE'
                )
                RETURNING id
                """,
                Long.class
        );

        Long creatorUserId = jdbcTemplate.queryForObject(
                """
                INSERT INTO "user" (
                    company_id,
                    email,
                    password_hash,
                    first_name,
                    last_name,
                    role,
                    status,
                    onboarding_status,
                    is_super_admin
                )
                VALUES (
                    NULL,
                    'legacy-token-superadmin@miletos.local',
                    '$2a$12$legacy-superadmin-password-hash',
                    'Legacy',
                    'Superadmin',
                    NULL,
                    'ACTIVE',
                    'NOT_REQUIRED',
                    TRUE
                )
                RETURNING id
                """,
                Long.class
        );

        Long invitedUserId = jdbcTemplate.queryForObject(
                """
                INSERT INTO "user" (
                    company_id,
                    email,
                    password_hash,
                    first_name,
                    last_name,
                    role,
                    status,
                    onboarding_status,
                    is_super_admin
                )
                VALUES (
                    ?,
                    'legacy-token-invited@miletos.local',
                    NULL,
                    'Legacy',
                    'Invited',
                    'USER',
                    'PENDING',
                    'PASSWORD_SETUP_REQUIRED',
                    FALSE
                )
                RETURNING id
                """,
                Long.class,
                companyId
        );

        Long activeUserId = jdbcTemplate.queryForObject(
                """
                INSERT INTO "user" (
                    company_id,
                    email,
                    password_hash,
                    first_name,
                    last_name,
                    role,
                    status,
                    onboarding_status,
                    is_super_admin
                )
                VALUES (
                    ?,
                    'legacy-token-active@miletos.local',
                    '$2a$12$legacy-active-password-hash',
                    'Legacy',
                    'Active',
                    'USER',
                    'ACTIVE',
                    'COMPLETED',
                    FALSE
                )
                RETURNING id
                """,
                Long.class,
                companyId
        );

        jdbcTemplate.update(
                """
                INSERT INTO invite_tokens (
                    company_id,
                    user_id,
                    created_by_user_id,
                    token_hash,
                    expires_at
                )
                VALUES (
                    ?,
                    ?,
                    ?,
                    'shared-legacy-token-hash',
                    CURRENT_TIMESTAMP + INTERVAL '24 hours'
                )
                """,
                companyId,
                invitedUserId,
                creatorUserId
        );

        jdbcTemplate.update(
                """
                INSERT INTO password_reset_tokens (
                    user_id,
                    token_hash,
                    expires_at
                )
                VALUES (
                    ?,
                    'shared-legacy-token-hash',
                    CURRENT_TIMESTAMP + INTERVAL '30 minutes'
                )
                """,
                activeUserId
        );

        Flyway.configure()
                .dataSource(
                        POSTGRESQL.getJdbcUrl(),
                        POSTGRESQL.getUsername(),
                        POSTGRESQL.getPassword()
                )
                .load()
                .migrate();

        List<Map<String, Object>> migratedTokens =
                jdbcTemplate.queryForList(
                        """
                        SELECT
                            type,
                            company_id,
                            user_id,
                            created_by_user_id,
                            token_hash
                        FROM auth_tokens
                        ORDER BY type
                        """
                );

        Integer legacyTableCount = jdbcTemplate.queryForObject(
                """
                SELECT COUNT(*)
                FROM information_schema.tables
                WHERE table_schema = 'public'
                  AND table_name IN (
                      'invite_tokens',
                      'password_reset_tokens'
                  )
                """,
                Integer.class
        );

        assertThat(migratedTokens).hasSize(2);

        assertThat(migratedTokens)
                .extracting(row -> row.get("type"))
                .containsExactly(
                        "INVITE",
                        "PASSWORD_RESET"
                );

        assertThat(migratedTokens)
                .extracting(row -> row.get("token_hash"))
                .containsOnly("shared-legacy-token-hash");

        Map<String, Object> inviteToken = migratedTokens.getFirst();
        Map<String, Object> passwordResetToken =
                migratedTokens.getLast();

        assertThat(inviteToken.get("company_id"))
                .isEqualTo(companyId);
        assertThat(inviteToken.get("user_id"))
                .isEqualTo(invitedUserId);
        assertThat(inviteToken.get("created_by_user_id"))
                .isEqualTo(creatorUserId);

        assertThat(passwordResetToken.get("company_id")).isNull();
        assertThat(passwordResetToken.get("user_id"))
                .isEqualTo(activeUserId);
        assertThat(passwordResetToken.get("created_by_user_id"))
                .isNull();

        assertThat(legacyTableCount).isZero();
    }
}
