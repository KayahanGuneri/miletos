package com.miletos.config;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "miletos.secrets")
public record SecretsProperties(String aesKey) {}
