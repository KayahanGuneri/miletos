package com.miletos.features.workflowruntime.controller;

import org.springframework.http.HttpHeaders;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.util.MultiValueMap;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.workflowruntime.client.GoRuntimeResponse;
import com.miletos.features.workflowruntime.documentation.WorkflowRuntimeSchemas;
import com.miletos.features.workflowruntime.service.WorkflowRuntimeService;
import com.miletos.security.authorization.Authorize;

import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.enums.ParameterIn;
import io.swagger.v3.oas.annotations.media.Content;
import io.swagger.v3.oas.annotations.media.Schema;
import io.swagger.v3.oas.annotations.responses.ApiResponse;
import io.swagger.v3.oas.annotations.responses.ApiResponses;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/v1")
@RequiredArgsConstructor
public class WorkflowRuntimeController {

        private final WorkflowRuntimeService workflowRuntimeService;

        @Authorize
        @GetMapping("/plugins")
        @Operation(summary = "List workflow runtime node plugins")
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.PluginPage.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> getPlugins(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.getPlugins(
                                                actorEmail,
                                                browserHeaders));
        }

        @Authorize
        @PostMapping("/executions/sync")
        @Operation(summary = "Execute a workflow synchronously")
        @io.swagger.v3.oas.annotations.parameters.RequestBody(required = true, content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionRequest.class)))
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionResponse.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "422", description = "WORKFLOW_VALIDATION_FAILED", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> executeSync(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @RequestBody JsonNode body,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.executeSync(
                                                actorEmail,
                                                body,
                                                browserHeaders));
        }

        @Authorize
        @PostMapping("/executions/async")
        @Operation(summary = "Accept a workflow for asynchronous execution", parameters = {
                        @Parameter(name = "Idempotency-Key", description = "Company-scoped idempotency key used to safely "
                                        + "replay the same asynchronous execution request "
                                        + "without creating a duplicate execution.", required = true, in = ParameterIn.HEADER, example = "manual-async-20260729-001", schema = @Schema(type = "string"))
        })
        @io.swagger.v3.oas.annotations.parameters.RequestBody(required = true, content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionRequest.class)))
        @ApiResponses({
                        @ApiResponse(responseCode = "202", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionResponse.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "409", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "422", description = "WORKFLOW_VALIDATION_FAILED", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> executeAsync(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @RequestBody JsonNode body,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.executeAsync(
                                                actorEmail,
                                                body,
                                                browserHeaders));
        }

        @Authorize
        @PostMapping("/executions/{executionId}/recover")
        @Operation(summary = "Create or resume a partial recovery execution", parameters = {
                        @Parameter(name = "Idempotency-Key", description = "Company-scoped idempotency key used to safely "
                                        + "replay the same partial recovery request without "
                                        + "creating a duplicate recovery execution.", required = true, in = ParameterIn.HEADER, example = "manual-recovery-20260729-001", schema = @Schema(type = "string"))
        })
        @ApiResponses({
                        @ApiResponse(responseCode = "202", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.RecoveryResponse.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "404", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "409", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> recoverExecution(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @PathVariable String executionId,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.recoverExecution(
                                                actorEmail,
                                                executionId,
                                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions")
        @Operation(summary = "List workflow executions", parameters = {
                        @Parameter(name = "limit", description = "Maximum number of execution records to return.", required = false, in = ParameterIn.QUERY, example = "50", schema = @Schema(type = "integer", format = "int32", minimum = "1", defaultValue = "50")),
                        @Parameter(name = "after", description = "Opaque pagination cursor returned by the previous page.", required = false, in = ParameterIn.QUERY, schema = @Schema(type = "string")),
                        @Parameter(name = "workflowId", description = "Filters executions by workflow identifier.", required = false, in = ParameterIn.QUERY, example = "manual-workflow-20260729-001", schema = @Schema(type = "string")),
                        @Parameter(name = "status", description = "Filters executions by workflow execution status.", required = false, in = ParameterIn.QUERY, example = "SUCCEEDED", schema = @Schema(type = "string"))
        })
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionPage.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> listExecutions(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @Parameter(hidden = true) @RequestParam MultiValueMap<String, String> query,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.listExecutions(
                                                actorEmail,
                                                query,
                                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}")
        @Operation(summary = "Get a workflow execution")
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionSummaryResponse.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "404", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> getExecution(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @PathVariable String executionId,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.getExecution(
                                                actorEmail,
                                                executionId,
                                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/definition")
        @Operation(summary = "Get the immutable workflow definition for an execution")
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.DefinitionResponse.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "404", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> getExecutionDefinition(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @PathVariable String executionId,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.getExecutionDefinition(
                                                actorEmail,
                                                executionId,
                                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/nodes")
        @Operation(summary = "List node executions", parameters = {
                        @Parameter(name = "limit", description = "Maximum number of node executions to return.", required = false, in = ParameterIn.QUERY, example = "50", schema = @Schema(type = "integer", format = "int32", minimum = "1", defaultValue = "50")),
                        @Parameter(name = "after", description = "Opaque pagination cursor returned by the previous page.", required = false, in = ParameterIn.QUERY, schema = @Schema(type = "string"))
        })
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.NodeExecutionPage.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "404", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> getExecutionNodes(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @PathVariable String executionId,

                        @Parameter(hidden = true) @RequestParam MultiValueMap<String, String> query,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.getExecutionNodes(
                                                actorEmail,
                                                executionId,
                                                query,
                                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/events")
        @Operation(summary = "List execution events", parameters = {
                        @Parameter(name = "limit", description = "Maximum number of execution events to return.", required = false, in = ParameterIn.QUERY, example = "50", schema = @Schema(type = "integer", format = "int32", minimum = "1", defaultValue = "50")),
                        @Parameter(name = "after", description = "Opaque pagination cursor returned by the previous page.", required = false, in = ParameterIn.QUERY, schema = @Schema(type = "string"))
        })
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionEventPage.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "404", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> getExecutionEvents(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @PathVariable String executionId,

                        @Parameter(hidden = true) @RequestParam MultiValueMap<String, String> query,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.getExecutionEvents(
                                                actorEmail,
                                                executionId,
                                                query,
                                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/logs")
        @Operation(summary = "List execution logs", parameters = {
                        @Parameter(name = "limit", description = "Maximum number of execution log records to return.", required = false, in = ParameterIn.QUERY, example = "50", schema = @Schema(type = "integer", format = "int32", minimum = "1", defaultValue = "50")),
                        @Parameter(name = "after", description = "Opaque pagination cursor returned by the previous page.", required = false, in = ParameterIn.QUERY, schema = @Schema(type = "string"))
        })
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionLogPage.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "404", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> getExecutionLogs(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @PathVariable String executionId,

                        @Parameter(hidden = true) @RequestParam MultiValueMap<String, String> query,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.getExecutionLogs(
                                                actorEmail,
                                                executionId,
                                                query,
                                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/errors")
        @Operation(summary = "List structured execution errors", parameters = {
                        @Parameter(name = "limit", description = "Maximum number of structured execution errors to return.", required = false, in = ParameterIn.QUERY, example = "50", schema = @Schema(type = "integer", format = "int32", minimum = "1", defaultValue = "50")),
                        @Parameter(name = "after", description = "Opaque pagination cursor returned by the previous page.", required = false, in = ParameterIn.QUERY, schema = @Schema(type = "string"))
        })
        @ApiResponses({
                        @ApiResponse(responseCode = "200", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ExecutionErrorPage.class))),
                        @ApiResponse(responseCode = "400", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "401", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "403", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "404", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "500", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class))),
                        @ApiResponse(responseCode = "503", content = @Content(schema = @Schema(implementation = WorkflowRuntimeSchemas.ApiError.class)))
        })
        public ResponseEntity<byte[]> getExecutionErrors(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,

                        @PathVariable String executionId,

                        @Parameter(hidden = true) @RequestParam MultiValueMap<String, String> query,

                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(
                                workflowRuntimeService.getExecutionErrors(
                                                actorEmail,
                                                executionId,
                                                query,
                                                browserHeaders));
        }

        private ResponseEntity<byte[]> toResponseEntity(
                        GoRuntimeResponse response) {
                return ResponseEntity
                                .status(response.status())
                                .headers(response.headers())
                                .body(response.body());
        }
}