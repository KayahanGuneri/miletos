package com.miletos.config;

import org.springframework.boot.context.properties.ConfigurationProperties;

@ConfigurationProperties(prefix = "miletos.profile-photo")
public record ProfilePhotoProperties(
                String storageDirectory,
                long maxSizeBytes) {
}