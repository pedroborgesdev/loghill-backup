import { Bell, Check, RefreshCw, Zap } from "lucide-react";
import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { alertsApi } from "../../api/alerts";
import { eventsApi } from "../../api/events";
import type { EmailAlert } from "../../types/alert";
import type { EventDefinition } from "../../types/event";
import { waitForMinimumLoading } from "../../utils/minimumLoading";
import { Button, ErrorAlert, ModalCloseButton, Skeleton } from "../ui";
import { getSenderAssociations, invalidateSenderAssociations, type AssociationItem } from "./senderAssociationsData";

export type AssociationKind = "alerts" | "events";

export function SenderAssociationsDialog({
  kind,
  senderId,
  senderAvailable,
  onClose,
}: {
  kind?: AssociationKind;
  senderId: string;
  senderAvailable: boolean;
  onClose: () => void;
}) {
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const [items, setItems] = useState<AssociationItem[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [original, setOriginal] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [loadedKind, setLoadedKind] = useState<AssociationKind>();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!kind) return;
    const startedAt = performance.now();
    setLoading(true);
    setError("");
    try {
      const nextItems = await getSenderAssociations(kind);
      await waitForMinimumLoading(startedAt);
      const associated = new Set(nextItems.filter((item) => item.senderIds.includes(senderId)).map((item) => item.id));
      setItems(nextItems);
      setSelected(associated);
      setOriginal(new Set(associated));
      setLoadedKind(kind);
    } catch (requestError) {
      await waitForMinimumLoading(startedAt);
      setError(requestError instanceof Error ? requestError.message : "Não foi possível carregar as associações.");
      setLoadedKind(kind);
    } finally {
      setLoading(false);
    }
  }, [kind, senderId]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => { if (!kind) setLoadedKind(undefined); }, [kind]);
  useEffect(() => {
    if (!kind) return;
    dialog.current?.focus();
    const keydown = (event: KeyboardEvent) => { if (event.key === "Escape" && !saving) onClose(); };
    document.addEventListener("keydown", keydown);
    return () => document.removeEventListener("keydown", keydown);
  }, [kind, onClose, saving]);

  const changed = useMemo(
    () => items.filter((item) => selected.has(item.id) !== original.has(item.id)),
    [items, original, selected],
  );

  if (!kind) return null;
  const isAlerts = kind === "alerts";
  const label = isAlerts ? "Alertas" : "Eventos";

  const toggle = (item: AssociationItem) => {
    if (!selected.has(item.id) && !senderAvailable) return;
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(item.id)) next.delete(item.id); else next.add(item.id);
      return next;
    });
  };

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      for (const item of changed) {
        const senderIds = selected.has(item.id)
          ? [...item.senderIds.filter((id) => id !== senderId), senderId]
          : item.senderIds.filter((id) => id !== senderId);
        if (kind === "alerts") {
          const alert = item.source as EmailAlert;
          await alertsApi.update(alert.id, {
            name: alert.name, sender_ids: senderIds, severities: alert.severities,
            recipients: alert.recipients, provider: alert.provider, enabled: alert.enabled,
          });
        } else {
          const event = item.source as EventDefinition;
          await eventsApi.update(event.id, {
            name: event.name, key: event.key, sender_ids: senderIds, action_type: event.action_type,
            recipients: event.recipients, subject_template: event.subject_template,
            message_template: event.message_template, enabled: event.enabled,
          });
        }
      }
      await invalidateSenderAssociations(kind);
      onClose();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Não foi possível salvar as associações.");
    } finally {
      setSaving(false);
    }
  };

  return createPortal(
    <div className="fixed inset-0 z-[220] grid place-items-center p-3 sm:p-5">
      <button type="button" aria-label="Fechar associações" className="absolute inset-0 bg-black/75" onClick={() => !saving && onClose()} />
      <div ref={dialog} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1} className="relative flex max-h-[85dvh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-zinc-700 bg-[#161618] shadow-2xl shadow-black/70 outline-none">
        <header className="flex items-start justify-between gap-3 border-b border-zinc-800 p-5">
          <div>
            <div className="flex items-center gap-2">{isAlerts ? <Bell className="size-4 text-amber-300/80" /> : <Zap className="size-4 text-amber-300/80" />}<h2 id={titleId} className="font-semibold text-zinc-100">{label} do sender</h2></div>
            <p className="mt-1 text-xs text-zinc-500">Marque para associar e desmarque para remover este sender.</p>
          </div>
          <ModalCloseButton label={`Fechar ${kind === "alerts" ? "alertas" : "eventos"} do sender`} disabled={saving} onClick={onClose} />
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto p-3">
          {error && <div className="mb-3"><ErrorAlert message={error} onRetry={() => void load()} /></div>}
          {loading || loadedKind !== kind ? <div className="space-y-2">{Array.from({ length: 5 }, (_, index) => <Skeleton key={index} className="h-14 w-full" />)}</div> : items.length ? (
            <div className="space-y-1.5">{items.map((item) => {
              const checked = selected.has(item.id);
              const locked = !checked && !senderAvailable;
              return <button key={item.id} type="button" role="checkbox" aria-checked={checked} disabled={saving || locked} onClick={() => toggle(item)} className="flex w-full items-center gap-3 rounded-lg border border-zinc-800 bg-[#1c1c1f] px-3 py-2.5 text-left transition-colors hover:border-zinc-600 disabled:cursor-not-allowed disabled:opacity-55">
                <span className={`grid size-4 shrink-0 place-items-center rounded border ${checked ? "border-sky-400/70 bg-sky-950/40 text-sky-400" : "border-zinc-700 text-transparent"}`}><Check className="size-3" /></span>
                <span className="min-w-0 flex-1"><span className="block truncate text-xs font-medium text-zinc-200">{item.name}</span><span className="mt-0.5 block truncate font-mono text-[10px] text-zinc-600">{item.description}</span></span>
                <span className={`text-[10px] ${item.enabled ? "text-emerald-500" : "text-zinc-600"}`}>{item.enabled ? "Ativo" : "Inativo"}</span>
              </button>;
            })}</div>
          ) : <div className="grid min-h-40 place-items-center text-center"><div><p className="text-sm text-zinc-300">Nenhum {isAlerts ? "alerta" : "evento"} configurado</p><p className="mt-1 text-xs text-zinc-600">Crie um na seção de {label.toLowerCase()} para associá-lo aqui.</p></div></div>}
        </div>
        <footer className="flex items-center justify-between gap-3 border-t border-zinc-800 bg-zinc-950 px-5 py-3">
          <span className="text-[10px] text-zinc-600">{changed.length} alteração{changed.length === 1 ? "" : "ões"}</span>
          <div className="flex gap-2"><Button onClick={onClose} disabled={saving}>Cancelar</Button><Button onClick={() => void save()} disabled={saving || loading || !changed.length}>{saving && <RefreshCw className="size-4 animate-spin" />}{saving ? "Salvando..." : "Salvar associações"}</Button></div>
        </footer>
      </div>
    </div>,
    document.body,
  );
}
