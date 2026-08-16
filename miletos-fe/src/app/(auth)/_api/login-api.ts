import {
  type LoginRequest,
  type LoginResponse,
} from "@/app/(auth)/_modules/login/model/login-types";
import { publicHttpClient } from "@/shared/api/http-client";

export async function login(request: LoginRequest) {
  const response = await publicHttpClient.post<LoginResponse>("/auth/login", request);

  return response.data;
}
