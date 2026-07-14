import { type UserProfile } from "@/shared/session/types/session-user-types";
import { type ChangePasswordRequest } from "../types/profile-api-types";
import { httpClient } from "@/shared/api/http-client";

interface UpdateProfilePhotoRequest {
  originalFilename: string;
  contentType: string;
  base64Content: string;
}

export async function updateOwnProfilePhoto(file: File) {
  const request: UpdateProfilePhotoRequest = {
    originalFilename: file.name,
    contentType: file.type,
    base64Content: await readFileAsBase64Content(file),
  };

  const response = await httpClient.put<UserProfile>("/api/users/me/profile-photo", request);

  return response.data;
}

export async function getOwnProfilePhoto() {
  const response = await httpClient.get<Blob>("/api/users/me/profile-photo", {
    responseType: "blob",
  });

  return response.data;
}

export async function changePassword(request: ChangePasswordRequest): Promise<void> {
  await httpClient.post("/api/auth/change-password", request);
}

function readFileAsBase64Content(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();

    reader.onerror = () => {
      reject(new Error("Profile photo could not be read."));
    };

    reader.onload = () => {
      if (typeof reader.result !== "string") {
        reject(new Error("Profile photo could not be converted to base64."));
        return;
      }

      const [, base64Content = ""] = reader.result.split(",");
      resolve(base64Content);
    };

    reader.readAsDataURL(file);
  });
}
