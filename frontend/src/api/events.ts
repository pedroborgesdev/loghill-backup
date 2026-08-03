import type { EventDefinition, EventInput, EventPage } from "../types/event";
import { request } from "./client";

function json(method: string, body: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export const eventsApi = {
  list: (query = "") => request<EventPage>(`/api/v1/events${query ? `?${query}` : ""}`),
  get: (id: string) => request<EventDefinition>(`/api/v1/events/${encodeURIComponent(id)}`),
  checkKey: (key: string, signal?: AbortSignal) => request<{ key: string; valid: boolean; available: boolean }>(`/api/v1/events/check-key?key=${encodeURIComponent(key)}`, { signal }),
  create: (input: EventInput) => request<EventDefinition>("/api/v1/events", json("POST", input)),
  update: (id: string, input: EventInput) => request<EventDefinition>(`/api/v1/events/${encodeURIComponent(id)}`, json("PUT", input)),
  remove: (id: string) => request<void>(`/api/v1/events/${encodeURIComponent(id)}`, { method: "DELETE" }),
  setEnabled: (id: string, enabled: boolean) => request<EventDefinition>(`/api/v1/events/${encodeURIComponent(id)}/status`, json("PATCH", { enabled })),
  sendTest: (id: string, recipient: string) => request<{ accepted: boolean; event_id: string }>(`/api/v1/events/${encodeURIComponent(id)}/test`, json("POST", { recipient })),
};
