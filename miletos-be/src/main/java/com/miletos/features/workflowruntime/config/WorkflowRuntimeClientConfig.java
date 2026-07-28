package com.miletos.features.workflowruntime.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.client.SimpleClientHttpRequestFactory;
import org.springframework.web.client.RestClient;

@Configuration
public class WorkflowRuntimeClientConfig {

        @Bean
        RestClient workflowRuntimeRestClient(
                        RestClient.Builder builder,
                        WorkflowRuntimeProperties properties) {
                SimpleClientHttpRequestFactory requestFactory = new SimpleClientHttpRequestFactory();
                requestFactory.setConnectTimeout(properties.connectTimeout());
                requestFactory.setReadTimeout(properties.readTimeout());

                return builder
                                .baseUrl(properties.normalizedBaseUrl())
                                .requestFactory(requestFactory)
                                .build();
        }
}
