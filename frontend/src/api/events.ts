import type { EventDefinition, EventInput, EventPage } from "../types/event";
import { request } from "./client";

function json(method: string, body: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export function normalizeEvent(event: EventDefinition): EventDefinition {
  return {
    ...event,
    sender_ids: Array.isArray(event.sender_ids) ? event.sender_ids : [],
    recipients: Array.isArray(event.recipients) ? event.recipients : [],
    phone_numbers: Array.isArray(event.phone_numbers) ? event.phone_numbers : [],
  };
}

export const eventsApi = {
  list: async (query = "") => {
    const page = await request<EventPage>(`/api/v1/events${query ? `?${query}` : ""}`);
    return { ...page, items: Array.isArray(page.items) ? page.items.map(normalizeEvent) : [] };
  },
  get: async (id: string) => normalizeEvent(await request<EventDefinition>(`/api/v1/events/${encodeURIComponent(id)}`)),
  checkKey: (key: string, signal?: AbortSignal) => request<{ key: string; valid: boolean; available: boolean }>(`/api/v1/events/check-key?key=${encodeURIComponent(key)}`, { signal }),
  create: async (input: EventInput) => normalizeEvent(await request<EventDefinition>("/api/v1/events", json("POST", input))),
  update: async (id: string, input: EventInput) => normalizeEvent(await request<EventDefinition>(`/api/v1/events/${encodeURIComponent(id)}`, json("PUT", input))),
  remove: (id: string) => request<void>(`/api/v1/events/${encodeURIComponent(id)}`, { method: "DELETE" }),
  setEnabled: async (id: string, enabled: boolean) => normalizeEvent(await request<EventDefinition>(`/api/v1/events/${encodeURIComponent(id)}/status`, json("PATCH", { enabled }))),
  sendTest: (id: string, recipient: string) => request<{ accepted: boolean; event_id: string }>(`/api/v1/events/${encodeURIComponent(id)}/test`, json("POST", { recipient })),
};
