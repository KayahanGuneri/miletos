package com.miletos.features.auth.command;

public record CompletePasswordCommand(
                String token,
                String password) {
}