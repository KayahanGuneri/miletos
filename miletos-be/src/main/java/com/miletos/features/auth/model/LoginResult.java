package com.miletos.features.auth.model;

import com.miletos.features.user.repository.entity.User;

public record LoginResult(
                User user,
                JwtToken accessToken) {
}