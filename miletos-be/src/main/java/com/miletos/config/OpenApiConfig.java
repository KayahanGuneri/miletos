package com.miletos.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import io.swagger.v3.oas.models.Components;
import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Info;
import io.swagger.v3.oas.models.security.SecurityRequirement;
import io.swagger.v3.oas.models.security.SecurityScheme;

@Configuration
public class OpenApiConfig {

        public static final String BEARER_AUTH_SCHEME = "bearerAuth";

        @Bean
        public OpenAPI miletosOpenApi() {
                return new OpenAPI()
                                .info(new Info()
                                                .title("Miletos API")
                                                .version("v1")
                                                .description("Miletos backend API documentation"))
                                .addSecurityItem(new SecurityRequirement().addList(BEARER_AUTH_SCHEME))
                                .components(new Components()
                                                .addSecuritySchemes(
                                                                BEARER_AUTH_SCHEME,
                                                                new SecurityScheme()
                                                                                .name(BEARER_AUTH_SCHEME)
                                                                                .type(SecurityScheme.Type.HTTP)
                                                                                .scheme("bearer")
                                                                                .bearerFormat("JWT")));
        }
}