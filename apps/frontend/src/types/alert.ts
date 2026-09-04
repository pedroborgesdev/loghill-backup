import type { LogSeverity, Pagination } from "./api";

export type EmailProviderType = "outlook" | "gmail";
export type DeliveryStatus = "pending" | "sent" | "failed";

export interface EmailAlert {
  id: string;
  name: string;
  sender_ids: string[];
  sender_names: string[];
  severities: LogSeverity[];
  recipients: string[];
  provider: EmailProviderType;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  last_triggered_at: string | null;
  last_delivery_at: string | null;
  last_delivery_status: DeliveryStatus | null;
  last_delivery_error: string | null;
  delivery_count: number;
  failure_count: number;
  test_delivery_count: number;
}

export interface AlertInput {
  name: string;
  sender_ids: string[];
  severities: LogSeverity[];
  recipients: string[];
  provider: EmailProviderType;
  enabled: boolean;
}

export interface AlertPage {
  items: EmailAlert[];
  pagination: Pagination;
  summary: { total: number; active: number; recent_failures: number };
  email_provider: import("./email").EmailSettings;
}
