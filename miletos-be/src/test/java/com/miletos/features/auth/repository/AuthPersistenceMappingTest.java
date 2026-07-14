package com.miletos.features.auth.repository;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.jdbc.AutoConfigureTestDatabase;
import org.springframework.boot.test.autoconfigure.orm.jpa.DataJpaTest;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.JdbcTemplate;

@DataJpaTest
@AutoConfigureTestDatabase(replace = AutoConfigureTestDatabase.Replace.NONE)
class AuthPersistenceMappingTest extends PostgreSqlContainerSupport {

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private JdbcTemplate jdbcTemplate;

    @Test
    void persistsCompanyAndUsersWithDatabaseGeneratedIdentifiers() {
        Company company = companyRepository.saveAndFlush(
                new Company(
                        "Core Persistence Tenant",
                        CompanyStatus.ACTIVE
                )
        );

        User superadmin = new User(
                null,
                null,
                "core-superadmin@miletos.local",
                "$2a$12$core-superadmin-password-hash",
                "Core",
                "Superadmin",
                null,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.NOT_REQUIRED,
                null,
                null,
                null,
                null
        );
        superadmin.setSuperAdmin(true);
        superadmin = userRepository.saveAndFlush(superadmin);

        User companyUser = userRepository.saveAndFlush(
                new User(
                        null,
                        company,
                        "core-user@miletos.local",
                        "$2a$12$core-user-password-hash",
                        "Core",
                        "User",
                        UserRole.USER,
                        false,
                        UserStatus.ACTIVE,
                        OnboardingStatus.COMPLETED,
                        null,
                        null,
                        null,
                        null
                )
        );

        assertThat(company.getId()).isNotNull();
        assertThat(superadmin.getId()).isNotNull();
        assertThat(companyUser.getId()).isNotNull();

        assertThat(superadmin.isSuperAdmin()).isTrue();
        assertThat(superadmin.getCompany()).isNull();
        assertThat(superadmin.getRole()).isNull();

        assertThat(companyUser.isSuperAdmin()).isFalse();
        assertThat(companyUser.getCompany()).isNotNull();
        assertThat(companyUser.getCompany().getId())
                .isEqualTo(company.getId());
        assertThat(companyUser.getRole())
                .isEqualTo(UserRole.USER);
    }

    @Test
    void rejectsCompanyScopedUserWithoutCompany() {
        User invalidUser = new User(
                null,
                null,
                "invalid-company-scope@miletos.local",
                null,
                "Invalid",
                "User",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        );

        assertThatThrownBy(
                () -> userRepository.saveAndFlush(invalidUser)
        ).isInstanceOf(DataIntegrityViolationException.class);
    }

    @Test
    void exposesExpectedCorePersistenceSchema() {
        Integer expectedTableCount = jdbcTemplate.queryForObject(
                """
                SELECT COUNT(*)
                FROM information_schema.tables
                WHERE table_schema = 'public'
                  AND table_name IN (
                      'companies',
                      'user',
                      'stored_files',
                      'auth_tokens'
                  )
                """,
                Integer.class
        );

        Integer bigintIdentityCount = jdbcTemplate.queryForObject(
                """
                SELECT COUNT(*)
                FROM information_schema.columns
                WHERE table_schema = 'public'
                  AND (
                      (table_name = 'companies' AND column_name = 'id')
                      OR
                      (table_name = 'user' AND column_name = 'id')
                      OR
                      (table_name = 'stored_files' AND column_name = 'id')
                      OR
                      (table_name = 'auth_tokens' AND column_name = 'id')
                  )
                  AND data_type = 'bigint'
                  AND is_identity = 'YES'
                """,
                Integer.class
        );

        Integer bigintForeignKeyColumnCount =
                jdbcTemplate.queryForObject(
                        """
                        SELECT COUNT(*)
                        FROM information_schema.columns
                        WHERE table_schema = 'public'
                          AND (
                              (
                                  table_name = 'user'
                                  AND column_name IN (
                                      'company_id',
                                      'profile_photo_file_id'
                                  )
                              )
                              OR
                              (
                                  table_name = 'stored_files'
                                  AND column_name = 'owner_user_id'
                              )
                              OR
                              (
                                  table_name = 'auth_tokens'
                                  AND column_name IN (
                                      'company_id',
                                      'user_id',
                                      'created_by_user_id'
                                  )
                              )
                          )
                          AND data_type = 'bigint'
                        """,
                        Integer.class
                );

        assertThat(expectedTableCount).isEqualTo(4);
        assertThat(bigintIdentityCount).isEqualTo(4);
        assertThat(bigintForeignKeyColumnCount).isEqualTo(6);
    }
}
