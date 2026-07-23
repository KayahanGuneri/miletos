import { mixed, object, ref, string } from "yup";
import { profileMessages } from "../messages/profile-messages";

export const SUPPORTED_PROFILE_PHOTO_TYPES = ["image/png", "image/jpeg", "image/webp"];

export const passwordFormSchema = object({
  currentPassword: string().required(profileMessages.password.currentRequired),
  newPassword: string().required(profileMessages.password.newRequired),
  confirmNewPassword: string()
    .required(profileMessages.password.confirmRequired)
    .oneOf([ref("newPassword")], profileMessages.password.mismatch),
});

export const profilePhotoSchema = mixed<File>()
  .required(profileMessages.photo.required)
  .test(
    "supported-profile-photo-type",
    profileMessages.photo.unsupportedType,
    (file) => !file || SUPPORTED_PROFILE_PHOTO_TYPES.includes(file.type),
  );
