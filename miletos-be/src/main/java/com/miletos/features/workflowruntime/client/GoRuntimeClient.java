package com.miletos.features.workflowruntime.client;

import java.net.SocketTimeoutException;
import java.net.http.HttpTimeoutException;
import java.util.List;

import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.util.MultiValueMap;
import org.springframework.util.StreamUtils;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.client.RestClient;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.workflowruntime.config.WorkflowRuntimeProperties;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeAuthenticationException;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeNotConfiguredException;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeTimeoutException;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeUnavailableException;

@Component
public class GoRuntimeClient {

    static final String COMPANY_HEADER = "X-Miletos-Company-ID";

    private static final List<String> FORWARDED_REQUEST_HEADERS = List.of(
            HttpHeaders.ACCEPT,
            "Idempotency-Key",
            "X-Correlation-ID",
            "X-Request-ID");

    private static final List<String> FORWARDED_RESPONSE_HEADERS = List.of(
            HttpHeaders.CACHE_CONTROL,
            HttpHeaders.CONTENT_TYPE,
            HttpHeaders.ETAG,
            HttpHeaders.LOCATION,
            HttpHeaders.RETRY_AFTER,
            "X-Correlation-ID",
            "X-Request-ID");

    private final RestClient restClient;
    private final String internalServiceToken;
    private final boolean configured;

    public GoRuntimeClient(
            RestClient workflowRuntimeRestClient,
            WorkflowRuntimeProperties properties) {
        this.restClient = workflowRuntimeRestClient;
        this.internalServiceToken = properties.internalServiceToken();
        this.configured = properties.isConfigured();
    }

    public GoRuntimeResponse getPlugins(
            String companyId,
            HttpHeaders browserHeaders) {
        return exchange(
                HttpMethod.GET,
                List.of("api", "v1", "plugins"),
                new LinkedMultiValueMap<>(),
                null,
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse executeSync(
            JsonNode body,
            String companyId,
            HttpHeaders browserHeaders) {
        return exchange(
                HttpMethod.POST,
                List.of("api", "v1", "executions", "sync"),
                new LinkedMultiValueMap<>(),
                body,
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse executeAsync(
            JsonNode body,
            String companyId,
            HttpHeaders browserHeaders) {
        return exchange(
                HttpMethod.POST,
                List.of("api", "v1", "executions", "async"),
                new LinkedMultiValueMap<>(),
                body,
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse recoverExecution(
            String executionId,
            String companyId,
            HttpHeaders browserHeaders) {
        return exchange(
                HttpMethod.POST,
                List.of("api", "v1", "executions", executionId, "recover"),
                new LinkedMultiValueMap<>(),
                null,
                companyId,
                browserHeaders,
                true);
    }

    public GoRuntimeResponse listExecutions(
            MultiValueMap<String, String> query,
            String companyId,
            HttpHeaders browserHeaders) {
        return getExecutionResource(
                List.of("api", "v1", "executions"),
                query,
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse getExecution(
            String executionId,
            String companyId,
            HttpHeaders browserHeaders) {
        return getExecutionResource(
                List.of("api", "v1", "executions", executionId),
                new LinkedMultiValueMap<>(),
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse getExecutionDefinition(
            String executionId,
            String companyId,
            HttpHeaders browserHeaders) {
        return getExecutionResource(
                List.of("api", "v1", "executions", executionId, "definition"),
                new LinkedMultiValueMap<>(),
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse getExecutionNodes(
            String executionId,
            MultiValueMap<String, String> query,
            String companyId,
            HttpHeaders browserHeaders) {
        return getExecutionResource(
                List.of("api", "v1", "executions", executionId, "nodes"),
                query,
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse getExecutionEvents(
            String executionId,
            MultiValueMap<String, String> query,
            String companyId,
            HttpHeaders browserHeaders) {
        return getExecutionResource(
                List.of("api", "v1", "executions", executionId, "events"),
                query,
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse getExecutionLogs(
            String executionId,
            MultiValueMap<String, String> query,
            String companyId,
            HttpHeaders browserHeaders) {
        return getExecutionResource(
                List.of("api", "v1", "executions", executionId, "logs"),
                query,
                companyId,
                browserHeaders);
    }

    public GoRuntimeResponse getExecutionErrors(
            String executionId,
            MultiValueMap<String, String> query,
            String companyId,
            HttpHeaders browserHeaders) {
        return getExecutionResource(
                List.of("api", "v1", "executions", executionId, "errors"),
                query,
                companyId,
                browserHeaders);
    }

    private GoRuntimeResponse getExecutionResource(
            List<String> pathSegments,
            MultiValueMap<String, String> query,
            String companyId,
            HttpHeaders browserHeaders) {
        return exchange(
                HttpMethod.GET,
                pathSegments,
                query,
                null,
                companyId,
                browserHeaders);
    }

    private GoRuntimeResponse exchange(
            HttpMethod method,
            List<String> pathSegments,
            MultiValueMap<String, String> query,
            JsonNode body,
            String companyId,
            HttpHeaders browserHeaders) {
        return exchange(
                method,
                pathSegments,
                query,
                body,
                companyId,
                browserHeaders,
                false);
    }

    private GoRuntimeResponse exchange(
            HttpMethod method,
            List<String> pathSegments,
            MultiValueMap<String, String> query,
            JsonNode body,
            String companyId,
            HttpHeaders browserHeaders,
            boolean bodylessPost) {
        if (!configured) {
            throw new WorkflowRuntimeNotConfiguredException();
        }

        try {
            RestClient.RequestBodySpec request = restClient
                    .method(method)
                    .uri(uriBuilder -> uriBuilder
                            .pathSegment(pathSegments.toArray(String[]::new))
                            .queryParams(query)
                            .build())
                    .headers(headers -> applyRequestHeaders(
                            headers,
                            browserHeaders,
                            companyId,
                            body != null,
                            bodylessPost));

            if (body != null) {
                request.body(body);
            }

            GoRuntimeResponse response = request.exchange(
                    (outgoingRequest, upstreamResponse) -> {
                        HttpHeaders responseHeaders = copyAllowedResponseHeaders(
                                upstreamResponse.getHeaders());
                        byte[] responseBody = StreamUtils.copyToByteArray(
                                upstreamResponse.getBody());

                        return new GoRuntimeResponse(
                                upstreamResponse.getStatusCode(),
                                responseHeaders,
                                responseBody);
                    });

            if (response.status().value() == HttpStatus.UNAUTHORIZED.value()) {
                throw new WorkflowRuntimeAuthenticationException();
            }

            return response;
        } catch (WorkflowRuntimeAuthenticationException exception) {
            throw exception;
        } catch (ResourceAccessException exception) {
            if (isTimeout(exception)) {
                throw new WorkflowRuntimeTimeoutException(exception);
            }

            throw new WorkflowRuntimeUnavailableException(exception);
        }
    }

    private void applyRequestHeaders(
            HttpHeaders outgoing,
            HttpHeaders browserHeaders,
            String companyId,
            boolean hasBody,
            boolean bodylessPost) {
        FORWARDED_REQUEST_HEADERS.forEach(headerName -> {
            List<String> values = browserHeaders.get(headerName);

            if (values != null && !values.isEmpty()) {
                outgoing.put(headerName, List.copyOf(values));
            }
        });

        if (!outgoing.containsKey(HttpHeaders.ACCEPT)) {
            outgoing.setAccept(List.of(MediaType.APPLICATION_JSON));
        }

        if (hasBody) {
            outgoing.setContentType(MediaType.APPLICATION_JSON);
        } else if (bodylessPost) {
            outgoing.setContentLength(0);
        }

        outgoing.setBearerAuth(internalServiceToken);
        outgoing.set(COMPANY_HEADER, companyId);
    }

    private HttpHeaders copyAllowedResponseHeaders(
            HttpHeaders upstreamHeaders) {
        HttpHeaders allowed = new HttpHeaders();

        FORWARDED_RESPONSE_HEADERS.forEach(headerName -> {
            List<String> values = upstreamHeaders.get(headerName);

            if (values != null && !values.isEmpty()) {
                allowed.put(headerName, List.copyOf(values));
            }
        });

        return allowed;
    }

    private boolean isTimeout(Throwable throwable) {
        Throwable current = throwable;

        while (current != null) {
            if (current instanceof SocketTimeoutException
                    || current instanceof HttpTimeoutException) {
                return true;
            }

            current = current.getCause();
        }

        return false;
    }
}
