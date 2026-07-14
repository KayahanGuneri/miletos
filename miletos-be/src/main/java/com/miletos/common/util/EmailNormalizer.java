package com.miletos.common.util;

import java.util.Locale;

public final class EmailNormalizer {

    private EmailNormalizer() {
    }

    public static String normalize(String email) {
        if (email == null) {
            throw new IllegalArgumentException("Email must not be null");
        }

        String normalizedEmail = email.trim().toLowerCase(Locale.ROOT);

        if (normalizedEmail.isBlank()) {
            throw new IllegalArgumentException("Email must not be blank");
        }

        return normalizedEmail;
    }
}