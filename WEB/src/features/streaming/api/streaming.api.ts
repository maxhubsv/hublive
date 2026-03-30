import { apiClient } from "@/lib/axios";

interface TokenResponse {
  token: string;
}

export const streamingApi = {
  getToken: (roomName: string) =>
    apiClient.post<TokenResponse>("/api/v1/rooms/token", { roomName }),
};
