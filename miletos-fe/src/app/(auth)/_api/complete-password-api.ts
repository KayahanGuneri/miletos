import {
  type CompletePasswordRequest,
  type CompletePasswordResponse,
} from "@/app/(auth)/_modules/complete-password/model/complete-password-types";
import { publicHttpClient } from "@/shared/api/http-client";

export async function completePassword(request: CompletePasswordRequest) {
  const response = await publicHttpClient.post<CompletePasswordResponse>(
    "/auth/complete-password",
    request,
  );

  return response.data;
}
