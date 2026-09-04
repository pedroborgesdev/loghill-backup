import { request } from "./client";
import type { MonitoringPage, MonitoringRule, MonitoringRuleInput, MonitoringTestInput, MonitoringTestResult } from "../types/monitoring";

const base = "/api/v1/monitoring";
export const monitoringApi = {
  list: (query = "") => request<MonitoringPage>(`${base}/rules${query ? `?${query}` : ""}`),
  get: (id: string) => request<MonitoringRule>(`${base}/rules/${encodeURIComponent(id)}`),
  create: (input: MonitoringRuleInput) => request<MonitoringRule>(`${base}/rules`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }),
  update: (id: string, input: MonitoringRuleInput) => request<MonitoringRule>(`${base}/rules/${encodeURIComponent(id)}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }),
  setEnabled: (id: string, enabled: boolean) => request<MonitoringRule>(`${base}/rules/${encodeURIComponent(id)}/status`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ enabled }) }),
  remove: (id: string) => request<void>(`${base}/rules/${encodeURIComponent(id)}`, { method: "DELETE" }),
  duplicate: (id: string) => request<MonitoringRule>(`${base}/rules/${encodeURIComponent(id)}/duplicate`, { method: "POST" }),
  validate: (input: MonitoringRuleInput) => request<{ valid: boolean }>(`${base}/rules/validate`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }),
  test: (id: string, input: MonitoringTestInput) => request<MonitoringTestResult>(`${base}/rules/${encodeURIComponent(id)}/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }),
};
