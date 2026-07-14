import { type OnboardingStatus, type UserStatus } from "@/shared/session/types/session-user-types";

export interface CompletePasswordRequest {
  token: string;
  password: string;
}

export interface CompletePasswordResponse {
  userId: number;
  email: string;
  status: UserStatus;
  onboardingStatus: OnboardingStatus;
}
