import type { DeliveryStatus } from "./alert";
import type { Pagination } from "./api";
import type { EmailSettings } from "./email";

export type EventActionType = "email";

export interface EventDefinition {
  id: string;
  name: string;
  key: string;
  senderIds: string[];
  actionType: EventActionType;
  recipients: string[];
  subjectTemplate: string;
  messageTemplate: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  lastTriggeredAt?: string;
  lastDeliveryAt?: string;
  lastDeliveryStatus?: DeliveryStatus;
  lastDeliveryError?: string;
  triggerCount: number;
  deliveryCount: number;
  failureCount: number;
  testDeliveryCount: number;
}

export interface EventInput {
  name: string;
  key: string;
  senderIds: string[];
  actionType: EventActionType;
  recipients: string[];
  subjectTemplate: string;
  messageTemplate: string;
  enabled: boolean;
}

export interface EventPage {
  items: EventDefinition[];
  pagination: Pagination;
  summary: { total: number; active: number; recentTriggered: number; recentFailures: number };
  emailProvider: EmailSettings;
}
