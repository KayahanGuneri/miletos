package com.miletos.features.workflowruntime.service;

public enum TrustedTriggerType {
    MANUAL_DIRECT,
    SYNCHRONOUS_CALLER,
    KAFKA_EVENT,
    MESSAGE_EVENT,
    SCHEDULED,
    CRON,
    BACKGROUND_SYSTEM,
    HTTP_WEBHOOK
}
