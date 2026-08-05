import {
  type LoginRequest,
  type LoginResponse,
} from "@/app/(auth)/_modules/login/model/login-types";
import { httpClient } from "@/shared/api/http-client";

export async function login(request: LoginRequest) {
  const response = await httpClient.post<LoginResponse>("/auth/login", request);

  return response.data;
}
