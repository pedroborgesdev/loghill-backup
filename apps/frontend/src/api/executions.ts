import { request } from "./client";
import type { ExecutionPage, ExecutionRecord } from "../types/execution";

const withoutSkipped = (page: ExecutionPage): ExecutionPage => ({
  ...page,
  items: page.items.filter((record) => record.status !== "skipped"),
});

const visibleStatuses = "pending,processing,success,partial,failed,cancelled";

function withVisibleStatuses(query: string) {
  const params = new URLSearchParams(query);
  if (!params.has("status")) params.set("status", visibleStatuses);
  return params.toString();
}

export const executionsApi = {
  list: (query: string) => request<ExecutionPage>(`/api/v1/executions?${withVisibleStatuses(query)}`).then(withoutSkipped),
  get: (id: string) => request<ExecutionRecord>(`/api/v1/executions/${encodeURIComponent(id)}`),
  recent: (query = "limit=10") => request<ExecutionPage>(`/api/v1/dashboard/recent-executions?${withVisibleStatuses(query)}`).then(withoutSkipped),
};
