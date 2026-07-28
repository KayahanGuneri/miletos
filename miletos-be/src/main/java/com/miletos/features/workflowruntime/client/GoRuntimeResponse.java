package com.miletos.features.workflowruntime.client;

import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatusCode;

public record GoRuntimeResponse(
        HttpStatusCode status,
        HttpHeaders headers,
        byte[] body) {

    public GoRuntimeResponse {
        HttpHeaders copiedHeaders = new HttpHeaders();
        copiedHeaders.putAll(headers);
        headers = HttpHeaders.readOnlyHttpHeaders(copiedHeaders);
        body = body == null ? new byte[0] : body.clone();
    }

    @Override
    public byte[] body() {
        return body.clone();
    }
}
