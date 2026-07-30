package com.miletos.features.workflowruntime.service;

import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

import org.junit.jupiter.api.Test;
import org.springframework.http.HttpHeaders;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeGrpcClient;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeResponse;
import com.miletos.features.workflowruntime.exception.CompanyContextRequiredException;
import com.miletos.security.AuthenticatedActorResolver;

class WorkflowRuntimeServiceTest {

    @Test
    void derivesCompanyIdFromAuthenticatedUser() {
        AuthenticatedActorResolver resolver =
                mock(AuthenticatedActorResolver.class);
        WorkflowRuntimeGrpcClient client = mock(WorkflowRuntimeGrpcClient.class);
        User user = mock(User.class);
        Company company = new Company(
                "Runtime Tenant",
                CompanyStatus.ACTIVE);
        company.setId(42L);
        HttpHeaders browserHeaders = new HttpHeaders();
        WorkflowRuntimeResponse response = mock(WorkflowRuntimeResponse.class);

        when(resolver.resolve("actor@miletos.local")).thenReturn(user);
        when(user.getCompany()).thenReturn(company);
        when(client.getPlugins("42", browserHeaders))
                .thenReturn(response);

        WorkflowRuntimeService service =
                new WorkflowRuntimeService(resolver, client);

        service.getPlugins("actor@miletos.local", browserHeaders);

        verify(resolver).resolve("actor@miletos.local");
        verify(client).getPlugins("42", browserHeaders);
    }

    @Test
    void rejectsAuthenticatedUserWithoutCompany() {
        AuthenticatedActorResolver resolver =
                mock(AuthenticatedActorResolver.class);
        WorkflowRuntimeGrpcClient client = mock(WorkflowRuntimeGrpcClient.class);
        User user = mock(User.class);

        when(resolver.resolve("actor@miletos.local")).thenReturn(user);
        when(user.getCompany()).thenReturn(null);

        WorkflowRuntimeService service =
                new WorkflowRuntimeService(resolver, client);

        assertThatThrownBy(() -> service.getPlugins(
                "actor@miletos.local",
                new HttpHeaders()))
                .isInstanceOf(CompanyContextRequiredException.class);
        verifyNoInteractions(client);
    }

    @Test
    void browserBodyCannotSpoofTrustedTriggerAndAutoReplayStaysSync()
            throws Exception {
        AuthenticatedActorResolver resolver =
                mock(AuthenticatedActorResolver.class);
        WorkflowRuntimeGrpcClient client =
                mock(WorkflowRuntimeGrpcClient.class);
        User user = mock(User.class);
        Company company = new Company(
                "Runtime Tenant",
                CompanyStatus.ACTIVE);
        company.setId(42L);
        HttpHeaders browserHeaders = new HttpHeaders();
        var body = new ObjectMapper().readTree(
                """
                {
                  "triggerType": "HTTP_WEBHOOK",
                  "mode": "ASYNC",
                  "definition": {}
                }
                """);
        WorkflowRuntimeResponse response =
                mock(WorkflowRuntimeResponse.class);

        when(resolver.resolve("actor@miletos.local")).thenReturn(user);
        when(user.getCompany()).thenReturn(company);
        when(client.executeSync(body, "42", browserHeaders))
                .thenReturn(response);
        WorkflowRuntimeService service =
                new WorkflowRuntimeService(resolver, client);

        service.execute("actor@miletos.local", body, browserHeaders);
        service.execute("actor@miletos.local", body, browserHeaders);

        verify(client, times(2)).executeSync(body, "42", browserHeaders);
        verify(client, times(0)).executeAsync(body, "42", browserHeaders);
    }
}
