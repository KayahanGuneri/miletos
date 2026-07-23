import {
  type OnboardingStatus,
  type UserRole,
  type UserStatus,
} from "@/shared/session/types/session-user-types";

export interface InviteUserRequest {
  companyId: number;
  email: string;
  firstName: string;
  lastName: string;
  role: UserRole;
}

export interface InviteUserResponse {
  userId: number;
  companyId: number;
  email: string;
  firstName: string;
  lastName: string;
  role: UserRole;
  status: UserStatus;
  onboardingStatus: OnboardingStatus;
  inviteExpiresAt: string;
}
