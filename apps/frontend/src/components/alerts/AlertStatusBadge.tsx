import type { DeliveryStatus } from "../../types/alert";

export function AlertStatusBadge({ enabled }: { enabled: boolean }) {
  return <span className={`rounded-full border px-2 py-1 text-[10px] font-medium ${enabled ? "border-emerald-900 bg-emerald-950/30 text-emerald-400" : "border-zinc-700 bg-zinc-900 text-zinc-500"}`}>{enabled ? "Active" : "Inactive"}</span>;
}

export function DeliveryBadge({ status }: { status: DeliveryStatus | null }) {
  if (!status) return <span className="text-zinc-600">No deliveries</span>;
  const value = {
    pending: ["Queued", "text-amber-400"],
    sent: ["Sent", "text-emerald-400"],
    failed: ["Delivery failed", "text-red-400"],
  }[status];
  return <span className={`inline-flex items-center gap-1.5 ${value[1]}`}><span className="size-1.5 rounded-full bg-current" />{value[0]}</span>;
}

