import { Edit3, Eye, Power, Send, Trash2 } from "lucide-react";
import type { EmailAlert } from "../../types/alert";
import { relativeDate } from "../../utils/format";
import { IconButton } from "../ui";
import { AlertStatusBadge } from "./AlertStatusBadge";
import { SeverityBadges } from "./SeveritySelector";

interface Props {
  items: EmailAlert[];
  busyId: string;
  onDetails: (alert: EmailAlert) => void;
  onEdit: (alert: EmailAlert) => void;
  onToggle: (alert: EmailAlert) => void;
  onTest: (alert: EmailAlert) => void;
  onDelete: (alert: EmailAlert) => void;
}

function CompactValue({ values, fallback }: { values: string[]; fallback: string }) {
  if (!values.length) return <span className="text-zinc-600">{fallback}</span>;
  return <span className="inline-flex min-w-0 items-center gap-1.5" title={values.join(", ")}><span className="truncate">{values[0]}</span>{values.length > 1 && <span className="shrink-0 rounded bg-zinc-800 px-1.5 py-0.5 text-[9px] text-zinc-400">+{values.length - 1}</span>}</span>;
}

function Actions({ alert, busy, props }: { alert: EmailAlert; busy: boolean; props: Props }) {
  return <div className="flex justify-end gap-0.5"><IconButton label="Ver detalhes" className="size-7" disabled={busy} onClick={() => props.onDetails(alert)}><Eye className="size-3.5" /></IconButton><IconButton label="Editar alerta" className="size-7" disabled={busy} onClick={() => props.onEdit(alert)}><Edit3 className="size-3.5" /></IconButton><IconButton label={alert.enabled ? "Desativar alerta" : "Ativar alerta"} className="size-7" disabled={busy} onClick={() => props.onToggle(alert)}><Power className="size-3.5" /></IconButton><IconButton label="Enviar teste" className="size-7" disabled={busy} onClick={() => props.onTest(alert)}><Send className="size-3.5" /></IconButton><IconButton label="Excluir alerta" className="size-7 text-red-400" disabled={busy} onClick={() => props.onDelete(alert)}><Trash2 className="size-3.5" /></IconButton></div>;
}

export function CompactAlertTable(props: Props) {
  return (
    <div className="min-h-52">
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[960px] table-fixed text-left text-xs">
          <thead className="bg-[#1c1c1f] text-zinc-500"><tr className="h-10 border-b border-zinc-800"><th className="w-24 px-3 font-medium">Status</th><th className="w-[20%] px-3 font-medium">Regra</th><th className="w-[18%] px-3 font-medium">Senders</th><th className="w-[17%] px-3 font-medium">Níveis</th><th className="w-[18%] px-3 font-medium">Destinatários</th><th className="w-28 px-3 font-medium">Último envio</th><th className="w-40 px-2 text-right font-medium">Ações</th></tr></thead>
          <tbody className="divide-y divide-zinc-800/80">
            {props.items.map((alert) => <tr key={alert.id} className="h-12 hover:bg-zinc-900/50"><td className="px-3"><AlertStatusBadge enabled={alert.enabled} /></td><td className="px-3"><button type="button" onClick={() => props.onDetails(alert)} className="block max-w-full truncate font-medium text-zinc-200 hover:text-white hover:underline" title={alert.name}>{alert.name}</button></td><td className="min-w-0 px-3 text-zinc-400"><CompactValue values={alert.sender_names.length ? alert.sender_names : alert.sender_ids} fallback="Sem senders" /></td><td className="px-3"><SeverityBadges values={alert.severities} /></td><td className="min-w-0 px-3 text-zinc-400"><CompactValue values={alert.recipients} fallback="—" /></td><td className="px-3 text-zinc-500" title={alert.last_delivery_at ?? undefined}>{alert.last_delivery_at ? relativeDate(alert.last_delivery_at) : "Nunca"}</td><td className="px-2"><Actions alert={alert} busy={props.busyId === alert.id} props={props} /></td></tr>)}
          </tbody>
        </table>
      </div>
      <div className="divide-y divide-zinc-800 md:hidden">{props.items.map((alert) => <article key={alert.id} className="p-3"><div className="flex items-start justify-between gap-3"><button type="button" onClick={() => props.onDetails(alert)} className="min-w-0 truncate text-left text-sm font-medium text-zinc-200">{alert.name}</button><AlertStatusBadge enabled={alert.enabled} /></div><div className="mt-2 grid grid-cols-2 gap-2 text-[10px] text-zinc-500"><p className="truncate"><CompactValue values={alert.sender_names.length ? alert.sender_names : alert.sender_ids} fallback="Sem senders" /></p><p className="truncate"><CompactValue values={alert.recipients} fallback="—" /></p><SeverityBadges values={alert.severities} /><p className="text-right">{alert.last_delivery_at ? relativeDate(alert.last_delivery_at) : "Nunca enviado"}</p></div><div className="mt-2 border-t border-zinc-800 pt-2"><Actions alert={alert} busy={props.busyId === alert.id} props={props} /></div></article>)}</div>
    </div>
  );
}

export const AlertTable = CompactAlertTable;
