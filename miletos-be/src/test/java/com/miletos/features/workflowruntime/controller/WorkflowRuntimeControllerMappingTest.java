package com.miletos.features.workflowruntime.controller;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.isNull;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.lang.reflect.Method;
import java.util.Arrays;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import org.junit.jupiter.api.Test;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;

import com.miletos.common.exception.GlobalExceptionHandler;
import com.miletos.common.response.ApiErrorResponseFactory;
import com.miletos.features.workflowruntime.exception.CompanyContextRequiredException;
import com.miletos.features.workflowruntime.service.WorkflowRuntimeService;
import com.miletos.security.authorization.Authorize;

class WorkflowRuntimeControllerMappingTest {

    @Test
    void exposesOnlyTheExplicitSupportedRuntimeRoutes() {
        Set<String> expected = Set.of(
                "GET /plugins",
                "POST /executions/sync",
                "POST /executions/async",
                "POST /executions/{executionId}/recover",
                "GET /executions",
                "GET /executions/{executionId}",
                "GET /executions/{executionId}/definition",
                "GET /executions/{executionId}/nodes",
                "GET /executions/{executionId}/events",
                "GET /executions/{executionId}/logs",
                "GET /executions/{executionId}/errors");

        Set<String> actual = Arrays
                .stream(WorkflowRuntimeController.class.getDeclaredMethods())
                .flatMap(this::route)
                .collect(Collectors.toSet());

        assertThat(actual).isEqualTo(expected);
    }

    @Test
    void everyRuntimeEndpointUsesExistingAuthorizationAndPrincipalPattern() {
        Method[] endpointMethods = Arrays
                .stream(WorkflowRuntimeController.class.getDeclaredMethods())
                .filter(method -> route(method).findAny().isPresent())
                .toArray(Method[]::new);

        assertThat(endpointMethods).allSatisfy(method -> {
            assertThat(method.isAnnotationPresent(Authorize.class)).isTrue();
            assertThat(Arrays.stream(method.getParameters())
                    .map(parameter -> parameter.getAnnotation(
                            AuthenticationPrincipal.class))
                    .anyMatch(annotation ->
                            annotation != null
                                    && "subject".equals(
                                            annotation.expression())))
                    .isTrue();
        });
    }

    @Test
    void arbitraryRuntimeSubpathsCannotBeProxied() throws Exception {
        WorkflowRuntimeService service =
                mock(WorkflowRuntimeService.class);
        MockMvc mvc = MockMvcBuilders
                .standaloneSetup(new WorkflowRuntimeController(service))
                .build();

        mvc.perform(get("/api/v1/executions/42/arbitrary"))
                .andExpect(status().isNotFound());

        verifyNoInteractions(service);
    }

    @Test
    void companyContextFailureUsesStableForbiddenApiError() throws Exception {
        WorkflowRuntimeService service =
                mock(WorkflowRuntimeService.class);
        org.mockito.Mockito
                .when(service.listExecutions(
                        isNull(),
                        any(),
                        any()))
                .thenThrow(new CompanyContextRequiredException());
        Clock clock = Clock.fixed(
                Instant.parse("2026-07-27T12:00:00Z"),
                ZoneOffset.UTC);
        MockMvc mvc = MockMvcBuilders
                .standaloneSetup(new WorkflowRuntimeController(service))
                .setControllerAdvice(new GlobalExceptionHandler(
                        new ApiErrorResponseFactory(clock)))
                .build();

        mvc.perform(get("/api/v1/executions"))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code")
                        .value("COMPANY_CONTEXT_REQUIRED"))
                .andExpect(jsonPath("$.path")
                        .value("/api/v1/executions"));
    }

    private Stream<String> route(Method method) {
        GetMapping getMapping = method.getAnnotation(GetMapping.class);
        PostMapping postMapping = method.getAnnotation(PostMapping.class);

        if (getMapping != null) {
            return Arrays.stream(getMapping.value())
                    .map(path -> "GET " + path);
        }

        if (postMapping != null) {
            return Arrays.stream(postMapping.value())
                    .map(path -> "POST " + path);
        }

        return Stream.empty();
    }
}
