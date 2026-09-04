import { api } from "../../api";
import { alertsApi } from "../../api/alerts";
import { eventsApi } from "../../api/events";
import { monitoringApi } from "../../api/monitoring";
import { cachedQuery, queryClient } from "../../api/queryClient";
import type { AlertPage } from "../../types/alert";
import type { EventPage } from "../../types/event";
import type { MonitoringRule } from "../../types/monitoring";
import type { SenderOption } from "../alerts/SenderSelect";

export interface MonitoringEditorData {
  eventPage: EventPage;
  alertPage: AlertPage;
  rule?: MonitoringRule;
  senders: SenderOption[];
}

const key = (ruleId?: string) => ["monitoring-editor", ruleId ?? "new"] as const;
const presented = new Set<string>();

export function hasPresentedMonitoringEditor(ruleId?: string) {
  return presented.has(ruleId ?? "new");
}

export function markMonitoringEditorPresented(ruleId?: string) {
  presented.add(ruleId ?? "new");
}

async function load(ruleId?: string): Promise<MonitoringEditorData> {
  const [eventPage, alertPage, rule] = await Promise.all([
    eventsApi.list("page=1&page_size=100"),
    alertsApi.list("page=1&page_size=100"),
    ruleId ? monitoringApi.get(ruleId) : Promise.resolve(undefined),
  ]);
  const senders = rule ? await Promise.all(rule.sender_ids.map((id) => api.sender(id))) : [];
  return { eventPage, alertPage, rule, senders };
}

export function getMonitoringEditorData(ruleId?: string) {
  return cachedQuery(key(ruleId), () => load(ruleId));
}

export function peekMonitoringEditorData(ruleId?: string) {
  return queryClient.getQueryData<MonitoringEditorData>(key(ruleId));
}

export function preloadMonitoringEditor(ruleId?: string) {
  void queryClient.prefetchQuery({ queryKey: key(ruleId), queryFn: () => load(ruleId) });
}

export function setMonitoringEditorData(ruleId: string, data: MonitoringEditorData) {
  queryClient.setQueryData(key(ruleId), data);
}
