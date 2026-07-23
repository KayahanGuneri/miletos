package com.miletos.features.user.controller;

import com.miletos.features.user.UserMapper;
import com.miletos.features.user.UserService;
import com.miletos.features.user.controller.request.UpdateProfilePhotoRequest;
import com.miletos.features.user.controller.response.UserProfileResponse;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.service.input.ProfilePhotoUploadInput;
import com.miletos.features.user.service.output.ProfilePhotoContent;
import com.miletos.security.authorization.Authorize;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.http.CacheControl;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/users")
@RequiredArgsConstructor
public class UserController {

    private final UserService userService;
    private final UserMapper userMapper;

    @Authorize
    @GetMapping("/me")
    public ResponseEntity<UserProfileResponse> getCurrentUser(
            @AuthenticationPrincipal(expression = "subject")
            String actorEmail
    ) {
        User user =
                userService.getCurrentUser(actorEmail);

        return ResponseEntity.ok(
                userMapper.toResponse(user)
        );
    }

    @Authorize
    @PutMapping("/me/profile-photo")
    public ResponseEntity<UserProfileResponse> updateOwnProfilePhoto(
            @Valid @RequestBody UpdateProfilePhotoRequest request,
            @AuthenticationPrincipal(expression = "subject")
            String actorEmail
    ) {
        ProfilePhotoUploadInput input =
                userMapper.toProfilePhotoUploadInput(
                        actorEmail,
                        request
                );

        User user =
                userService.updateOwnProfilePhoto(input);

        return ResponseEntity.ok(
                userMapper.toResponse(user)
        );
    }

    @Authorize
    @GetMapping("/me/profile-photo")
    public ResponseEntity<byte[]> getOwnProfilePhoto(
            @AuthenticationPrincipal(expression = "subject")
            String actorEmail
    ) {
        ProfilePhotoContent profilePhoto =
                userService.getOwnProfilePhoto(actorEmail);

        return ResponseEntity.ok()
                .contentType(
                        MediaType.parseMediaType(
                                profilePhoto.contentType()
                        )
                )
                .cacheControl(CacheControl.noStore())
                .header(HttpHeaders.PRAGMA, "no-cache")
                .body(profilePhoto.content());
    }

}
