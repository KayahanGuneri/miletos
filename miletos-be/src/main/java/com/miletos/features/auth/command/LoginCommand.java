package com.miletos.features.auth.command;

public record LoginCommand(
                String email,
                String password) {
}