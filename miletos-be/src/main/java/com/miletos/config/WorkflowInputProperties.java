package com.miletos.config;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "miletos.workflow")
public record WorkflowInputProperties(String inputDirectory) {}
