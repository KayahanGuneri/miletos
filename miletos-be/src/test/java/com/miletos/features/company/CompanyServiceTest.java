package com.miletos.features.company;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.auth.service.TokenHasher;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.data.domain.Page;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest
@Transactional
class CompanyServiceTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL = "company-checkpoint-superadmin@miletos.local";

    @Autowired
    private CompanyService companyService;

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private AuthTokenRepository authTokenRepository;

    @Autowired
    private TokenHasher tokenHasher;

    @BeforeEach
    void createRequiredSuperadmin() {
        if (userRepository.findByEmail(SUPERADMIN_EMAIL).isPresent()) {
            return;
        }

        userRepository.saveAndFlush(new User(
                null,
                null,
                SUPERADMIN_EMAIL,
                "unused-test-password-hash",
                "Company",
                "Admin",
                null,
                true,
                UserStatus.ACTIVE,
                OnboardingStatus.NOT_REQUIRED,
                null,
                null,
                null,
                null));
    }

    @Test
    void createsCompanyWhenActorIsSuperadmin() {
        Company company = new Company("  Miletos Tenant  ", CompanyStatus.DISABLED);

        Company createdCompany = companyService.createCompany(SUPERADMIN_EMAIL, company);

        assertThat(createdCompany.getId()).isNotNull();
        assertThat(createdCompany.getName()).isEqualTo("Miletos Tenant");
        assertThat(createdCompany.getStatus()).isEqualTo(CompanyStatus.ACTIVE);
        assertThat(companyRepository.existsByNameIgnoreCase("miletos tenant")).isTrue();
    }

    @Test
    void listsCompaniesWithPagination() {
        companyRepository.saveAndFlush(new Company("Page Tenant A", CompanyStatus.ACTIVE));
        companyRepository.saveAndFlush(new Company("Page Tenant B", CompanyStatus.ACTIVE));
        companyRepository.saveAndFlush(new Company("Page Tenant C", CompanyStatus.DISABLED));

        Page<Company> page = companyService.listCompanies(0, 2);

        assertThat(page.getContent()).hasSize(2);
        assertThat(page.getNumber()).isZero();
        assertThat(page.getSize()).isEqualTo(2);
        assertThat(page.getTotalElements()).isEqualTo(3);
        assertThat(page.getTotalPages()).isEqualTo(2);
    }

    @Test
    void updatesCompanyWhenActorIsSuperadmin() {
        Company company = companyRepository.saveAndFlush(new Company("Editable Tenant", CompanyStatus.ACTIVE));

        Company updatedCompany = companyService.updateCompany(
                SUPERADMIN_EMAIL,
                company.getId(),
                "  Edited Tenant  ",
                CompanyStatus.DISABLED
        );

        assertThat(updatedCompany.getName()).isEqualTo("Edited Tenant");
        assertThat(updatedCompany.getStatus()).isEqualTo(CompanyStatus.DISABLED);
    }

    @Test
    void rejectsDuplicateCompanyNameOnUpdateIgnoringCase() {
        companyRepository.saveAndFlush(new Company("Existing Update Tenant", CompanyStatus.ACTIVE));
        Company targetCompany = companyRepository.saveAndFlush(new Company("Target Update Tenant", CompanyStatus.ACTIVE));

        assertThatThrownBy(() -> companyService.updateCompany(
                SUPERADMIN_EMAIL,
                targetCompany.getId(),
                "existing update tenant",
                CompanyStatus.ACTIVE
        ))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.COMPANY_ALREADY_EXISTS)
                );
    }

    @Test
    void hardDeletesCompanyUsersAndAuthTokens() {
        Company company = companyRepository.saveAndFlush(new Company("Hard Delete Tenant", CompanyStatus.ACTIVE));
        User superadmin = userRepository.findByEmail(SUPERADMIN_EMAIL).orElseThrow();

        User adminUser = userRepository.saveAndFlush(new User(
                null,
                company,
                "delete-admin@miletos.local",
                "$2a$12$admin-password-hash",
                "Delete",
                "Admin",
                UserRole.ADMIN,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        ));

        User invitedUser = userRepository.saveAndFlush(new User(
                null,
                company,
                "delete-invited@miletos.local",
                null,
                "Delete",
                "Invited",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        ));

        AuthToken inviteToken = authTokenRepository.saveAndFlush(AuthToken.invite(
                company,
                invitedUser,
                adminUser,
                tokenHasher.hash("delete-company-invite-token"),
                Instant.now().plus(24, ChronoUnit.HOURS)
        ));

        AuthToken passwordResetToken = authTokenRepository.saveAndFlush(AuthToken.passwordReset(
                adminUser,
                tokenHasher.hash("delete-company-reset-token"),
                Instant.now().plus(30, ChronoUnit.MINUTES)
        ));

        companyService.deleteCompany(SUPERADMIN_EMAIL, company.getId());

        assertThat(companyRepository.findById(company.getId())).isEmpty();
        assertThat(userRepository.findById(adminUser.getId())).isEmpty();
        assertThat(userRepository.findById(invitedUser.getId())).isEmpty();
        assertThat(userRepository.findById(superadmin.getId())).isPresent();
        assertThat(authTokenRepository.findById(inviteToken.getId())).isEmpty();
        assertThat(authTokenRepository.findById(passwordResetToken.getId())).isEmpty();
    }

    @Test
    void rejectsCompanyCreationWhenActorIsNotSuperadmin() {
        Company company = companyRepository.saveAndFlush(new Company("Regular Actor Tenant", CompanyStatus.ACTIVE));

        User regularUser = userRepository.saveAndFlush(new User(
                null,
                company,
                "regular-company-actor@miletos.local",
                "$2a$12$regular-user-password-hash",
                "Regular",
                "Actor",
                UserRole.USER,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        ));

        assertThatThrownBy(() -> companyService.createCompany(
                regularUser.getEmail(),
                new Company("Forbidden Tenant", CompanyStatus.ACTIVE)
        ))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.FORBIDDEN)
                );
    }

    @Test
    void rejectsDuplicateCompanyNameIgnoringCase() {
        companyRepository.saveAndFlush(new Company("Duplicate Tenant", CompanyStatus.ACTIVE));

        assertThatThrownBy(() -> companyService.createCompany(
                SUPERADMIN_EMAIL,
                new Company("duplicate tenant", CompanyStatus.ACTIVE)
        ))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.COMPANY_ALREADY_EXISTS)
                );
    }

    @Test
    void rejectsUnknownActor() {
        assertThatThrownBy(() -> companyService.createCompany(
                "missing-actor@miletos.local",
                new Company("Unknown Actor Tenant", CompanyStatus.ACTIVE)
        ))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.AUTHENTICATED_USER_NOT_FOUND)
                );
    }
}
