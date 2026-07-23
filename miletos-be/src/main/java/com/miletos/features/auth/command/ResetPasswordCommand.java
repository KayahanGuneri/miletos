package com.miletos.features.auth.command;

public record ResetPasswordCommand(
                String token,
                String password) {
}