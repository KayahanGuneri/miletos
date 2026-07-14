package com.miletos.features.company;

import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.ReportingPolicy;
import org.springframework.data.domain.Page;

import com.miletos.features.company.controller.request.CreateCompanyRequest;
import com.miletos.features.company.controller.request.InviteUserRequest;
import com.miletos.features.company.controller.request.UpdateUserNameRequest;
import com.miletos.features.company.controller.response.CompanyPageResponse;
import com.miletos.features.company.controller.response.CompanyResponse;
import com.miletos.features.company.controller.response.InviteUserResponse;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.auth.repository.entity.AuthToken;
import com.miletos.features.user.controller.response.UserProfileResponse;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.service.input.UserNameUpdateInput;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.ERROR)
public interface CompanyMapper {

    @Mapping(target = "id", ignore = true)
    @Mapping(target = "status", ignore = true)
    @Mapping(target = "createdAt", ignore = true)
    @Mapping(target = "updatedAt", ignore = true)
    Company toEntity(CreateCompanyRequest request);

    CompanyResponse toResponse(Company company);

    @Mapping(target = "company", ignore = true)
    @Mapping(target = "passwordHash", ignore = true)
    @Mapping(target = "status", ignore = true)
    @Mapping(target = "onboardingStatus", ignore = true)
    @Mapping(target = "id", ignore = true)
    @Mapping(target = "superAdmin", ignore = true)
    @Mapping(target = "profilePhotoFile", ignore = true)
    @Mapping(target = "lastLoginAt", ignore = true)
    @Mapping(target = "createdAt", ignore = true)
    @Mapping(target = "updatedAt", ignore = true)
    User toInvitedUser(InviteUserRequest request);

    default InviteUserResponse toInviteUserResponse(AuthToken authToken) {
        User user = authToken.getUser();
        return new InviteUserResponse(
                user.getId(),
                authToken.getCompany().getId(),
                user.getEmail(),
                user.getFirstName(),
                user.getLastName(),
                enumName(user.getRole()),
                enumName(user.getStatus()),
                enumName(user.getOnboardingStatus()),
                authToken.getExpiresAt());
    }

    @Mapping(target = "targetUserId", source = "targetUserId")
    UserNameUpdateInput toUserNameUpdateInput(
            String actorEmail,
            Long targetUserId,
            UpdateUserNameRequest request);

    @Mapping(target = "companyId", source = "company.id")
    @Mapping(target = "profilePhotoFileId", source = "profilePhotoFile.id")
    UserProfileResponse toUserResponse(User user);

    default CompanyPageResponse toPageResponse(Page<Company> page) {
        return new CompanyPageResponse(
                page.getContent().stream().map(this::toResponse).toList(),
                page.getNumber(),
                page.getSize(),
                page.getTotalElements(),
                page.getTotalPages(),
                page.isFirst(),
                page.isLast());
    }

    private static String enumName(Enum<?> value) {
        return value == null ? null : value.name();
    }
}
