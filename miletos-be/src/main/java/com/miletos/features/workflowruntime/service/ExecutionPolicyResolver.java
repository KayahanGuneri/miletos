package com.miletos.features.workflowruntime.service;

import org.springframework.stereotype.Component;

@Component
public class ExecutionPolicyResolver {

    public ResolvedExecutionMode resolve(
            ExecutionModePolicy policy,
            TrustedTriggerType triggerType) {
        if (policy == ExecutionModePolicy.SYNC) {
            return ResolvedExecutionMode.SYNC;
        }
        if (policy == ExecutionModePolicy.ASYNC) {
            return ResolvedExecutionMode.ASYNC;
        }
        return switch (triggerType) {
            case MANUAL_DIRECT, SYNCHRONOUS_CALLER -> ResolvedExecutionMode.SYNC;
            case KAFKA_EVENT, MESSAGE_EVENT, SCHEDULED, CRON,
                    BACKGROUND_SYSTEM, HTTP_WEBHOOK -> ResolvedExecutionMode.ASYNC;
        };
    }
}
