import { request } from "./client";
import type { AlertInput, AlertPage, EmailAlert } from "../types/alert";

export const alertsApi = {
  list: async (query = "") => { const page = await request<AlertPage>(`/api/v1/alerts${query ? `?${query}` : ""}`); return { ...page, items: page.items.map(normalizeAlert) }; },
  get: async (id: string) => normalizeAlert(await request<EmailAlert>(`/api/v1/alerts/${encodeURIComponent(id)}`)),
  create: async (input: AlertInput) => normalizeAlert(await request<EmailAlert>("/api/v1/alerts", json("POST", input))),
  update: async (id: string, input: AlertInput) => normalizeAlert(await request<EmailAlert>(`/api/v1/alerts/${encodeURIComponent(id)}`, json("PUT", input))),
  remove: (id: string) => request<void>(`/api/v1/alerts/${encodeURIComponent(id)}`, { method: "DELETE" }),
  setEnabled: async (id: string, enabled: boolean) => normalizeAlert(await request<EmailAlert>(`/api/v1/alerts/${encodeURIComponent(id)}/status`, json("PATCH", { enabled }))),
  sendTest: (id: string) => request<{ accepted: boolean; alert_id: string }>(`/api/v1/alerts/${encodeURIComponent(id)}/test`, { method: "POST" }),
};

function normalizeAlert(alert: EmailAlert & { sender_id?: string; sender_name?: string }): EmailAlert {
  return {
    ...alert,
    sender_ids: alert.sender_ids ?? (alert.sender_id ? [alert.sender_id] : []),
    sender_names: alert.sender_names ?? (alert.sender_name ? [alert.sender_name] : []),
  };
}

function json(method: string, value: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(value) };
}
