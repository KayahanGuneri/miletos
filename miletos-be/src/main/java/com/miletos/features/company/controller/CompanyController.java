package com.miletos.features.company.controller;

import java.util.List;

import org.springframework.data.domain.Page;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.miletos.features.company.CompanyMapper;
import com.miletos.features.company.CompanyService;
import com.miletos.features.company.controller.request.CreateCompanyRequest;
import com.miletos.features.company.controller.request.InviteUserRequest;
import com.miletos.features.company.controller.request.UpdateCompanyRequest;
import com.miletos.features.company.controller.request.UpdateUserNameRequest;
import com.miletos.features.company.controller.response.CompanyPageResponse;
import com.miletos.features.company.controller.response.CompanyResponse;
import com.miletos.features.company.controller.response.InviteUserResponse;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.user.controller.response.UserProfileResponse;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.service.input.UserNameUpdateInput;
import com.miletos.security.authorization.Authorize;
import com.miletos.security.authorization.RequiredRole;

import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;

@RestController
@RequiredArgsConstructor
public class CompanyController {

    private final CompanyService companyService;
    private final CompanyMapper companyMapper;

    @Authorize(RequiredRole.SUPERADMIN)
    @GetMapping("/api/companies")
    public ResponseEntity<CompanyPageResponse> listCompanies(
            @RequestParam(defaultValue = "0") int page,
            @RequestParam(defaultValue = "20") int size) {
        Page<Company> companies = companyService.listCompanies(
                page,
                size);

        return ResponseEntity.ok(
                companyMapper.toPageResponse(companies));
    }

    @Authorize(RequiredRole.SUPERADMIN)
    @PostMapping("/api/companies")
    public ResponseEntity<CompanyResponse> createCompany(
            @Valid @RequestBody CreateCompanyRequest request,
            @AuthenticationPrincipal(expression = "subject") String actorEmail) {
        Company company = companyMapper.toEntity(request);

        Company createdCompany = companyService.createCompany(
                actorEmail,
                company);

        return ResponseEntity
                .status(HttpStatus.CREATED)
                .body(
                        companyMapper.toResponse(createdCompany));
    }

    @Authorize(RequiredRole.SUPERADMIN)
    @PutMapping("/api/companies/{companyId}")
    public ResponseEntity<CompanyResponse> updateCompany(
            @PathVariable Long companyId,
            @Valid @RequestBody UpdateCompanyRequest request,
            @AuthenticationPrincipal(expression = "subject") String actorEmail) {
        Company updatedCompany = companyService.updateCompany(
                actorEmail,
                companyId,
                request.name(),
                request.status());

        return ResponseEntity.ok(
                companyMapper.toResponse(updatedCompany));
    }

    @Authorize(RequiredRole.SUPERADMIN)
    @DeleteMapping("/api/companies/{companyId}")
    public ResponseEntity<Void> deleteCompany(
            @PathVariable Long companyId,
            @AuthenticationPrincipal(expression = "subject") String actorEmail) {
        companyService.deleteCompany(
                actorEmail,
                companyId);

        return ResponseEntity.noContent().build();
    }

    @Authorize(RequiredRole.ADMIN)
    @GetMapping("/api/companies/{companyId}/users")
    public ResponseEntity<List<UserProfileResponse>> listCompanyUsers(
            @PathVariable Long companyId,
            @AuthenticationPrincipal(expression = "subject") String actorEmail) {
        List<UserProfileResponse> users = companyService.listCompanyUsers(actorEmail, companyId)
                .stream().map(companyMapper::toUserResponse).toList();
        return ResponseEntity.ok(users);
    }

    @Authorize(RequiredRole.ADMIN)
    @PostMapping("/api/auth/invitations")
    public ResponseEntity<InviteUserResponse> inviteUser(
            @Valid @RequestBody InviteUserRequest request,
            @AuthenticationPrincipal(expression = "subject") String actorEmail) {
        User invitedUser = companyMapper.toInvitedUser(request);
        return ResponseEntity.status(HttpStatus.CREATED).body(
                companyMapper.toInviteUserResponse(companyService.inviteUser(
                        actorEmail, request.companyId(), invitedUser)));
    }

    @Authorize(RequiredRole.ADMIN)
    @PutMapping("/api/users/{userId}/name")
    public ResponseEntity<UserProfileResponse> updateUserName(
            @PathVariable Long userId,
            @Valid @RequestBody UpdateUserNameRequest request,
            @AuthenticationPrincipal(expression = "subject") String actorEmail) {
        UserNameUpdateInput input = companyMapper.toUserNameUpdateInput(actorEmail, userId, request);
        return ResponseEntity.ok(companyMapper.toUserResponse(companyService.updateUserName(input)));
    }
}
