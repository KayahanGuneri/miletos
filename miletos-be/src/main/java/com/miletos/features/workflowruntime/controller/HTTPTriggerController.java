package com.miletos.features.workflowruntime.controller;

import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeResponse;
import com.miletos.features.workflowruntime.controller.request.LegacyCreateHTTPTriggerRequest;
import com.miletos.features.workflowruntime.documentation.WorkflowRuntimeSchemas;
import com.miletos.features.workflowruntime.service.WorkflowRuntimeService;
import com.miletos.features.workflowruntime.service.WorkflowTriggerManagementService;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.security.authorization.Authorize;
import com.miletos.security.authorization.RequiredRole;

import jakarta.validation.Valid;

import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.media.Content;
import io.swagger.v3.oas.annotations.media.Schema;
import io.swagger.v3.oas.annotations.responses.ApiResponse;
import io.swagger.v3.oas.annotations.responses.ApiResponses;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping(
        value = "/api/v1/http-triggers",
        produces = MediaType.APPLICATION_JSON_VALUE)
@RequiredArgsConstructor
public class HTTPTriggerController {

    private final WorkflowRuntimeService workflowRuntimeService;
    private final WorkflowTriggerManagementService triggerManagementService;
    private final AuthenticatedActorResolver authenticatedActorResolver;

    @Authorize(RequiredRole.COMPANY_ADMIN)
    @PostMapping(consumes = MediaType.APPLICATION_JSON_VALUE)
    @Operation(
            summary = "Create an HTTP trigger",
            description = "Creates an immutable ASYNC HTTP webhook binding from a persisted workflow. "
                    + "The public URL is returned only by this successful response.")
    @io.swagger.v3.oas.annotations.parameters.RequestBody(
            required = true,
            content = @Content(
                    mediaType = "application/json",
                    schema = @Schema(
                            implementation =
                                    LegacyCreateHTTPTriggerRequest.class)))
    @ApiResponses({
            @ApiResponse(
                    responseCode = "201",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(
                                    implementation =
                                            WorkflowRuntimeSchemas.CreateHTTPTriggerResponse.class))),
            @ApiResponse(
                    responseCode = "400",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "401",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "403",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "422",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "503",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
    })
    public ResponseEntity<byte[]> create(
            @Valid @RequestBody LegacyCreateHTTPTriggerRequest body,
            @AuthenticationPrincipal(expression = "subject") String actorEmail,
            @RequestHeader HttpHeaders browserHeaders) {
        User user = authenticatedActorResolver.resolve(actorEmail);
        return toResponseEntity(
                triggerManagementService.createHTTPTrigger(
                        user, body.workflowId(), body.triggerNodeId(), browserHeaders));
    }

    @Authorize(RequiredRole.COMPANY_ADMIN)
    @GetMapping("/{triggerId}")
    @Operation(summary = "Get an HTTP trigger without its secret URL")
    @ApiResponses({
            @ApiResponse(
                    responseCode = "200",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(
                                    implementation =
                                            WorkflowRuntimeSchemas.HTTPTriggerResponse.class))),
            @ApiResponse(
                    responseCode = "401",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "403",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "404",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
    })
    public ResponseEntity<byte[]> get(
            @AuthenticationPrincipal(expression = "subject") String actorEmail,
            @PathVariable String triggerId,
            @RequestHeader HttpHeaders browserHeaders) {
        return toResponseEntity(
                workflowRuntimeService.getHTTPTrigger(
                        actorEmail, triggerId, browserHeaders));
    }

    @Authorize(RequiredRole.COMPANY_ADMIN)
    @PostMapping("/{triggerId}/disable")
    @Operation(summary = "Idempotently disable an HTTP trigger")
    @ApiResponses({
            @ApiResponse(
                    responseCode = "200",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(
                                    implementation =
                                            WorkflowRuntimeSchemas.HTTPTriggerResponse.class))),
            @ApiResponse(
                    responseCode = "401",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "403",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
            @ApiResponse(
                    responseCode = "404",
                    content = @Content(
                            mediaType = "application/json",
                            schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
    })
    public ResponseEntity<byte[]> disable(
            @AuthenticationPrincipal(expression = "subject") String actorEmail,
            @PathVariable String triggerId,
            @RequestHeader HttpHeaders browserHeaders) {
        return toResponseEntity(
                workflowRuntimeService.disableHTTPTrigger(
                        actorEmail, triggerId, browserHeaders));
    }

    private ResponseEntity<byte[]> toResponseEntity(
            WorkflowRuntimeResponse response) {
        return ResponseEntity
                .status(response.status())
                .headers(response.headers())
                .body(response.body());
    }
}
