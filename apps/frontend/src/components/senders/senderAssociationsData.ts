import { alertsApi } from "../../api/alerts";
import { eventsApi } from "../../api/events";
import { cachedQuery, queryClient } from "../../api/queryClient";
import type { EmailAlert } from "../../types/alert";
import type { EventDefinition } from "../../types/event";
import type { AssociationKind } from "./SenderAssociationsDialog";

export interface AssociationItem {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  senderIds: string[];
  source: EmailAlert | EventDefinition;
}

async function loadAll(kind: AssociationKind) {
  const items: AssociationItem[] = [];
  let page = 1;
  let totalPages = 1;
  do {
    const query = new URLSearchParams({ page: String(page), page_size: "100" }).toString();
    if (kind === "alerts") {
      const result = await alertsApi.list(query);
      items.push(...result.items.map((alert) => ({ id: alert.id, name: alert.name, description: alert.severities.join(", "), enabled: alert.enabled, senderIds: alert.sender_ids, source: alert })));
      totalPages = result.pagination.total_pages;
    } else {
      const result = await eventsApi.list(query);
      items.push(...result.items.map((event) => ({ id: event.id, name: event.name, description: event.key, enabled: event.enabled, senderIds: event.sender_ids, source: event })));
      totalPages = result.pagination.total_pages;
    }
    page += 1;
  } while (page <= totalPages);
  return items;
}

const queryKey = (kind: AssociationKind) => ["sender-associations", kind] as const;

export function getSenderAssociations(kind: AssociationKind) {
  return cachedQuery(queryKey(kind), () => loadAll(kind));
}

export function preloadSenderAssociations(kind: AssociationKind) {
  void queryClient.prefetchQuery({ queryKey: queryKey(kind), queryFn: () => loadAll(kind) });
}

export function invalidateSenderAssociations(kind: AssociationKind) {
  return queryClient.invalidateQueries({ queryKey: queryKey(kind), exact: true });
}
