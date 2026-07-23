package com.miletos.security.authorization;

import static org.assertj.core.api.Assertions.assertThat;

import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import org.junit.jupiter.api.Test;

class AuthorizationEvaluatorTest {

    private final AuthorizationEvaluator evaluator = new AuthorizationEvaluator();

    @Test
    void grantsSuperadminPermissionToSuperadmin() {
        User user = activeUser(null);
        user.setSuperAdmin(true);

        assertThat(evaluator.hasAnyRole(user, RequiredRole.SUPERADMIN)).isTrue();
        assertThat(evaluator.hasAnyRole(user, RequiredRole.ADMIN)).isTrue();
        assertThat(evaluator.hasAnyRole(user)).isTrue();
    }

    @Test
    void grantsCompanyAdminPermissionToAdminRole() {
        User user = activeUser(UserRole.ADMIN);

        assertThat(evaluator.hasAnyRole(user, RequiredRole.SUPERADMIN)).isFalse();
        assertThat(evaluator.hasAnyRole(user, RequiredRole.ADMIN)).isTrue();
        assertThat(evaluator.hasAnyRole(user)).isTrue();
    }

    @Test
    void deniesCompanyAdminPermissionToRegularUser() {
        User user = activeUser(UserRole.USER);

        assertThat(evaluator.hasAnyRole(user, RequiredRole.SUPERADMIN)).isFalse();
        assertThat(evaluator.hasAnyRole(user, RequiredRole.ADMIN)).isFalse();
        assertThat(evaluator.hasAnyRole(user, RequiredRole.USER)).isTrue();
        assertThat(evaluator.hasAnyRole(user)).isTrue();
    }

    @Test
    void deniesAllPermissionsToInactiveUser() {
        User user = new User(
                null,
                null,
                "disabled@miletos.local",
                "hash",
                "Disabled",
                "User",
                UserRole.ADMIN,
                false,
                UserStatus.DISABLED,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        );

        assertThat(evaluator.hasAnyRole(user, RequiredRole.SUPERADMIN)).isFalse();
        assertThat(evaluator.hasAnyRole(user, RequiredRole.ADMIN)).isFalse();
        assertThat(evaluator.hasAnyRole(user)).isFalse();
    }

    private User activeUser(UserRole role) {
        return new User(
                null,
                null,
                "active@miletos.local",
                "hash",
                "Active",
                "User",
                role,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        );
    }
}
