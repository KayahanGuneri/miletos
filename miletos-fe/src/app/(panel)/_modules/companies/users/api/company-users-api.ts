import { type InviteUserRequest, type InviteUserResponse } from "../types/invite-user-types";
import { type UserProfile } from "@/shared/session/types/session-user-types";
import { httpClient } from "@/shared/api/http-client";

export async function getCompanyUsers(companyId: number) {
  const response = await httpClient.get<UserProfile[]>(`/companies/${companyId}/users`);

  return response.data;
}

export async function inviteUser(request: InviteUserRequest) {
  const response = await httpClient.post<InviteUserResponse>("/auth/invitations", request);

  return response.data;
}
