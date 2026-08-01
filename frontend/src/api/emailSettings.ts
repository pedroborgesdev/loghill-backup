import { request } from "./client";
import type { EmailSettings, EmailSettingsInput } from "../types/email";

export const emailSettingsApi = {
  get: () => request<EmailSettings>("/api/v1/settings/email"),
  update: (input: EmailSettingsInput) => request<EmailSettings>("/api/v1/settings/email", json("PUT", input)),
  testConnection: () => request<{ success: boolean; message: string }>("/api/v1/settings/email/test-connection", { method: "POST" }),
  sendTest: (recipient: string) => request<{ success: boolean; message: string }>("/api/v1/settings/email/send-test", json("POST", { recipient })),
};

function json(method: string, value: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(value) };
}

