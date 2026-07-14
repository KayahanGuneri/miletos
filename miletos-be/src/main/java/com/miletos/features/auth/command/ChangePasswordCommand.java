package com.miletos.features.auth.command;

public record ChangePasswordCommand(
                String actorEmail,
                String currentPassword,
                String newPassword) {
}