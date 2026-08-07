import { type ForgotPasswordRequest } from "@/app/(auth)/_modules/forgot-password/model/forgot-password-types";
import {
  type ResetPasswordRequest,
  type ResetPasswordResponse,
} from "@/app/(auth)/_modules/reset-password/model/reset-password-types";
import { httpClient } from "@/shared/api/http-client";

export async function requestPasswordReset(request: ForgotPasswordRequest): Promise<void> {
  await httpClient.post("/auth/forgot-password", request);
}

export async function resetPassword(request: ResetPasswordRequest) {
  const response = await httpClient.post<ResetPasswordResponse>("/auth/reset-password", request);

  return response.data;
}
