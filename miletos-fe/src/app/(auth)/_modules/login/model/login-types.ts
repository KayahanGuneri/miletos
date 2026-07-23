import { type AuthenticatedUser } from "@/shared/session/types/session-user-types";

export interface LoginRequest {
  email: string;
  password: string;
}

export type TokenType = "Bearer";

export interface LoginResponse {
  accessToken: string;
  tokenType: TokenType;
  expiresAt: string;
  user: AuthenticatedUser;
}
