import { apiClient } from "@/lib/axios";
import type { LoginRequest, LoginResponse, User } from "../types/auth.types";

export const authApi = {
  login: (data: LoginRequest) =>
    apiClient.post<LoginResponse>("/api/v1/auth/login", data),

  me: () => apiClient.get<User>("/api/v1/auth/me"),
};
