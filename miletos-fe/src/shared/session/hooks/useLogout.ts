"use client";

import { useQueryClient } from "@tanstack/react-query";
import { removeAccessToken } from "@/shared/session/storage/access-token-storage";

export function useLogout() {
  const queryClient = useQueryClient();

  return () => {
    removeAccessToken();
    queryClient.clear();
  };
}
