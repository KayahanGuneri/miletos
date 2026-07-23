import { type UserProfile } from "@/shared/session/types/session-user-types";
import { httpClient } from "@/shared/api/http-client";

export async function getCurrentUser() {
  const response = await httpClient.get<UserProfile>("/api/users/me");

  return response.data;
}
