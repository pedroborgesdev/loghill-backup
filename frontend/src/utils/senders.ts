import type { Sender, SenderStatus } from "../types/api";

export interface SenderGroup {
  key: string;
  name: string;
  items: Sender[];
  status: SenderStatus;
  lastActivityAt: string | null;
  logLineCount: number;
  logFileSize: number;
  recentErrorCount: number;
}

function groupStatus(items: Sender[]): SenderStatus {
  const priority: SenderStatus[] = [
    "online",
    "inactive",
    "never_connected",
    "revoked",
    "archived",
    "expired",
  ];
  return priority.find((status) => items.some((item) => item.status === status))
    ?? "expired";
}

export function groupSenders(items: Sender[]): SenderGroup[] {
  const groups = new Map<string, Sender[]>();
  for (const sender of items) {
    const key = sender.name.trim().toLocaleLowerCase("pt-BR");
    const current = groups.get(key);
    if (current) current.push(sender);
    else groups.set(key, [sender]);
  }

  return Array.from(groups, ([key, groupItems]) => ({
    key,
    name: groupItems[0].name,
    items: groupItems,
    status: groupStatus(groupItems),
    lastActivityAt: groupItems.reduce<string | null>(
      (latest, sender) => !latest || (sender.last_activity_at && new Date(sender.last_activity_at) > new Date(latest)) ? sender.last_activity_at : latest,
      groupItems[0].last_activity_at,
    ),
    logLineCount: groupItems.reduce(
      (total, sender) => total + sender.log_line_count,
      0,
    ),
    logFileSize: groupItems.reduce(
      (total, sender) => total + sender.log_file_size,
      0,
    ),
    recentErrorCount: groupItems.reduce(
      (total, sender) => total + (sender.recent_error_count ?? 0),
      0,
    ),
  }));
}
