export type UserRole = "ADMIN" | "MOD" | "USER";

export type UserStatus = "PENDING" | "ACTIVE" | "DISABLED";

export type OnboardingStatus = "NOT_REQUIRED" | "INVITED" | "PASSWORD_SETUP_REQUIRED" | "COMPLETED";

export interface AuthenticatedUser {
  id: number;
  companyId: number | null;
  email: string;
  firstName: string;
  lastName: string;
  role: UserRole | null;
  superAdmin: boolean;
  status: UserStatus;
  onboardingStatus: OnboardingStatus;
}

export interface UserProfile extends AuthenticatedUser {
  profilePhotoFileId: number | null;
}
