import { type OnboardingStatus, type UserStatus } from "@/shared/session/types/session-user-types";

export interface ResetPasswordRequest {
  token: string;
  password: string;
}

export interface ResetPasswordResponse {
  userId: number;
  email: string;
  status: UserStatus;
  onboardingStatus: OnboardingStatus;
}
