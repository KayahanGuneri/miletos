package com.miletos.features.company;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import com.miletos.features.user.service.input.UserNameUpdateInput;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.repository.UserRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest(properties = {
        "miletos.security.jwt.secret=test-management-secret-key-for-miletos-auth-flow-32-bytes-minimum",
        "miletos.security.jwt.issuer=https://miletos-management-test.local",
        "miletos.security.jwt.access-token-minutes=30"
})
@Transactional
class CompanyUserNameUpdateTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL = "user-management-superadmin@miletos.local";

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
                "Management",
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
    void superadminUpdatesAnyUserNameWithoutChangingEmail() {
        Company company = companyRepository.saveAndFlush(new Company("Management Superadmin Tenant", CompanyStatus.ACTIVE));
        User targetUser = saveUser(company, "superadmin-target@miletos.local", UserRole.USER);

        User updatedUser = companyService.updateUserName(new UserNameUpdateInput(
                SUPERADMIN_EMAIL,
                targetUser.getId(),
                " Updated ",
                " User "
        ));

        assertThat(updatedUser.getFirstName()).isEqualTo("Updated");
        assertThat(updatedUser.getLastName()).isEqualTo("User");
        assertThat(updatedUser.getEmail()).isEqualTo("superadmin-target@miletos.local");
    }

    @Test
    void companyAdminUpdatesOtherUserInOwnCompany() {
        Company company = companyRepository.saveAndFlush(new Company("Management Own Tenant", CompanyStatus.ACTIVE));
        User companyAdmin = saveUser(company, "company-admin-management@miletos.local", UserRole.ADMIN);
        User targetUser = saveUser(company, "own-company-target@miletos.local", UserRole.USER);

        User updatedUser = companyService.updateUserName(new UserNameUpdateInput(
                companyAdmin.getEmail(),
                targetUser.getId(),
                "Own",
                "Company"
        ));

        assertThat(updatedUser.getFirstName()).isEqualTo("Own");
        assertThat(updatedUser.getLastName()).isEqualTo("Company");
        assertThat(updatedUser.getEmail()).isEqualTo("own-company-target@miletos.local");
    }

    @Test
    void companyAdminCannotUpdateOwnName() {
        Company company = companyRepository.saveAndFlush(new Company("Management Self Tenant", CompanyStatus.ACTIVE));
        User companyAdmin = saveUser(company, "self-company-admin@miletos.local", UserRole.ADMIN);

        assertThatThrownBy(() -> companyService.updateUserName(new UserNameUpdateInput(
                companyAdmin.getEmail(),
                companyAdmin.getId(),
                "Self",
                "Update"
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.USER_NAME_UPDATE_NOT_ALLOWED)
                );
    }

    @Test
    void companyAdminCannotUpdateUserFromAnotherCompany() {
        Company ownCompany = companyRepository.saveAndFlush(new Company("Management Company A", CompanyStatus.ACTIVE));
        Company otherCompany = companyRepository.saveAndFlush(new Company("Management Company B", CompanyStatus.ACTIVE));
        User companyAdmin = saveUser(ownCompany, "cross-company-admin@miletos.local", UserRole.ADMIN);
        User targetUser = saveUser(otherCompany, "cross-company-target@miletos.local", UserRole.USER);

        assertThatThrownBy(() -> companyService.updateUserName(new UserNameUpdateInput(
                companyAdmin.getEmail(),
                targetUser.getId(),
                "Cross",
                "Company"
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.USER_NAME_UPDATE_NOT_ALLOWED)
                );
    }

    @Test
    void regularUserCannotUpdateNames() {
        Company company = companyRepository.saveAndFlush(new Company("Management Regular Tenant", CompanyStatus.ACTIVE));
        User actor = saveUser(company, "regular-management-actor@miletos.local", UserRole.USER);
        User targetUser = saveUser(company, "regular-management-target@miletos.local", UserRole.USER);

        assertThatThrownBy(() -> companyService.updateUserName(new UserNameUpdateInput(
                actor.getEmail(),
                targetUser.getId(),
                "Regular",
                "Update"
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.USER_NAME_UPDATE_NOT_ALLOWED)
                );
    }

    @Test
    void rejectsUnknownTargetUser() {
        assertThatThrownBy(() -> companyService.updateUserName(new UserNameUpdateInput(
                SUPERADMIN_EMAIL,
                9_999_999L,
                "Missing",
                "Target"
        )))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.USER_NOT_FOUND)
                );
    }

    private User saveUser(Company company, String email, UserRole role) {
        return userRepository.saveAndFlush(new User(
                null,
                company,
                email,
                "$2a$12$management-test-password-hash",
                "Original",
                "Name",
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
