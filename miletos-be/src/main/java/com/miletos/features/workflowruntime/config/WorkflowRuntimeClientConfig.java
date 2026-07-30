package com.miletos.features.workflowruntime.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;

@Configuration
public class WorkflowRuntimeClientConfig {

    @Bean(destroyMethod = "shutdown")
    ManagedChannel workflowRuntimeChannel(WorkflowRuntimeProperties properties) {
        return buildChannel(ManagedChannelBuilder.forAddress(
                properties.grpcHost(), properties.grpcPort()));
    }

    static ManagedChannel buildChannel(ManagedChannelBuilder<?> builder) {
        return builder
                .usePlaintext()
                .disableRetry()
                .build();
    }
}
