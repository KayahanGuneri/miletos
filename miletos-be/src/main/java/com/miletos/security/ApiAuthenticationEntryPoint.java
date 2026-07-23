package com.miletos.security;

import java.io.IOException;
import java.nio.charset.StandardCharsets;

import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.security.core.AuthenticationException;
import org.springframework.security.oauth2.core.OAuth2AuthenticationException;
import org.springframework.security.web.AuthenticationEntryPoint;
import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.miletos.common.exception.ErrorCode;
import com.miletos.common.response.ApiErrorResponse;
import com.miletos.common.response.ApiErrorResponseFactory;

import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import lombok.RequiredArgsConstructor;

@Component
@RequiredArgsConstructor
public class ApiAuthenticationEntryPoint
                implements AuthenticationEntryPoint {

        private final ObjectMapper objectMapper;
        private final ApiErrorResponseFactory apiErrorResponseFactory;

        @Override
        public void commence(
                        HttpServletRequest request,
                        HttpServletResponse response,
                        AuthenticationException authException) throws IOException, ServletException {
                ErrorCode errorCode = resolveErrorCode(authException);
                ApiErrorResponse body = apiErrorResponseFactory.create(
                                errorCode,
                                errorCode.name(),
                                request.getRequestURI());

                response.setStatus(
                                HttpStatus.UNAUTHORIZED.value());

                response.setContentType(
                                MediaType.APPLICATION_JSON_VALUE);

                response.setCharacterEncoding(
                                StandardCharsets.UTF_8.name());

                objectMapper.writeValue(
                                response.getOutputStream(),
                                body);
        }

        ErrorCode resolveErrorCode(AuthenticationException exception) {
                if (!(exception instanceof OAuth2AuthenticationException oauthException)) {
                        return ErrorCode.UNAUTHENTICATED;
                }

                String oauthCode = oauthException.getError().getErrorCode();
                String description = oauthException.getError().getDescription();
                String normalizedDescription = description == null ? "" : description.toLowerCase();

                if ("inactive_session".equals(oauthCode)
                                || normalizedDescription.contains("session is not active")) {
                        return ErrorCode.INACTIVE_SESSION;
                }

                if (normalizedDescription.contains("expired")) {
                        return ErrorCode.EXPIRED_TOKEN;
                }

                if (normalizedDescription.contains("malformed")
                                || normalizedDescription.contains("not a jwt")) {
                        return ErrorCode.MALFORMED_TOKEN;
                }

                return ErrorCode.INVALID_TOKEN;
        }
}
