package com.miletos.features.company;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.auth.repository.entity.AuthTokenType;
import com.miletos.common.exception.MiletosException;
import com.miletos.common.exception.ErrorCode;
import com.miletos.services.mail.EmailMessage;
import com.miletos.services.mail.EmailSender;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.testsupport.PostgreSqlContainerSupport;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.repository.UserRepository;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Primary;
import org.springframework.transaction.annotation.Transactional;

@SpringBootTest(properties = {
        "miletos.web.base-url=http://localhost:3000"
})
@Transactional
class CompanyServiceInvitationTest extends PostgreSqlContainerSupport {

    private static final String SUPERADMIN_EMAIL = "invite-checkpoint-superadmin@miletos.local";

    @Autowired
    private CompanyService companyService;

    @Autowired
    private CompanyRepository companyRepository;

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private AuthTokenRepository authTokenRepository;

    @Autowired
    private RecordingEmailSender recordingEmailSender;

    @BeforeEach
    void prepareTestState() {
        recordingEmailSender.clear();

        if (userRepository.findByEmail(SUPERADMIN_EMAIL).isEmpty()) {
            userRepository.saveAndFlush(new User(
                    null,
                    null,
                    SUPERADMIN_EMAIL,
                    "unused-test-password-hash",
                    "Invite",
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
    }

    @Test
    void superadminInvitesUserAndSendsMail() {
        Company company = companyRepository.saveAndFlush(new Company("Invite Tenant", CompanyStatus.ACTIVE));

        User invitedUser = new User(
                null,
                null,
                " invited-user@miletos.local ",
                "temporary-value",
                "Invited",
                "User",
                UserRole.USER,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        );

        AuthToken inviteToken = companyService.inviteUser(SUPERADMIN_EMAIL, company.getId(), invitedUser);

        User savedUser = userRepository.findByEmail("invited-user@miletos.local").orElseThrow();

        assertThat(savedUser.getCompany().getId()).isEqualTo(company.getId());
        assertThat(savedUser.isSuperAdmin()).isFalse();
        assertThat(savedUser.getPasswordHash()).isNull();
        assertThat(savedUser.getStatus()).isEqualTo(UserStatus.PENDING);
        assertThat(savedUser.getOnboardingStatus()).isEqualTo(OnboardingStatus.PASSWORD_SETUP_REQUIRED);
        assertThat(inviteToken.getTokenHash()).hasSize(64);
        assertThat(inviteToken.getExpiresAt()).isAfter(inviteToken.getCreatedAt());
        assertThat(authTokenRepository.findAllByTypeAndUser_Id(
                AuthTokenType.INVITE,
                savedUser.getId()
        )).hasSize(1);
        assertThat(recordingEmailSender.messages()).hasSize(1);
        assertThat(recordingEmailSender.messages().getFirst().to()).isEqualTo("invited-user@miletos.local");
        assertThat(recordingEmailSender.messages().getFirst().textBody())
                .contains("http://localhost:3000/complete-password?token=");
    }

    @Test
    void companyAdminCanInviteUserToOwnCompany() {
        Company company = companyRepository.saveAndFlush(new Company("Company Admin Invite Tenant", CompanyStatus.ACTIVE));

        User companyAdmin = userRepository.saveAndFlush(new User(
                null,
                company,
                "company-admin@miletos.local",
                "$2a$12$company-admin-password-hash",
                "Company",
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

        User invitedUser = new User(
                null,
                null,
                "company-admin-invited-user@miletos.local",
                null,
                "Company",
                "User",
                UserRole.MOD,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        );

        AuthToken inviteToken = companyService.inviteUser(companyAdmin.getEmail(), company.getId(), invitedUser);

        assertThat(inviteToken.getCompany().getId()).isEqualTo(company.getId());
        assertThat(inviteToken.getUser().getRole()).isEqualTo(UserRole.MOD);
        assertThat(recordingEmailSender.messages()).hasSize(1);
    }

    @Test
    void companyAdminCannotInviteUserToAnotherCompany() {
        Company ownCompany = companyRepository.saveAndFlush(new Company("Own Tenant", CompanyStatus.ACTIVE));
        Company otherCompany = companyRepository.saveAndFlush(new Company("Other Tenant", CompanyStatus.ACTIVE));

        User companyAdmin = userRepository.saveAndFlush(new User(
                null,
                ownCompany,
                "limited-company-admin@miletos.local",
                "$2a$12$company-admin-password-hash",
                "Limited",
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

        User invitedUser = new User(
                null,
                null,
                "cross-company-invite@miletos.local",
                null,
                "Cross",
                "Company",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        );

        assertThatThrownBy(() -> companyService.inviteUser(companyAdmin.getEmail(), otherCompany.getId(), invitedUser))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.FORBIDDEN)
                );

        assertThat(recordingEmailSender.messages()).isEmpty();
    }

    @Test
    void rejectsDuplicateEmail() {
        Company company = companyRepository.saveAndFlush(new Company("Duplicate Invite Tenant", CompanyStatus.ACTIVE));

        userRepository.saveAndFlush(new User(
                null,
                company,
                "duplicate-invite@miletos.local",
                null,
                "Duplicate",
                "Existing",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        ));

        User invitedUser = new User(
                null,
                null,
                "duplicate-invite@miletos.local",
                null,
                "Duplicate",
                "New",
                UserRole.USER,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        );

        assertThatThrownBy(() -> companyService.inviteUser(SUPERADMIN_EMAIL, company.getId(), invitedUser))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.EMAIL_ALREADY_EXISTS)
                );

        assertThat(recordingEmailSender.messages()).isEmpty();
    }

    @Test
    void rejectsMissingInviteRole() {
        Company company = companyRepository.saveAndFlush(new Company("Invalid Role Invite Tenant", CompanyStatus.ACTIVE));

        User invitedUser = new User(
                null,
                null,
                "invalid-role-invite@miletos.local",
                null,
                "Invalid",
                "Role",
                null,
                false,
                UserStatus.PENDING,
                OnboardingStatus.PASSWORD_SETUP_REQUIRED,
                null,
                null,
                null,
                null
        );

        assertThatThrownBy(() -> companyService.inviteUser(SUPERADMIN_EMAIL, company.getId(), invitedUser))
                .isInstanceOfSatisfying(MiletosException.class, exception ->
                        assertThat(exception.getCode()).isEqualTo(ErrorCode.INVITE_ROLE_NOT_ALLOWED)
                );

        assertThat(recordingEmailSender.messages()).isEmpty();
    }

    @TestConfiguration
    static class CompanyServiceInvitationTestConfig {

        @Bean
        @Primary
        RecordingEmailSender recordingEmailSender() {
            return new RecordingEmailSender();
        }
    }

    static class RecordingEmailSender implements EmailSender {

        private final List<MailMessage> messages = new ArrayList<>();

        @Override
        public void send(EmailMessage message) {
            messages.add(new MailMessage(message.to(), message.subject(), message.textBody(), message.htmlBody()));
        }

        List<MailMessage> messages() {
            return messages;
        }

        void clear() {
            messages.clear();
        }
    }

    record MailMessage(String to, String subject, String textBody, String htmlBody) {
    }
}
