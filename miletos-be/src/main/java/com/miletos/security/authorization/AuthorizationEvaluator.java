package com.miletos.security.authorization;

import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import org.springframework.stereotype.Component;

@Component
public class AuthorizationEvaluator {

    public boolean hasAnyRole(User user, RequiredRole... requiredRoles) {
        if (user == null || !isActive(user)) {
            return false;
        }

        if (requiredRoles == null || requiredRoles.length == 0) {
            return true;
        }

        for (RequiredRole requiredRole : requiredRoles) {
            if (hasRole(user, requiredRole)) {
                return true;
            }
        }

        return false;
    }

    public boolean isActive(User user) {
        if (user == null) {
            return false;
        }

        boolean statusActive = user.getStatus() == UserStatus.ACTIVE;
        boolean onboardingCompleted = user.getOnboardingStatus() == OnboardingStatus.COMPLETED
                || user.getOnboardingStatus() == OnboardingStatus.NOT_REQUIRED;

        return statusActive && onboardingCompleted;
    }

    public boolean isCompanyAdmin(User user) {
        return isActive(user)
                && !user.isSuperAdmin()
                && user.getRole() == UserRole.ADMIN
                && user.getCompany() != null
                && user.getCompany().getId() != null;
    }

    private boolean hasRole(User user, RequiredRole requiredRole) {
        return switch (requiredRole) {
            case SUPERADMIN -> user.isSuperAdmin();
            case ADMIN -> user.isSuperAdmin() || user.getRole() == UserRole.ADMIN;
            case COMPANY_ADMIN -> isCompanyAdmin(user);
            case USER -> user.isSuperAdmin() || user.getRole() != null;
        };
    }
}
