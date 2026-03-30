import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { authApi } from "../api/auth.api";
import { useAuthStore } from "../store/auth.store";
import type { LoginRequest } from "../types/auth.types";

export function useLogin() {
  const { t } = useTranslation();
  const setAuth = useAuthStore((s) => s.setAuth);

  return useMutation({
    mutationFn: (data: LoginRequest) => authApi.login(data),
    onSuccess: (res) => {
      setAuth(res.data.user, res.data.token);
      toast.success(t("auth.loginSuccess"));
    },
    onError: () => {
      toast.error(t("auth.loginError"));
    },
  });
}

export function useCurrentUser() {
  const token = useAuthStore((s) => s.token);

  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: () => authApi.me().then((res) => res.data),
    enabled: !!token,
  });
}
