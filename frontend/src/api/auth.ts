import { request } from "./client";

export interface SessionStatus {
  authenticated: boolean;
  auth_required: boolean;
  expires_at?: string;
}

export const authApi = {
  session: () => request<SessionStatus>("/api/v1/auth/session"),
  login: (password: string) =>
    request<SessionStatus>("/api/v1/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    }),
  logout: () => request<{ authenticated: boolean }>("/api/v1/auth/logout", { method: "POST" }),
};
