import axios, { AxiosHeaders } from "axios";
import { dispatchUnauthorizedEvent } from "@/shared/session/events/session-events";
import { getAccessToken, removeAccessToken } from "@/shared/session/storage/access-token-storage";

const apiBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;

if (!apiBaseUrl) {
  throw new Error("NEXT_PUBLIC_API_BASE_URL is required");
}

function resolveApiBaseUrl(baseUrl: string) {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, "");

  return normalizedBaseUrl.endsWith("/api") ? normalizedBaseUrl : `${normalizedBaseUrl}/api`;
}

export const httpClient = axios.create({
  baseURL: resolveApiBaseUrl(apiBaseUrl),
  timeout: 15000,
});

httpClient.interceptors.request.use((config) => {
  const accessToken = getAccessToken();

  if (!accessToken) {
    return config;
  }

  if (config.headers instanceof AxiosHeaders) {
    config.headers.set("Authorization", `Bearer ${accessToken}`);
    return config;
  }

  const headers = new AxiosHeaders(config.headers);
  headers.set("Authorization", `Bearer ${accessToken}`);
  config.headers = headers;

  return config;
});

httpClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error?.response?.status === 401) {
      removeAccessToken();

      dispatchUnauthorizedEvent();
    }

    return Promise.reject(error);
  },
);
