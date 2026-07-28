package com.miletos.features.workflowruntime.service;

import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

import org.junit.jupiter.api.Test;
import org.springframework.http.HttpHeaders;

import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflowruntime.client.GoRuntimeClient;
import com.miletos.features.workflowruntime.client.GoRuntimeResponse;
import com.miletos.features.workflowruntime.exception.CompanyContextRequiredException;
import com.miletos.security.AuthenticatedActorResolver;

class WorkflowRuntimeServiceTest {

    @Test
    void derivesCompanyIdFromAuthenticatedUser() {
        AuthenticatedActorResolver resolver =
                mock(AuthenticatedActorResolver.class);
        GoRuntimeClient client = mock(GoRuntimeClient.class);
        User user = mock(User.class);
        Company company = new Company(
                "Runtime Tenant",
                CompanyStatus.ACTIVE);
        company.setId(42L);
        HttpHeaders browserHeaders = new HttpHeaders();
        GoRuntimeResponse response = mock(GoRuntimeResponse.class);

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
        GoRuntimeClient client = mock(GoRuntimeClient.class);
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
}
