import { Edit3, Send } from "lucide-react";
import { useEffect, useId } from "react";
import { createPortal } from "react-dom";
import type { EmailAlert } from "../../types/alert";
import { formatDate, formatNumber } from "../../utils/format";
import { Button, ModalCloseButton } from "../ui";
import { AlertStatusBadge, DeliveryBadge } from "./AlertStatusBadge";
import { EmailProviderBadge } from "./EmailProviderBadge";
import { SeverityBadges } from "./SeveritySelector";
import { SourceRecentExecutions } from "../executions/SourceRecentExecutions";

export function AlertDetailsDrawer({ alert, onClose, onEdit, onTest }: { alert?: EmailAlert; onClose: () => void; onEdit: (alert: EmailAlert) => void; onTest: (alert: EmailAlert) => void }) {
  const titleID = useId();
  useEffect(() => {
    if (!alert) return;
    const close = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", close);
    return () => document.removeEventListener("keydown", close);
  }, [alert, onClose]);
  if (!alert) return null;
  return createPortal(
    <div className="fixed inset-0 z-[215]">
      <button type="button" aria-label="Close details" onClick={onClose} className="absolute inset-0 bg-black/65" />
      <aside role="dialog" aria-modal="true" aria-labelledby={titleID} className="absolute inset-y-0 right-0 flex w-full max-w-xl flex-col border-l border-zinc-700 bg-[#111113] shadow-2xl shadow-black/70">
        <header className="flex items-start justify-between gap-4 border-b border-zinc-800 px-5 py-4"><div className="min-w-0"><div className="flex items-center gap-2"><h2 id={titleID} className="truncate text-base font-semibold text-zinc-100">{alert.name}</h2><AlertStatusBadge enabled={alert.enabled} /></div><code className="mt-1 block text-[10px] text-zinc-600">{alert.id}</code></div><ModalCloseButton label="Close alert details" onClick={onClose} /></header>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
          <section><h3 className="text-[10px] font-medium uppercase tracking-wide text-zinc-600">Senders</h3><div className="mt-2 space-y-1.5">{alert.sender_ids.map((id, index) => <div key={id} className="rounded-lg border border-zinc-800 bg-zinc-950/60 px-3 py-2"><p className="text-xs text-zinc-200">{alert.sender_names[index] ?? id}</p><code className="text-[10px] text-zinc-600">{id}</code></div>)}</div></section>
          <section className="grid gap-4 sm:grid-cols-2"><div><h3 className="mb-2 text-[10px] font-medium uppercase tracking-wide text-zinc-600">Severidades</h3><SeverityBadges values={alert.severities} /></div><div><h3 className="mb-2 text-[10px] font-medium uppercase tracking-wide text-zinc-600">Provider</h3><EmailProviderBadge provider={alert.provider} /></div></section>
          <section><h3 className="text-[10px] font-medium uppercase tracking-wide text-zinc-600">Recipients</h3><div className="mt-2 flex flex-wrap gap-1.5">{alert.recipients.map((recipient) => <span key={recipient} className="rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-[10px] text-zinc-300">{recipient}</span>)}</div></section>
          <section className="grid grid-cols-2 gap-3 rounded-xl border border-zinc-800 bg-zinc-950/40 p-4 text-xs"><Value label="Created" value={formatDate(alert.created_at)} /><Value label="Last updated" value={formatDate(alert.updated_at)} /><Value label="Last trigger" value={formatDate(alert.last_triggered_at)} /><Value label="Last delivery" value={formatDate(alert.last_delivery_at)} /><Value label="Deliveries" value={formatNumber(alert.delivery_count)} /><Value label="Failures" value={formatNumber(alert.failure_count)} /><div className="col-span-2"><p className="text-zinc-600">Result</p><div className="mt-1"><DeliveryBadge status={alert.last_delivery_status} /></div></div>{alert.last_delivery_error && <div className="col-span-2"><p className="text-zinc-600">Last error</p><p className="mt-1 break-words text-red-400">{alert.last_delivery_error}</p></div>}</section>
          <SourceRecentExecutions source="alert" sourceID={alert.id}/>
        </div>
        <footer className="flex justify-end gap-2 border-t border-zinc-800 bg-zinc-950 p-3"><Button onClick={() => onTest(alert)}><Send className="size-4" />Send test</Button><Button onClick={() => onEdit(alert)} className="border-zinc-500 bg-zinc-800 text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700"><Edit3 className="size-4" />Edit</Button></footer>
      </aside>
    </div>, document.body,
  );
}

function Value({ label, value }: { label: string; value: string }) { return <div><p className="text-zinc-600">{label}</p><p className="mt-1 text-zinc-300">{value}</p></div>; }
