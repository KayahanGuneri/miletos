package com.miletos.config;

import com.miletos.features.workflowruntime.documentation.WorkflowRuntimeSchemas;
import io.swagger.v3.core.converter.ModelConverters;
import io.swagger.v3.oas.models.Components;
import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Info;
import io.swagger.v3.oas.models.media.Content;
import io.swagger.v3.oas.models.media.MediaType;
import io.swagger.v3.oas.models.media.Schema;
import io.swagger.v3.oas.models.responses.ApiResponses;
import io.swagger.v3.oas.models.security.SecurityRequirement;
import io.swagger.v3.oas.models.security.SecurityScheme;
import java.util.Set;
import org.springdoc.core.customizers.OperationCustomizer;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class OpenApiConfig {

  public static final String BEARER_AUTH_SCHEME = "bearerAuth";
  private static final Set<String> WORKFLOW_RUNTIME_PROXY_CONTROLLERS =
      Set.of(
          "com.miletos.features.workflowruntime.controller.HTTPTriggerController",
          "com.miletos.features.workflowruntime.controller.WorkflowRuntimeController");
  private static final String WORKFLOW_RUNTIME_ERROR_SCHEMA = "#/components/schemas/ApiError";

  @Bean
  public OpenAPI miletosOpenApi() {
    Components components =
        new Components()
            .addSecuritySchemes(
                BEARER_AUTH_SCHEME,
                new SecurityScheme()
                    .name(BEARER_AUTH_SCHEME)
                    .type(SecurityScheme.Type.HTTP)
                    .scheme("bearer")
                    .bearerFormat("JWT"));
    ModelConverters.getInstance()
        .read(WorkflowRuntimeSchemas.ApiError.class)
        .forEach(components::addSchemas);
    return new OpenAPI()
        .info(
            new Info()
                .title("Miletos API")
                .version("v1")
                .description("Miletos backend API documentation"))
        .addSecurityItem(new SecurityRequirement().addList(BEARER_AUTH_SCHEME))
        .components(components);
  }

  @Bean
  public OperationCustomizer workflowRuntimeErrorResponses() {
    return (operation, handlerMethod) -> {
      if (!WORKFLOW_RUNTIME_PROXY_CONTROLLERS.contains(handlerMethod.getBeanType().getName())) {
        return operation;
      }
      ApiResponses responses = operation.getResponses();
      if (responses == null) {
        responses = new ApiResponses();
        operation.setResponses(responses);
      }
      addWorkflowRuntimeError(responses, "400", "Bad request");
      addWorkflowRuntimeError(responses, "401", "Unauthorized");
      addWorkflowRuntimeError(responses, "403", "Forbidden");
      addWorkflowRuntimeError(responses, "404", "Not found");
      addWorkflowRuntimeError(responses, "409", "Conflict");
      addWorkflowRuntimeError(responses, "422", "Unprocessable entity");
      addWorkflowRuntimeError(responses, "500", "Internal server error");
      addWorkflowRuntimeError(responses, "503", "Workflow runtime unavailable");
      return operation;
    };
  }

  private static void addWorkflowRuntimeError(
      ApiResponses responses, String status, String description) {
    responses.putIfAbsent(
        status,
        new io.swagger.v3.oas.models.responses.ApiResponse()
            .description(description)
            .content(
                new Content()
                    .addMediaType(
                        org.springframework.http.MediaType.APPLICATION_JSON_VALUE,
                        new MediaType()
                            .schema(new Schema<>().$ref(WORKFLOW_RUNTIME_ERROR_SCHEMA)))));
  }
}
