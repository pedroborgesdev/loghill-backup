import type { DeliveryStatus } from "../../types/alert";

export function EventStatusBadge({ enabled }: { enabled: boolean }) {
  return <span className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-1 text-[10px] font-medium ${enabled ? "border-emerald-900 bg-emerald-950/30 text-emerald-400" : "border-zinc-700 bg-zinc-900 text-zinc-500"}`}><span className="size-1.5 rounded-full bg-current" />{enabled ? "Active" : "Inactive"}</span>;
}

export function EventDeliveryBadge({ status }: { status?: DeliveryStatus | null }) {
  if (!status) return <span className="text-zinc-600">No executions</span>;
  const value = { pending: ["Queued", "text-amber-400"], sent: ["Sent", "text-emerald-400"], failed: ["Failed", "text-red-400"] }[status];
  return <span className={`inline-flex items-center gap-1.5 text-xs ${value[1]}`}><span className="size-1.5 rounded-full bg-current" />{value[0]}</span>;
}
