package com.miletos.features.workflowruntime.service;

import java.util.List;

import org.springframework.http.HttpHeaders;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.util.MultiValueMap;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.company.exception.CompanyNotFoundException;
import com.miletos.features.company.exception.CompanyOperationForbiddenException;
import com.miletos.features.company.repository.CompanyRepository;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflowruntime.client.GoRuntimeClient;
import com.miletos.features.workflowruntime.client.GoRuntimeResponse;
import com.miletos.features.workflowruntime.exception.CompanyContextRequiredException;
import com.miletos.security.AuthenticatedActorResolver;

@Service
public class WorkflowRuntimeService {

        private static final String COMPANY_HEADER = "X-Miletos-Company-ID";

        private final AuthenticatedActorResolver authenticatedActorResolver;
        private final CompanyRepository companyRepository;
        private final GoRuntimeClient goRuntimeClient;

        @Autowired
        public WorkflowRuntimeService(
                        AuthenticatedActorResolver authenticatedActorResolver,
                        CompanyRepository companyRepository,
                        GoRuntimeClient goRuntimeClient) {
                this.authenticatedActorResolver = authenticatedActorResolver;
                this.companyRepository = companyRepository;
                this.goRuntimeClient = goRuntimeClient;
        }

        public WorkflowRuntimeService(
                        AuthenticatedActorResolver authenticatedActorResolver,
                        GoRuntimeClient goRuntimeClient) {
                this(authenticatedActorResolver, null, goRuntimeClient);
        }

        public GoRuntimeResponse getPlugins(
                        String actorEmail,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getPlugins(
                                resolveCompanyId(actorEmail, browserHeaders),
                                browserHeaders);
        }

        public GoRuntimeResponse executeSync(
                        String actorEmail,
                        JsonNode body,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.executeSync(
                                body,
                                resolveCompanyId(actorEmail, browserHeaders),
                                browserHeaders);
        }

        public GoRuntimeResponse executeAsync(
                        String actorEmail,
                        JsonNode body,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.executeAsync(
                                body,
                                resolveCompanyId(actorEmail, browserHeaders),
                                browserHeaders);
        }

        public GoRuntimeResponse recoverExecution(
                        String actorEmail,
                        String executionId,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.recoverExecution(
                                executionId,
                                resolveCompanyId(actorEmail, browserHeaders),
                                browserHeaders);
        }

        public GoRuntimeResponse listExecutions(
                        String actorEmail,
                        MultiValueMap<String, String> query,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.listExecutions(
                                query,
                                resolveCompanyId(actorEmail, browserHeaders),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecution(
                        String actorEmail,
                        String executionId,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecution(
                                executionId,
                                resolveCompanyId(actorEmail, browserHeaders),
                                browserHeaders);
        }

        public GoRuntimeResponse getExecutionDefinition(
                        String actorEmail,
                        String executionId,
                        HttpHeaders browserHeaders) {
                return goRuntimeClient.getExecutionDefinition(
                                executionId,
                                resolveCompanyId(actorEmail, browserHeaders),
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
                                resolveCompanyId(actorEmail, browserHeaders),
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
                                resolveCompanyId(actorEmail, browserHeaders),
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
                                resolveCompanyId(actorEmail, browserHeaders),
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
                                resolveCompanyId(actorEmail, browserHeaders),
                                browserHeaders);
        }

        private String resolveCompanyId(
                        String actorEmail,
                        HttpHeaders browserHeaders) {
                User authenticatedUser = authenticatedActorResolver.resolve(actorEmail);
                List<String> headerValues = browserHeaders.get(COMPANY_HEADER);
                List<String> requestedCompanyIds = (headerValues == null
                                ? List.<String>of()
                                : headerValues)
                                .stream()
                                .map(String::trim)
                                .filter(value -> !value.isBlank())
                                .toList();

                if (!authenticatedUser.isSuperAdmin()) {
                        if (authenticatedUser.getCompany() == null
                                        || authenticatedUser.getCompany().getId() == null) {
                                throw new CompanyContextRequiredException();
                        }

                        String authenticatedCompanyId =
                                        authenticatedUser.getCompany().getId().toString();
                        for (String requestedCompanyId : requestedCompanyIds) {
                                if (!authenticatedCompanyId.equals(requestedCompanyId)) {
                                        throw new CompanyOperationForbiddenException();
                                }
                        }

                        return authenticatedCompanyId;
                }

                if (requestedCompanyIds.isEmpty()) {
                        throw new CompanyContextRequiredException();
                }
                if (requestedCompanyIds.size() != 1) {
                        throw new CompanyOperationForbiddenException();
                }
                String requestedCompanyId = requestedCompanyIds.getFirst();

                Long companyId;
                try {
                        companyId = Long.valueOf(requestedCompanyId);
                } catch (NumberFormatException exception) {
                        throw new CompanyNotFoundException();
                }

                if (companyId <= 0
                                || companyRepository == null
                                || !companyRepository.existsById(companyId)) {
                        throw new CompanyNotFoundException();
                }

                return companyId.toString();
        }
}
