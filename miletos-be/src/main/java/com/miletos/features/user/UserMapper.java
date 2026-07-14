package com.miletos.features.user;

import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.ReportingPolicy;

import com.miletos.features.user.controller.request.UpdateProfilePhotoRequest;
import com.miletos.features.user.controller.response.UserProfileResponse;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.service.input.ProfilePhotoUploadInput;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.ERROR)
public interface UserMapper {

        ProfilePhotoUploadInput toProfilePhotoUploadInput(
                        String actorEmail,
                        UpdateProfilePhotoRequest request);

        @Mapping(target = "companyId", source = "company.id")
        @Mapping(target = "profilePhotoFileId", source = "profilePhotoFile.id")
        UserProfileResponse toResponse(User user);
}
