package com.miletos.features.company;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.repository.UserRepository;
import java.util.List;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest
@Transactional
class CompanyServiceUserManagementTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL = "company-user-list-superadmin@miletos.local";

    @Autowired
    private CompanyService companyService;

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

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
                "List",
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
    void superadminListsUsersForAnyCompany() {
        Company company = companyRepository.saveAndFlush(new Company("List Tenant", CompanyStatus.ACTIVE));
        User user = createActiveUser(company, "list-user@miletos.local", UserRole.USER);

        List<User> users = companyService.listCompanyUsers(SUPERADMIN_EMAIL, company.getId());

        assertThat(users)
                .extracting(User::getEmail)
                .contains(user.getEmail());
    }

    @Test
    void companyAdminListsUsersForOwnCompany() {
        Company company = companyRepository.saveAndFlush(new Company("Own List Tenant", CompanyStatus.ACTIVE));
        User companyAdmin = createActiveUser(company, "own-list-admin@miletos.local", UserRole.ADMIN);
        User user = createActiveUser(company, "own-list-user@miletos.local", UserRole.USER);

        List<User> users = companyService.listCompanyUsers(companyAdmin.getEmail(), company.getId());

        assertThat(users)
                .extracting(User::getEmail)
                .contains(companyAdmin.getEmail(), user.getEmail());
    }

    @Test
    void companyAdminCannotListUsersForAnotherCompany() {
        Company ownCompany = companyRepository.saveAndFlush(new Company("Own Forbidden List Tenant", CompanyStatus.ACTIVE));
        Company otherCompany = companyRepository.saveAndFlush(new Company("Other Forbidden List Tenant", CompanyStatus.ACTIVE));
        User companyAdmin = createActiveUser(ownCompany, "forbidden-list-admin@miletos.local", UserRole.ADMIN);

        assertThatThrownBy(() -> companyService.listCompanyUsers(companyAdmin.getEmail(), otherCompany.getId()))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.FORBIDDEN)
                );
    }

    @Test
    void regularUserCannotListCompanyUsers() {
        Company company = companyRepository.saveAndFlush(new Company("User Forbidden List Tenant", CompanyStatus.ACTIVE));
        User user = createActiveUser(company, "forbidden-list-user@miletos.local", UserRole.USER);

        assertThatThrownBy(() -> companyService.listCompanyUsers(user.getEmail(), company.getId()))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.FORBIDDEN)
                );
    }

    private User createActiveUser(Company company, String email, UserRole role) {
        return userRepository.saveAndFlush(new User(
                null,
                company,
                email,
                "$2a$12$company-user-list-password-hash",
                "Company",
                "User",
                role,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        ));
    }
}
