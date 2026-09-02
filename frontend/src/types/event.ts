import type { DeliveryStatus } from "./alert";
import type { Pagination } from "./api";
import type { EmailSettings } from "./email";

export type EventActionType = "none" | "email" | "webhook" | "sms";

export interface EventDefinition {
  id: string;
  name: string;
  key: string;
  sender_ids: string[];
  action_type: EventActionType;
  recipients: string[];
  subject_template: string;
  message_template: string;
  webhook_url?: string;
  phone_numbers?: string[];
  sms_template?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  last_triggered_at?: string | null;
  last_delivery_at?: string | null;
  last_delivery_status?: DeliveryStatus | null;
  last_delivery_error?: string | null;
  trigger_count: number;
  delivery_count: number;
  failure_count: number;
  test_delivery_count: number;
}

export interface EventInput {
  name: string;
  key: string;
  sender_ids: string[];
  action_type: EventActionType;
  recipients: string[];
  subject_template: string;
  message_template: string;
  webhook_url?: string;
  phone_numbers?: string[];
  sms_template?: string;
  enabled: boolean;
}

export interface EventPage {
  items: EventDefinition[];
  pagination: Pagination;
  summary: { total: number; active: number; recent_triggered: number; recent_failures: number };
  email_provider: EmailSettings;
}
