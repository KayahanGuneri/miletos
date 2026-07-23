package com.miletos.features.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import com.miletos.features.auth.command.ChangePasswordCommand;
import com.miletos.features.auth.command.ForgotPasswordCommand;
import com.miletos.features.auth.request.ChangePasswordRequest;
import com.miletos.features.auth.request.ForgotPasswordRequest;
import com.miletos.features.auth.service.AuthService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;

class AuthControllerStatusContractTest {

    private AuthService authService;
    private AuthMapper authMapper;
    private AuthController authController;

    @BeforeEach
    void setUp() {
        authService = mock(AuthService.class);
        authMapper = mock(AuthMapper.class);

        authController =
                new AuthController(
                        authService,
                        authMapper
                );
    }

    @Test
    void forgotPasswordReturnsAcceptedWithoutBody() {
        ForgotPasswordRequest request =
                new ForgotPasswordRequest(
                        "user@example.com"
                );

        ForgotPasswordCommand command =
                new ForgotPasswordCommand(
                        "user@example.com"
                );

        when(
                authMapper.toForgotPasswordCommand(
                        request
                )
        ).thenReturn(command);

        ResponseEntity<Void> response =
                authController.forgotPassword(
                        request
                );

        verify(authService)
                .requestPasswordReset(command);

        assertThat(response.getStatusCode())
                .isEqualTo(HttpStatus.ACCEPTED);

        assertThat(response.getBody()).isNull();
    }

    @Test
    void changePasswordReturnsNoContent() {
        ChangePasswordRequest request =
                new ChangePasswordRequest(
                        "Current123!",
                        "NewPassword123!"
                );

        ChangePasswordCommand command =
                new ChangePasswordCommand(
                        "user@example.com",
                        "Current123!",
                        "NewPassword123!"
                );

        when(
                authMapper.toChangePasswordCommand(
                        "user@example.com",
                        request
                )
        ).thenReturn(command);

        ResponseEntity<Void> response =
                authController.changePassword(
                        request,
                        "user@example.com"
                );

        verify(authService)
                .changePassword(command);

        assertThat(response.getStatusCode())
                .isEqualTo(HttpStatus.NO_CONTENT);

        assertThat(response.getBody()).isNull();
    }
}
