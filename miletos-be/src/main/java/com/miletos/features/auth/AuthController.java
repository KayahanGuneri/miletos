package com.miletos.features.auth;

import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

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
import com.miletos.features.auth.response.CompletePasswordResponse;
import com.miletos.features.auth.response.LoginResponse;
import com.miletos.features.auth.response.ResetPasswordResponse;
import com.miletos.features.auth.service.AuthService;
import com.miletos.features.user.repository.entity.User;
import com.miletos.security.authorization.Authorize;

import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/auth")
@RequiredArgsConstructor
public class AuthController {

        private final AuthService authService;
        private final AuthMapper authMapper;

        @PostMapping("/login")
        public ResponseEntity<LoginResponse> login(
                        @Valid @RequestBody LoginRequest request) {
                LoginCommand command = authMapper.toLoginCommand(request);

                LoginResult result = authService.login(command);

                return ResponseEntity.ok(
                                authMapper.toLoginResponse(result));
        }

        @PostMapping("/complete-password")
        public ResponseEntity<CompletePasswordResponse> completePassword(
                        @Valid @RequestBody CompletePasswordRequest request) {
                CompletePasswordCommand command = authMapper.toCompletePasswordCommand(
                                request);

                User user = authService.completePassword(command);

                return ResponseEntity.ok(
                                authMapper.toCompletePasswordResponse(
                                                user));
        }

        @PostMapping("/forgot-password")
        public ResponseEntity<Void> forgotPassword(
                        @Valid @RequestBody ForgotPasswordRequest request) {
                ForgotPasswordCommand command = authMapper.toForgotPasswordCommand(
                                request);

                authService.requestPasswordReset(command);

                return ResponseEntity.accepted().build();
        }

        @PostMapping("/reset-password")
        public ResponseEntity<ResetPasswordResponse> resetPassword(
                        @Valid @RequestBody ResetPasswordRequest request) {
                ResetPasswordCommand command = authMapper.toResetPasswordCommand(
                                request);

                User user = authService.resetPassword(command);

                return ResponseEntity.ok(
                                authMapper.toResetPasswordResponse(user));
        }

        @Authorize
        @PostMapping("/change-password")
        public ResponseEntity<Void> changePassword(
                        @Valid @RequestBody ChangePasswordRequest request,
                        @AuthenticationPrincipal(expression = "subject") String actorEmail) {
                ChangePasswordCommand command = authMapper.toChangePasswordCommand(
                                actorEmail,
                                request);

                authService.changePassword(command);

                return ResponseEntity.noContent().build();
        }

}
