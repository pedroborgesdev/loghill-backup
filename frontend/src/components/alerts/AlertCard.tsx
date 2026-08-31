import { Edit3, MoreHorizontal, Power, Send, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { EmailAlert } from "../../types/alert";
import { formatDate } from "../../utils/format";
import { AlertStatusBadge, DeliveryBadge } from "./AlertStatusBadge";
import { EmailProviderBadge } from "./EmailProviderBadge";
import { SeverityBadges } from "./SeveritySelector";

export function AlertCard({ alert, busy, onEdit, onToggle, onTest, onDelete }: { alert: EmailAlert; busy: boolean; onEdit: () => void; onToggle: () => void; onTest: () => void; onDelete: () => void }) {
  const [open, setOpen] = useState(false);
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => { if (!menu.current?.contains(event.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open]);
  const action = (callback: () => void) => { setOpen(false); callback(); };
  return (
    <article className="relative border-b border-zinc-800 p-4 last:border-b-0">
      <div className="flex items-start gap-3"><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h3 className="truncate text-sm font-medium text-zinc-100">{alert.name}</h3><AlertStatusBadge enabled={alert.enabled} /></div><p className="mt-1 truncate text-[11px] text-zinc-500"><span className="text-zinc-400">{alert.sender_names.join(", ")}</span> · <span className="font-mono">{alert.sender_ids.join(", ")}</span></p></div><div ref={menu} className="relative"><button type="button" aria-label={`Actions de ${alert.name}`} aria-haspopup="menu" aria-expanded={open} disabled={busy} onClick={() => setOpen((value) => !value)} className="grid size-8 place-items-center rounded-lg text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100"><MoreHorizontal className="size-4" /></button>{open && <div role="menu" className="absolute right-0 top-9 z-20 w-44 rounded-lg border border-zinc-700 bg-[#1b1b1e] p-1 shadow-xl shadow-black/50">{[[Edit3,"Edit",onEdit],[Power,alert.enabled ? "Disable" : "Enable",onToggle],[Send,"Send test",onTest],[Trash2,"Delete",onDelete]].map(([Icon,label,callback]) => { const Component=Icon as typeof Edit3; return <button key={String(label)} type="button" role="menuitem" onClick={() => action(callback as () => void)} className={`flex h-8 w-full items-center gap-2 rounded-md px-2 text-xs hover:bg-zinc-800 ${label === "Delete" ? "text-red-400" : "text-zinc-300"}`}><Component className="size-3.5" />{String(label)}</button>; })}</div>}</div></div>
      <div className="mt-3 grid gap-3 text-[11px] sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto]"><SeverityBadges values={alert.severities} /><p className="truncate text-zinc-500" title={alert.recipients.join(", ")}><span className="text-zinc-600">Recipients:</span> {alert.recipients.join(", ")}</p><EmailProviderBadge provider={alert.provider} /></div>
      <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-1 border-t border-zinc-800/70 pt-3 text-[10px] text-zinc-600"><DeliveryBadge status={alert.last_delivery_status} /><span>Last delivery: {alert.last_delivery_at ? formatDate(alert.last_delivery_at) : "—"}</span><span>Deliveries: {alert.delivery_count}</span><span>Created: {formatDate(alert.created_at)}</span><span>Updated: {formatDate(alert.updated_at)}</span>{alert.last_delivery_error && <span className="basis-full truncate text-red-400" title={alert.last_delivery_error}>{alert.last_delivery_error}</span>}</div>
    </article>
  );
}
