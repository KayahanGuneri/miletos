package com.miletos.config;

import static org.assertj.core.api.Assertions.assertThat;

import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.security.SecurityRequirement;
import io.swagger.v3.oas.models.security.SecurityScheme;
import org.junit.jupiter.api.Test;

class OpenApiConfigTest {

    private final OpenApiConfig openApiConfig = new OpenApiConfig();

    @Test
    void createsOpenApiInfo() {
        OpenAPI openAPI = openApiConfig.miletosOpenApi();

        assertThat(openAPI.getInfo().getTitle()).isEqualTo("Miletos API");
        assertThat(openAPI.getInfo().getVersion()).isEqualTo("v1");
        assertThat(openAPI.getInfo().getDescription()).isEqualTo("Miletos backend API documentation");
    }

    @Test
    void createsJwtBearerSecurityScheme() {
        OpenAPI openAPI = openApiConfig.miletosOpenApi();

        SecurityScheme securityScheme = openAPI
                .getComponents()
                .getSecuritySchemes()
                .get(OpenApiConfig.BEARER_AUTH_SCHEME);

        assertThat(securityScheme).isNotNull();
        assertThat(securityScheme.getType()).isEqualTo(SecurityScheme.Type.HTTP);
        assertThat(securityScheme.getScheme()).isEqualTo("bearer");
        assertThat(securityScheme.getBearerFormat()).isEqualTo("JWT");

        assertThat(openAPI.getSecurity())
                .extracting(SecurityRequirement::keySet)
                .anySatisfy(keys -> assertThat(keys).contains(OpenApiConfig.BEARER_AUTH_SCHEME));
    }
}