import { AlertTriangle, Check } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { alertsApi } from "../../api/alerts";
import { APIError, type LogSeverity } from "../../types/api";
import type { AlertInput, EmailAlert } from "../../types/alert";
import type { SenderOption } from "./SenderSelect";
import { RecipientInput } from "./RecipientInput";
import { SenderMultiSelect } from "./SenderSelect";
import { SeveritySelector } from "./SeveritySelector";
import { Button, Input, ModalCloseButton } from "../ui";

interface Props {
  alert?: EmailAlert;
  outlookReady: boolean;
  onSaved: (alert: EmailAlert) => void;
  onClose: () => void;
  onConfigureOutlook: (trigger: HTMLButtonElement) => void;
}

export function AlertFormDialog({ alert, outlookReady, onSaved, onClose, onConfigureOutlook }: Props) {
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const [name, setName] = useState(alert?.name ?? "");
  const [senders, setSenders] = useState<SenderOption[]>(alert ? alert.sender_ids.map((id, index) => ({ id, name: alert.sender_names[index] ?? id, status: "online" })) : []);
  const [severities, setSeverities] = useState<LogSeverity[]>(alert?.severities ?? ["ERROR", "FATAL"]);
  const [recipients, setRecipients] = useState<string[]>(alert?.recipients ?? []);
  const [enabled, setEnabled] = useState(alert?.enabled ?? true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [field, setField] = useState("");

  useEffect(() => {
    dialog.current?.focus();
    const keydown = (event: KeyboardEvent) => { if (event.key === "Escape" && !saving) onClose(); };
    document.addEventListener("keydown", keydown);
    return () => document.removeEventListener("keydown", keydown);
  }, [onClose, saving]);

  const validation = useMemo(() => {
    if (name.trim().length < 3 || name.trim().length > 100) return "O nome deve possuir entre 3 e 100 caracteres.";
    if (!severities.length) return "Selecione ao menos uma severidade.";
    if (!recipients.length) return "Adicione ao menos um destinatário.";
    if (enabled && !outlookReady) return "Configure e habilite um e-mail ou salve o alerta como inativo.";
    return "";
  }, [enabled, name, outlookReady, recipients.length, severities.length]);

  const save = async () => {
    if (validation || saving) { setError(validation); return; }
    const input: AlertInput = { name: name.trim(), sender_ids: senders.map((sender) => sender.id), severities, recipients, provider: "outlook", enabled };
    setSaving(true); setError(""); setField("");
    try {
      const saved = alert ? await alertsApi.update(alert.id, input) : await alertsApi.create(input);
      onSaved(saved);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Não foi possível salvar o alerta.");
      if (requestError instanceof APIError) setField(requestError.field ?? "");
    } finally { setSaving(false); }
  };

  return createPortal(
    <div className="fixed inset-0 z-[210] grid place-items-center p-3 sm:p-5">
      <button type="button" aria-label="Fechar formulário" className="absolute inset-0 bg-black/75" onClick={() => !saving && onClose()} />
      <div ref={dialog} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1} className="relative flex max-h-[92dvh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-zinc-700 bg-[#111113] shadow-2xl shadow-black/70 outline-none">
        <header className="flex shrink-0 items-start justify-between border-b border-zinc-800 px-5 py-4"><div><h2 id={titleId} className="text-base font-semibold text-zinc-100">{alert ? "Editar alerta" : "Novo alerta de e-mail"}</h2><p className="mt-1 text-xs text-zinc-500">Cada log correspondente cria uma notificação individual.</p></div><ModalCloseButton label="Fechar formulário de alerta" disabled={saving} onClick={onClose} /></header>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
          {!outlookReady && <div className="flex items-start gap-3 rounded-lg border border-amber-950 bg-amber-950/20 p-3"><AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-500" /><div className="text-xs leading-5 text-amber-300"><p>Nenhum e-mail está configurado e habilitado.</p><button type="button" onClick={(event) => onConfigureOutlook(event.currentTarget)} className="mt-1 font-medium underline underline-offset-2">Configurar e-mail</button></div></div>}
          <label className="block text-xs font-medium text-zinc-300">Nome do alerta<Input autoFocus disabled={saving} value={name} minLength={3} maxLength={100} onChange={(event) => { setName(event.target.value); setError(""); }} placeholder="Erros críticos da automação financeira" aria-invalid={field === "name"} className="mt-2 w-full" /><span className="mt-1.5 block text-[10px] text-zinc-600">Mínimo de 3 caracteres.</span></label>
          <SenderMultiSelect value={senders} onChange={setSenders} disabled={saving} />
          <SeveritySelector value={severities} onChange={setSeverities} disabled={saving} />
          <RecipientInput value={recipients} onChange={setRecipients} disabled={saving} />
          <div className="rounded-xl border border-zinc-800 bg-zinc-950/60 p-4"><div className="flex items-center gap-3"><div className="grid size-9 place-items-center rounded-lg border border-zinc-700 bg-zinc-900 text-sm">@</div><div><p className="text-sm font-medium text-zinc-200">E-mail</p><p className="text-[11px] text-zinc-600">Provider global selecionado</p></div><span className={`ml-auto text-[11px] ${outlookReady ? "text-emerald-400" : "text-amber-400"}`}>{outlookReady ? "Configurado" : "Não configurado"}</span></div></div>
          <button type="button" role="switch" aria-checked={enabled} disabled={saving} onClick={() => setEnabled((value) => !value)} className="flex w-full items-center justify-between rounded-xl border border-zinc-800 bg-zinc-950/60 p-4 text-left focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"><span><span className="block text-sm font-medium text-zinc-200">Alerta ativo</span><span className="mt-1 block text-[11px] text-zinc-600">Regras inativas permanecem salvas sem enviar mensagens.</span></span><span className={`relative h-6 w-11 rounded-full transition-colors ${enabled ? "bg-emerald-600" : "bg-zinc-700"}`}><span className={`absolute top-1 size-4 rounded-full bg-white transition-transform ${enabled ? "translate-x-6" : "translate-x-1"}`} /></span></button>
          {error && <p role="alert" className="rounded-lg border border-red-950 bg-red-950/20 px-3 py-2 text-xs text-red-300">{error}</p>}
        </div>
        <footer className="flex shrink-0 justify-end gap-2 border-t border-zinc-800 bg-zinc-950 px-5 py-3"><Button onClick={onClose} disabled={saving} className="border-transparent bg-transparent">Cancelar</Button><Button onClick={() => void save()} disabled={saving || Boolean(validation)} className="border-zinc-500 bg-zinc-800 text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700"><Check className="size-4" />{saving ? "Salvando..." : "Salvar alerta"}</Button></footer>
      </div>
    </div>, document.body,
  );
}
