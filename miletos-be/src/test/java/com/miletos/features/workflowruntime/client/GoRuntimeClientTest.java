package com.miletos.features.workflowruntime.client;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.springframework.test.web.client.ExpectedCount.once;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.header;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.headerDoesNotExist;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.method;
import static org.springframework.test.web.client.match.MockRestRequestMatchers.requestTo;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withException;
import static org.springframework.test.web.client.response.MockRestResponseCreators.withStatus;

import java.io.IOException;
import java.net.SocketTimeoutException;
import java.net.URI;
import java.time.Duration;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.test.web.client.MockRestServiceServer;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.web.client.RestClient;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.miletos.features.workflowruntime.config.WorkflowRuntimeProperties;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeAuthenticationException;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeNotConfiguredException;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeTimeoutException;
import com.miletos.features.workflowruntime.exception.WorkflowRuntimeUnavailableException;

class GoRuntimeClientTest {

    private static final String INTERNAL_TOKEN =
            "test-runtime-internal-service-token-at-least-32-bytes";

    private final ObjectMapper objectMapper = new ObjectMapper();

    private MockRestServiceServer server;
    private GoRuntimeClient client;

    @BeforeEach
    void createClient() {
        WorkflowRuntimeProperties properties =
                new WorkflowRuntimeProperties(
                        URI.create("http://runtime.internal/"),
                        INTERNAL_TOKEN,
                        Duration.ofSeconds(3),
                        Duration.ofSeconds(35));
        RestClient.Builder builder = RestClient
                .builder()
                .baseUrl(properties.normalizedBaseUrl());

        server = MockRestServiceServer
                .bindTo(builder)
                .build();
        client = new GoRuntimeClient(builder.build(), properties);
    }

    @Test
    void sendsOnlyInternalAuthenticationAndDerivedCompanyContext()
            throws Exception {
        HttpHeaders browserHeaders = new HttpHeaders();
        browserHeaders.setBearerAuth("user-java-jwt");
        browserHeaders.set(GoRuntimeClient.COMPANY_HEADER, "attacker-company");
        browserHeaders.set(HttpHeaders.COOKIE, "session=browser-cookie");
        browserHeaders.set(HttpHeaders.ORIGIN, "https://browser.example");
        browserHeaders.set("X-Correlation-ID", "correlation-42");

        server.expect(once(), requestTo(
                        "http://runtime.internal/api/v1/executions/sync"))
                .andExpect(method(HttpMethod.POST))
                .andExpect(header(
                        HttpHeaders.AUTHORIZATION,
                        "Bearer " + INTERNAL_TOKEN))
                .andExpect(header(
                        GoRuntimeClient.COMPANY_HEADER,
                        "42"))
                .andExpect(header(
                        "X-Correlation-ID",
                        "correlation-42"))
                .andExpect(headerDoesNotExist(HttpHeaders.COOKIE))
                .andExpect(headerDoesNotExist(HttpHeaders.ORIGIN))
                .andRespond(withStatus(HttpStatus.OK)
                        .contentType(MediaType.APPLICATION_JSON)
                        .body("{\"status\":\"SUCCEEDED\"}"));

        GoRuntimeResponse response = client.executeSync(
                objectMapper.readTree("{\"definition\":{}}"),
                "42",
                browserHeaders);

        assertThat(response.status()).isEqualTo(HttpStatus.OK);
        server.verify();
    }

    @Test
    void preservesQueryStatusBodyAndSafeResponseHeaders() {
        LinkedMultiValueMap<String, String> query =
                new LinkedMultiValueMap<>();
        query.add("limit", "20");
        query.add("status", "FAILED");

        server.expect(once(), requestTo(
                        "http://runtime.internal/api/v1/executions?limit=20&status=FAILED"))
                .andExpect(method(HttpMethod.GET))
                .andRespond(withStatus(HttpStatus.PARTIAL_CONTENT)
                        .contentType(MediaType.APPLICATION_JSON)
                        .header(HttpHeaders.CACHE_CONTROL, "no-store")
                        .header(HttpHeaders.ETAG, "\"runtime-etag\"")
                        .header("X-Request-ID", "request-42")
                        .header(HttpHeaders.SET_COOKIE, "internal-cookie=secret")
                        .header("Server", "runtime-internal")
                        .body("{\"items\":[]}"));

        GoRuntimeResponse response = client.listExecutions(
                query,
                "42",
                new HttpHeaders());

        assertThat(response.status())
                .isEqualTo(HttpStatus.PARTIAL_CONTENT);
        assertThat(response.body())
                .isEqualTo("{\"items\":[]}".getBytes());
        assertThat(response.headers().getContentType())
                .isEqualTo(MediaType.APPLICATION_JSON);
        assertThat(response.headers().getCacheControl())
                .isEqualTo("no-store");
        assertThat(response.headers().getETag())
                .isEqualTo("\"runtime-etag\"");
        assertThat(response.headers().getFirst("X-Request-ID"))
                .isEqualTo("request-42");
        assertThat(response.headers()).doesNotContainKey(
                HttpHeaders.SET_COOKIE);
        assertThat(response.headers()).doesNotContainKey("Server");
        server.verify();
    }

    @Test
    void safelyEncodesExecutionIdAsOnePathSegment() {
        server.expect(once(), requestTo(
                        "http://runtime.internal/api/v1/executions/execution%2F..%2Finternal/logs"))
                .andRespond(withStatus(HttpStatus.OK)
                        .contentType(MediaType.APPLICATION_JSON)
                        .body("{\"items\":[]}"));

        client.getExecutionLogs(
                "execution/../internal",
                new LinkedMultiValueMap<>(),
                "42",
                new HttpHeaders());

        server.verify();
    }

    @Test
    void mapsGoInternalUnauthorizedToBridgeFailure() {
        server.expect(once(), requestTo(
                        "http://runtime.internal/api/v1/plugins"))
                .andRespond(withStatus(HttpStatus.UNAUTHORIZED)
                        .contentType(MediaType.APPLICATION_JSON)
                        .body("{\"code\":\"UNAUTHORIZED\"}"));

        assertThatThrownBy(() -> client.getPlugins(
                "42",
                new HttpHeaders()))
                .isInstanceOf(WorkflowRuntimeAuthenticationException.class)
                .extracting("httpStatus")
                .isEqualTo(HttpStatus.BAD_GATEWAY);

        server.verify();
    }

    @Test
    void preservesNormalGoApplicationErrorContract() {
        server.expect(once(), requestTo(
                        "http://runtime.internal/api/v1/executions/missing"))
                .andRespond(withStatus(HttpStatus.NOT_FOUND)
                        .contentType(MediaType.APPLICATION_JSON)
                        .body("{\"code\":\"NOT_FOUND\"}"));

        GoRuntimeResponse response = client.getExecution(
                "missing",
                "42",
                new HttpHeaders());

        assertThat(response.status()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(response.body())
                .isEqualTo("{\"code\":\"NOT_FOUND\"}".getBytes());
        server.verify();
    }

    @Test
    void mapsNetworkFailureToBadGateway() {
        server.expect(once(), requestTo(
                        "http://runtime.internal/api/v1/plugins"))
                .andRespond(withException(new IOException(
                        "simulated connection failure")));

        assertThatThrownBy(() -> client.getPlugins(
                "42",
                new HttpHeaders()))
                .isInstanceOf(WorkflowRuntimeUnavailableException.class)
                .extracting("httpStatus")
                .isEqualTo(HttpStatus.BAD_GATEWAY);

        server.verify();
    }

    @Test
    void mapsReadTimeoutToGatewayTimeout() {
        server.expect(once(), requestTo(
                        "http://runtime.internal/api/v1/plugins"))
                .andRespond(withException(new SocketTimeoutException(
                        "simulated read timeout")));

        assertThatThrownBy(() -> client.getPlugins(
                "42",
                new HttpHeaders()))
                .isInstanceOf(WorkflowRuntimeTimeoutException.class)
                .extracting("httpStatus")
                .isEqualTo(HttpStatus.GATEWAY_TIMEOUT);

        server.verify();
    }

    @Test
    void mapsMissingConfigurationToControlledServiceUnavailable() {
        WorkflowRuntimeProperties properties =
                new WorkflowRuntimeProperties(
                        URI.create("http://runtime.internal/"),
                        "",
                        Duration.ofSeconds(3),
                        Duration.ofSeconds(35));
        GoRuntimeClient unconfiguredClient =
                new GoRuntimeClient(RestClient.create(), properties);

        assertThatThrownBy(() -> unconfiguredClient.getPlugins(
                "42",
                new HttpHeaders()))
                .isInstanceOf(WorkflowRuntimeNotConfiguredException.class)
                .extracting("httpStatus")
                .isEqualTo(HttpStatus.SERVICE_UNAVAILABLE);
    }
}
