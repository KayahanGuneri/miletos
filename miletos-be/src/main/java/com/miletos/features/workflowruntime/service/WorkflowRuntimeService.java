package com.miletos.features.workflowruntime.service;

import org.springframework.http.HttpHeaders;
import org.springframework.stereotype.Service;
import org.springframework.util.MultiValueMap;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflowruntime.client.GoRuntimeClient;
import com.miletos.features.workflowruntime.client.GoRuntimeResponse;
import com.miletos.features.workflowruntime.exception.CompanyContextRequiredException;
import com.miletos.security.AuthenticatedActorResolver;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class WorkflowRuntimeService {

        private final AuthenticatedActorResolver authenticatedActorResolver;
        private final GoRuntimeClient goRuntimeClient;

        public GoRuntimeResponse getPlugins(
                        String actorEmail,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getPlugins(
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse executeSync(
                        String actorEmail,
                        JsonNode body,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.executeSync(
                                body,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse executeAsync(
                        String actorEmail,
                        JsonNode body,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.executeAsync(
                                body,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse recoverExecution(
                        String actorEmail,
                        String executionId,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.recoverExecution(
                                executionId,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse listExecutions(
                        String actorEmail,
                        MultiValueMap<String, String> query,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.listExecutions(
                                query,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecution(
                        String actorEmail,
                        String executionId,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecution(
                                executionId,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecutionDefinition(
                        String actorEmail,
                        String executionId,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecutionDefinition(
                                executionId,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecutionNodes(
                        String actorEmail,
                        String executionId,
                        MultiValueMap<String, String> query,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecutionNodes(
                                executionId,
                                query,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecutionEvents(
                        String actorEmail,
                        String executionId,
                        MultiValueMap<String, String> query,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecutionEvents(
                                executionId,
                                query,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecutionLogs(
                        String actorEmail,
                        String executionId,
                        MultiValueMap<String, String> query,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecutionLogs(
                                executionId,
                                query,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecutionErrors(
                        String actorEmail,
                        String executionId,
                        MultiValueMap<String, String> query,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecutionErrors(
                                executionId,
                                query,
                                resolveCompanyId(actorEmail),
                                browserHeaders);
        }

        private String resolveCompanyId(String actorEmail) {
                User authenticatedUser = authenticatedActorResolver.resolve(actorEmail);

                if (authenticatedUser.getCompany() == null
                                || authenticatedUser.getCompany().getId() == null) {
                        throw new CompanyContextRequiredException();
                }

                return authenticatedUser.getCompany().getId().toString();
        }
}
