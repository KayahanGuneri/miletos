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
import com.miletos.features.workflowruntime.service.WorkflowRuntimeService;
import com.miletos.security.authorization.Authorize;

import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/v1")
@RequiredArgsConstructor
public class WorkflowRuntimeController {

        private final WorkflowRuntimeService workflowRuntimeService;

        @Authorize
        @GetMapping("/plugins")
        public ResponseEntity<byte[]> getPlugins(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.getPlugins(
                                actorEmail,
                                browserHeaders));
        }

        @Authorize
        @PostMapping("/executions/sync")
        public ResponseEntity<byte[]> executeSync(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @RequestBody JsonNode body,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.executeSync(
                                actorEmail,
                                body,
                                browserHeaders));
        }

        @Authorize
        @PostMapping("/executions/async")
        public ResponseEntity<byte[]> executeAsync(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @RequestBody JsonNode body,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.executeAsync(
                                actorEmail,
                                body,
                                browserHeaders));
        }

        @Authorize
        @PostMapping("/executions/{executionId}/recover")
        public ResponseEntity<byte[]> recoverExecution(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @PathVariable String executionId,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.recoverExecution(
                                actorEmail,
                                executionId,
                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions")
        public ResponseEntity<byte[]> listExecutions(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @RequestParam MultiValueMap<String, String> query,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.listExecutions(
                                actorEmail,
                                query,
                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}")
        public ResponseEntity<byte[]> getExecution(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @PathVariable String executionId,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.getExecution(
                                actorEmail,
                                executionId,
                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/definition")
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
        public ResponseEntity<byte[]> getExecutionNodes(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @PathVariable String executionId,
                        @RequestParam MultiValueMap<String, String> query,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.getExecutionNodes(
                                actorEmail,
                                executionId,
                                query,
                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/events")
        public ResponseEntity<byte[]> getExecutionEvents(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @PathVariable String executionId,
                        @RequestParam MultiValueMap<String, String> query,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.getExecutionEvents(
                                actorEmail,
                                executionId,
                                query,
                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/logs")
        public ResponseEntity<byte[]> getExecutionLogs(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @PathVariable String executionId,
                        @RequestParam MultiValueMap<String, String> query,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.getExecutionLogs(
                                actorEmail,
                                executionId,
                                query,
                                browserHeaders));
        }

        @Authorize
        @GetMapping("/executions/{executionId}/errors")
        public ResponseEntity<byte[]> getExecutionErrors(
                        @AuthenticationPrincipal(expression = "subject") String actorEmail,
                        @PathVariable String executionId,
                        @RequestParam MultiValueMap<String, String> query,
                        @RequestHeader HttpHeaders browserHeaders) {
                return toResponseEntity(workflowRuntimeService.getExecutionErrors(
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
