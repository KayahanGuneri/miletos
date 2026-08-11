import axios from "axios";

export type ApiFieldErrors = Record<string, string>;

export interface ApiErrorResponse {
  code: string;
  message: string;
  path?: string;
  timestamp: string;
  fieldErrors?: ApiFieldErrors | null;
}

export interface ApiError {
  code: string;
  message: string;
  fieldErrors: ApiFieldErrors;
  status?: number;
  isUnauthorized: boolean;
  isForbidden: boolean;
  originalError: unknown;
}

const DEFAULT_API_ERROR_MESSAGE = "Something went wrong. Please try again.";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function isApiErrorResponse(value: unknown): value is ApiErrorResponse {
  if (!isRecord(value)) {
    return false;
  }

  return (
    typeof value.code === "string" &&
    typeof value.message === "string" &&
    (value.path === undefined || typeof value.path === "string") &&
    typeof value.timestamp === "string"
  );
}

export function toApiError(error: unknown): ApiError {
  if (axios.isAxiosError(error)) {
    const status = error.response?.status;
    const responseData = error.response?.data;

    if (isApiErrorResponse(responseData)) {
      return {
        code: responseData.code,
        message: responseData.message,
        fieldErrors: responseData.fieldErrors ?? {},
        status,
        isUnauthorized: status === 401,
        isForbidden: status === 403,
        originalError: error,
      };
    }

    return {
      code: status === 401 ? "UNAUTHORIZED" : "HTTP_ERROR",
      message: error.message || DEFAULT_API_ERROR_MESSAGE,
      fieldErrors: {},
      status,
      isUnauthorized: status === 401,
      isForbidden: status === 403,
      originalError: error,
    };
  }

  if (error instanceof Error) {
    return {
      code: "UNKNOWN_ERROR",
      message: error.message || DEFAULT_API_ERROR_MESSAGE,
      fieldErrors: {},
      isUnauthorized: false,
      isForbidden: false,
      originalError: error,
    };
  }

  return {
    code: "UNKNOWN_ERROR",
    message: DEFAULT_API_ERROR_MESSAGE,
    fieldErrors: {},
    isUnauthorized: false,
    isForbidden: false,
    originalError: error,
  };
}
