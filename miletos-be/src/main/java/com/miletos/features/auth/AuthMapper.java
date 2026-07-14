package com.miletos.features.auth;

import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.Named;
import org.mapstruct.ReportingPolicy;

import com.miletos.features.auth.command.ChangePasswordCommand;
import com.miletos.features.auth.command.CompletePasswordCommand;
import com.miletos.features.auth.command.ForgotPasswordCommand;
import com.miletos.features.auth.command.LoginCommand;
import com.miletos.features.auth.command.ResetPasswordCommand;
import com.miletos.features.auth.model.LoginResult;
import com.miletos.features.auth.request.ChangePasswordRequest;
import com.miletos.features.auth.request.CompletePasswordRequest;
import com.miletos.features.auth.request.ForgotPasswordRequest;
import com.miletos.features.auth.request.LoginRequest;
import com.miletos.features.auth.request.ResetPasswordRequest;
import com.miletos.features.auth.response.AuthenticatedUserResponse;
import com.miletos.features.auth.response.CompletePasswordResponse;
import com.miletos.features.auth.response.LoginResponse;
import com.miletos.features.auth.response.ResetPasswordResponse;
import com.miletos.features.user.repository.entity.User;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.ERROR)
public interface AuthMapper {

    @Mapping(target = "email", source = "email", qualifiedByName = "trim")
    LoginCommand toLoginCommand(LoginRequest request);

    @Mapping(target = "accessToken", source = "accessToken.value")
    @Mapping(target = "tokenType", constant = "BEARER")
    @Mapping(target = "expiresAt", source = "accessToken.expiresAt")
    @Mapping(target = "user", source = "user")
    LoginResponse toLoginResponse(LoginResult result);

    @Mapping(target = "companyId", source = "company.id")
    AuthenticatedUserResponse toAuthenticatedUserResponse(User user);

    @Mapping(target = "actorEmail", source = "actorEmail")
    @Mapping(target = "currentPassword", source = "request.currentPassword")
    @Mapping(target = "newPassword", source = "request.newPassword")
    ChangePasswordCommand toChangePasswordCommand(
            String actorEmail,
            ChangePasswordRequest request);

    @Mapping(target = "token", source = "token", qualifiedByName = "trim")
    CompletePasswordCommand toCompletePasswordCommand(CompletePasswordRequest request);

    @Mapping(target = "userId", source = "id")
    CompletePasswordResponse toCompletePasswordResponse(User user);

    @Mapping(target = "email", source = "email", qualifiedByName = "trim")
    ForgotPasswordCommand toForgotPasswordCommand(ForgotPasswordRequest request);

    @Mapping(target = "token", source = "token", qualifiedByName = "trim")
    ResetPasswordCommand toResetPasswordCommand(ResetPasswordRequest request);

    @Mapping(target = "userId", source = "id")
    ResetPasswordResponse toResetPasswordResponse(User user);

    @Named("trim")
    default String trim(String value) {
        return value == null ? null : value.trim();
    }
}
