package com.miletos.features.company;

import java.time.Clock;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import com.miletos.common.util.EmailNormalizer;
import com.miletos.features.auth.exception.InviteNotAllowedException;
import com.miletos.features.auth.exception.InviteRoleNotAllowedException;
import com.miletos.features.auth.exception.PasswordSetupRequiredException;
import com.miletos.features.auth.repository.AuthTokenRepository;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.auth.repository.entity.AuthTokenType;
import com.miletos.features.auth.service.SecureTokenGenerator;
import com.miletos.features.auth.service.TokenHasher;
import com.miletos.features.company.exception.CompanyAlreadyExistsException;
import com.miletos.features.company.exception.CompanyDisabledException;
import com.miletos.features.company.exception.CompanyNotFoundException;
import com.miletos.features.company.exception.CompanyOperationForbiddenException;
import com.miletos.features.company.exception.InvalidCompanyNameException;
import com.miletos.features.company.exception.InvalidCompanyStatusException;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.exception.CompanyUserListNotAllowedException;
import com.miletos.features.user.exception.EmailAlreadyExistsException;
import com.miletos.features.user.exception.InvalidUserNameException;
import com.miletos.features.user.exception.UserDisabledException;
import com.miletos.features.user.exception.UserNameUpdateNotAllowedException;
import com.miletos.features.user.exception.UserNotFoundException;
import com.miletos.features.user.repository.UserRepository;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import com.miletos.features.user.service.input.UserNameUpdateInput;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.services.mail.EmailMessage;
import com.miletos.services.mail.EmailSender;
import com.miletos.services.mail.MailTemplateRenderer;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class CompanyService {

    private static final int DEFAULT_PAGE_SIZE = 20;
    private static final int MAX_PAGE_SIZE = 100;

    private final CompanyRepository companyRepository;
    private final UserRepository userRepository;
    private final AuthTokenRepository authTokenRepository;
    private final AuthenticatedActorResolver authenticatedActorResolver;
    private final SecureTokenGenerator secureTokenGenerator;
    private final TokenHasher tokenHasher;
    private final EmailSender emailSender;
    private final MailTemplateRenderer mailTemplateRenderer;
    private final Clock clock;

    @Value("${miletos.web.base-url}")
    private String webBaseUrl;

    @Transactional
    public Company createCompany(
            String actorEmail,
            Company company) {
        User actor = authenticatedActorResolver.resolve(actorEmail);
        validateSuperadmin(actor);

        String companyName = normalizeCompanyName(company.getName());

        if (companyRepository.existsByNameIgnoreCase(
                companyName)) {
            throw new CompanyAlreadyExistsException();
        }

        company.setName(companyName);
        company.setStatus(CompanyStatus.ACTIVE);

        return companyRepository.save(company);
    }

    @Transactional(readOnly = true)
    public Page<Company> listCompanies(
            int page,
            int size) {
        int normalizedPage = Math.max(page, 0);
        int normalizedSize = normalizePageSize(size);

        return companyRepository.findAll(
                PageRequest.of(
                        normalizedPage,
                        normalizedSize,
                        Sort.by(
                                Sort.Direction.DESC,
                                "createdAt")));
    }

    @Transactional
    public Company updateCompany(
            String actorEmail,
            Long companyId,
            String name,
            CompanyStatus status) {
        User actor = authenticatedActorResolver.resolve(actorEmail);
        validateSuperadmin(actor);

        Company company = findCompany(companyId);
        String companyName = normalizeCompanyName(name);
        CompanyStatus companyStatus = requireCompanyStatus(status);

        if (companyRepository
                .existsByNameIgnoreCaseAndIdNot(
                        companyName,
                        companyId)) {
            throw new CompanyAlreadyExistsException();
        }

        company.setName(companyName);
        company.setStatus(companyStatus);

        return companyRepository.save(company);
    }

    @Transactional
    public void deleteCompany(
            String actorEmail,
            Long companyId) {
        User actor = authenticatedActorResolver.resolve(actorEmail);
        validateSuperadmin(actor);

        Company company = findCompany(companyId);
        List<User> companyUsers = userRepository.findAllByCompany_Id(companyId);

        deleteAuthTokens(companyId, companyUsers);

        userRepository.deleteAll(companyUsers);
        userRepository.flush();

        companyRepository.delete(company);
        companyRepository.flush();
    }

    private Company findCompany(Long companyId) {
        return companyRepository
                .findById(companyId)
                .orElseThrow(
                        CompanyNotFoundException::new);
    }

    private void validateSuperadmin(User actor) {
        if (!actor.isSuperAdmin()) {
            throw new CompanyOperationForbiddenException();
        }
    }

    private String normalizeCompanyName(String name) {
        if (name == null || name.isBlank()) {
            throw new InvalidCompanyNameException();
        }

        return name.trim();
    }

    private CompanyStatus requireCompanyStatus(
            CompanyStatus status) {
        if (status == null) {
            throw new InvalidCompanyStatusException();
        }

        return status;
    }

    private int normalizePageSize(int size) {
        if (size <= 0) {
            return DEFAULT_PAGE_SIZE;
        }

        return Math.min(size, MAX_PAGE_SIZE);
    }

    private void deleteAuthTokens(
            Long companyId,
            List<User> companyUsers) {
        Map<Long, AuthToken> authTokens = new LinkedHashMap<>();

        authTokenRepository
                .findAllByTypeAndCompany_Id(
                        AuthTokenType.INVITE,
                        companyId)
                .forEach(authToken -> authTokens.put(
                        authToken.getId(),
                        authToken));

        for (User user : companyUsers) {
            authTokenRepository
                    .findAllByUser_Id(user.getId())
                    .forEach(authToken -> authTokens.put(
                            authToken.getId(),
                            authToken));

            authTokenRepository
                    .findAllByTypeAndCreatedByUser_Id(
                            AuthTokenType.INVITE,
                            user.getId())
                    .forEach(authToken -> authTokens.put(
                            authToken.getId(),
                            authToken));
        }

        authTokenRepository.deleteAll(
                authTokens.values());

        authTokenRepository.flush();
    }

    @Transactional(readOnly = true)
    public List<User> listCompanyUsers(String actorEmail, Long companyId) {
        User actor = authenticatedActorResolver.resolve(actorEmail);
        validateActiveActor(actor);
        Company company = findActiveCompany(companyId);
        if (!actor.isSuperAdmin()
                && (actor.getRole() != UserRole.ADMIN
                        || actor.getCompany() == null
                        || !Objects.equals(actor.getCompany().getId(), company.getId()))) {
            throw new CompanyUserListNotAllowedException();
        }
        return userRepository.findAllByCompany_IdOrderByCreatedAtAsc(company.getId());
    }

    @Transactional
    public User updateUserName(UserNameUpdateInput input) {
        User actor = authenticatedActorResolver.resolve(input.actorEmail());
        validateActiveActor(actor);
        User target = userRepository.findById(input.targetUserId())
                .orElseThrow(UserNotFoundException::new);
        if (!actor.isSuperAdmin()
                && (actor.getRole() != UserRole.ADMIN
                        || Objects.equals(actor.getId(), target.getId())
                        || actor.getCompany() == null || target.getCompany() == null
                        || !Objects.equals(actor.getCompany().getId(), target.getCompany().getId()))) {
            throw new UserNameUpdateNotAllowedException();
        }
        target.setFirstName(normalizeUserName(input.firstName()));
        target.setLastName(normalizeUserName(input.lastName()));
        return userRepository.save(target);
    }

    @Transactional
    public AuthToken inviteUser(String actorEmail, Long companyId, User invitedUser) {
        User actor = authenticatedActorResolver.resolve(actorEmail);
        Company company = findActiveCompany(companyId);
        if (!actor.isSuperAdmin()
                && (actor.getRole() != UserRole.ADMIN || actor.getCompany() == null
                        || !Objects.equals(actor.getCompany().getId(), company.getId()))) {
            throw new InviteNotAllowedException();
        }
        if (invitedUser.getRole() == null) {
            throw new InviteRoleNotAllowedException();
        }
        String email = EmailNormalizer.normalize(invitedUser.getEmail());
        if (userRepository.existsByEmail(email)) {
            throw new EmailAlreadyExistsException();
        }
        invitedUser.setCompany(company);
        invitedUser.setSuperAdmin(false);
        invitedUser.setEmail(email);
        invitedUser.setFirstName(normalizeUserName(invitedUser.getFirstName()));
        invitedUser.setLastName(normalizeUserName(invitedUser.getLastName()));
        invitedUser.setPasswordHash(null);
        invitedUser.setStatus(UserStatus.PENDING);
        invitedUser.setOnboardingStatus(OnboardingStatus.PASSWORD_SETUP_REQUIRED);
        User savedUser = userRepository.save(invitedUser);
        String rawToken = secureTokenGenerator.generateToken();
        AuthToken token = authTokenRepository.save(AuthToken.invite(
                company, savedUser, actor, tokenHasher.hash(rawToken),
                Instant.now(clock).plus(24, ChronoUnit.HOURS)));
        sendInviteEmail(savedUser, rawToken);
        return token;
    }

    private Company findActiveCompany(Long companyId) {
        Company company = findCompany(companyId);
        if (company.getStatus() != CompanyStatus.ACTIVE) {
            throw new CompanyDisabledException();
        }
        return company;
    }

    private void validateActiveActor(User actor) {
        if (actor.getStatus() == UserStatus.DISABLED) {
            throw new UserDisabledException();
        }
        if (actor.getStatus() == UserStatus.PENDING
                || actor.getOnboardingStatus() == OnboardingStatus.PASSWORD_SETUP_REQUIRED) {
            throw new PasswordSetupRequiredException();
        }
        if (actor.getStatus() != UserStatus.ACTIVE) {
            throw new UserDisabledException();
        }
        if (actor.getOnboardingStatus() != OnboardingStatus.COMPLETED
                && actor.getOnboardingStatus() != OnboardingStatus.NOT_REQUIRED) {
            throw new PasswordSetupRequiredException();
        }
    }

    private String normalizeUserName(String name) {
        if (name == null || name.isBlank()) {
            throw new InvalidUserNameException();
        }
        return name.trim();
    }

    private void sendInviteEmail(User user, String rawToken) {
        String link = webBaseUrl + "/complete-password?token=" + rawToken;
        Map<String, String> variables = Map.of("firstName", user.getFirstName(), "inviteLink", link);
        emailSender.send(new EmailMessage(user.getEmail(), "You are invited to Miletos",
                mailTemplateRenderer.renderText("invite-user.txt", variables),
                mailTemplateRenderer.renderHtml("invite-user.html", variables)));
    }
}
