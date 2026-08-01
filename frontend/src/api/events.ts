import type { EventDefinition, EventInput, EventPage } from "../types/event";
import type { EmailSettings } from "../types/email";
import type { Pagination } from "../types/api";
import { request } from "./client";

interface RawEvent {
  id: string; name: string; key: string; sender_ids: string[]; action_type: "email";
  recipients: string[]; subject_template: string; message_template: string; enabled: boolean;
  created_at: string; updated_at: string; last_triggered_at?: string | null;
  last_delivery_at?: string | null; last_delivery_status?: "pending" | "sent" | "failed" | null;
  last_delivery_error?: string | null; trigger_count: number; delivery_count: number;
  failure_count: number; test_delivery_count: number;
}

interface RawPage {
  items: RawEvent[];
  pagination: Pagination;
  summary: { total: number; active: number; recent_triggered: number; recent_failures: number };
  email_provider: EmailSettings;
}

function normalize(value: RawEvent): EventDefinition {
  return {
    id: value.id, name: value.name, key: value.key, senderIds: value.sender_ids ?? [], actionType: value.action_type,
    recipients: value.recipients ?? [], subjectTemplate: value.subject_template, messageTemplate: value.message_template,
    enabled: value.enabled, createdAt: value.created_at, updatedAt: value.updated_at,
    lastTriggeredAt: value.last_triggered_at ?? undefined, lastDeliveryAt: value.last_delivery_at ?? undefined,
    lastDeliveryStatus: value.last_delivery_status ?? undefined, lastDeliveryError: value.last_delivery_error ?? undefined,
    triggerCount: value.trigger_count ?? 0, deliveryCount: value.delivery_count ?? 0,
    failureCount: value.failure_count ?? 0, testDeliveryCount: value.test_delivery_count ?? 0,
  };
}

function payload(value: EventInput) {
  return { name: value.name, key: value.key, sender_ids: value.senderIds, action_type: value.actionType, recipients: value.recipients, subject_template: value.subjectTemplate, message_template: value.messageTemplate, enabled: value.enabled };
}

function json(method: string, body: unknown): RequestInit {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

export const eventsApi = {
  list: async (query = ""): Promise<EventPage> => {
    const value = await request<RawPage>(`/api/v1/events${query ? `?${query}` : ""}`);
    return { items: value.items.map(normalize), pagination: value.pagination, summary: { total: value.summary.total, active: value.summary.active, recentTriggered: value.summary.recent_triggered, recentFailures: value.summary.recent_failures }, emailProvider: value.email_provider };
  },
  get: async (id: string) => normalize(await request<RawEvent>(`/api/v1/events/${encodeURIComponent(id)}`)),
  checkKey: (key: string, signal?: AbortSignal) => request<{ key: string; valid: boolean; available: boolean }>(`/api/v1/events/check-key?key=${encodeURIComponent(key)}`, { signal }),
  create: async (input: EventInput) => normalize(await request<RawEvent>("/api/v1/events", json("POST", payload(input)))),
  update: async (id: string, input: EventInput) => normalize(await request<RawEvent>(`/api/v1/events/${encodeURIComponent(id)}`, json("PUT", payload(input)))),
  remove: (id: string) => request<void>(`/api/v1/events/${encodeURIComponent(id)}`, { method: "DELETE" }),
  setEnabled: async (id: string, enabled: boolean) => normalize(await request<RawEvent>(`/api/v1/events/${encodeURIComponent(id)}/status`, json("PATCH", { enabled }))),
  sendTest: (id: string, recipient: string) => request<{ accepted: boolean; event_id: string }>(`/api/v1/events/${encodeURIComponent(id)}/test`, json("POST", { recipient })),
};
