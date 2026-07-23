package com.miletos.config;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "miletos.mail")
public record MailProperties(
                String from) {
}
