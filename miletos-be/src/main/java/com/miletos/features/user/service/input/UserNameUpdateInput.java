package com.miletos.features.user.service.input;


public record UserNameUpdateInput(
        String actorEmail,
        Long targetUserId,
        String firstName,
        String lastName
) {
}