import {
  AlertTriangle,
  Check,
  CheckCircle2,
  Clipboard,
  Copy,
  KeyRound,
  Plus,
  RefreshCw,
  ShieldOff,
  Trash2,
} from "lucide-react";
import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { api } from "../../api";
import { queryClient } from "../../api/queryClient";
import { APIError, type Sender, type SenderCredentials, type SenderDependencies } from "../../types/api";
import { normalizeSenderID } from "../../utils/senderID";
import { CONTROL_SURFACE } from "../controlStyles";
import { Button, Input, ModalCloseButton } from "../ui";

export function CreateSenderButton({ onClick, compact = false }: { onClick: () => void; compact?: boolean }) {
  return (
    <Button onClick={onClick}>
      <Plus className="size-4" />
      <span className={compact ? "sr-only sm:not-sr-only" : ""}>Novo sender</span>
    </Button>
  );
}

export function SenderIDPreview({ id }: { id: string }) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950/70 px-3 py-2.5">
      <p className="text-[10px] uppercase tracking-wide text-zinc-600">Identificador imutável</p>
      <code className={`mt-1 block truncate text-xs ${id ? "text-zinc-200" : "text-zinc-600"}`}>
        {id || "O identificador aparecerá aqui"}
      </code>
    </div>
  );
}

export function SenderAvailabilityIndicator({ state }: { state: "idle" | "checking" | "available" | "unavailable" }) {
  if (state === "idle") return null;
  const content = {
    checking: [<RefreshCw key="icon" className="size-3 animate-spin" />, "Verificando disponibilidade..."],
    available: [<CheckCircle2 key="icon" className="size-3" />, "Identificador disponível"],
    unavailable: [<AlertTriangle key="icon" className="size-3" />, "Este identificador já está em uso. Escolha outro nome."],
  }[state];
  return <p role="status" className={`mt-1.5 flex min-h-4 items-center gap-1.5 text-[10px] ${state === "available" ? "text-emerald-400" : state === "unavailable" ? "text-amber-400" : "text-zinc-500"}`}>{content}</p>;
}

function ModalFrame({ title, description, children, footer, onClose, closeDisabled = false, width = "max-w-xl" }: { title: string; description?: string; children: React.ReactNode; footer?: React.ReactNode; onClose: () => void; closeDisabled?: boolean; width?: string }) {
  const titleID = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  const closeDisabledRef = useRef(closeDisabled);
  useEffect(() => { closeRef.current = onClose; closeDisabledRef.current = closeDisabled; }, [closeDisabled, onClose]);
  useEffect(() => {
    dialog.current?.focus();
    const close = (event: KeyboardEvent) => { if (event.key === "Escape" && !closeDisabledRef.current) closeRef.current(); };
    document.addEventListener("keydown", close);
    return () => document.removeEventListener("keydown", close);
  }, []);
  return createPortal(
    <div className="fixed inset-0 z-[220] grid place-items-center p-3 sm:p-5">
      <button type="button" aria-label="Fechar janela" className="absolute inset-0 bg-black/80" onClick={() => !closeDisabled && onClose()} />
      <div ref={dialog} role="dialog" aria-modal="true" aria-labelledby={titleID} tabIndex={-1} className={`relative flex max-h-[94dvh] w-full ${width} flex-col overflow-hidden rounded-xl border border-zinc-700 bg-[#111113] shadow-2xl shadow-black/70 outline-none`}>
        <header className="flex shrink-0 items-start justify-between gap-4 border-b border-zinc-800 px-5 py-4">
          <div><h2 id={titleID} className="text-base font-semibold text-zinc-100">{title}</h2>{description && <p className="mt-1 text-xs leading-5 text-zinc-500">{description}</p>}</div>
          <ModalCloseButton label={`Fechar ${title.toLocaleLowerCase("pt-BR")}`} disabled={closeDisabled} onClick={onClose} />
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">{children}</div>
        {footer && <footer className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-zinc-800 bg-zinc-950 px-5 py-3">{footer}</footer>}
      </div>
    </div>,
    document.body,
  );
}

interface CredentialsState { sender: Pick<Sender, "id" | "name" | "description">; credentials: SenderCredentials }

async function copyText(value: string) {
  await navigator.clipboard.writeText(value);
}

export function SenderKeyField({ value, onCopied }: { value: string; onCopied: () => void }) {
  return (
    <div className="flex min-w-0 items-center gap-2 rounded-lg border border-zinc-700 bg-black p-2">
      <code className="min-w-0 flex-1 select-all break-all px-1 text-xs text-zinc-100">{value}</code>
      <Button aria-label="Copiar chave" className="size-8 px-0" onClick={() => void copyText(value).then(onCopied)}><Copy className="size-3.5" /></Button>
    </div>
  );
}

export function SenderCredentialsDialog({ value, onClose }: { value: CredentialsState; onClose: () => void }) {
  const [keyCopied, setKeyCopied] = useState(false);
  const [confirmClose, setConfirmClose] = useState(false);
  const baseURL = window.location.origin;
  const environment = `LOG_API_URL=${baseURL}\nLOG_SENDER_ID=${value.sender.id}\nLOG_SENDER_KEY=${value.credentials.sender_key}`;
  const goExample = `client := LogClient{\n  BaseURL: ${JSON.stringify(baseURL)},\n  SenderID: ${JSON.stringify(value.sender.id)},\n  SenderKey: os.Getenv("LOG_SENDER_KEY"),\n}`;
  const requestClose = useCallback(() => keyCopied ? onClose() : setConfirmClose(true), [keyCopied, onClose]);

  return (
    <>
      <ModalFrame title="Sender criado" description="O sender foi criado com sucesso. Copie a chave agora, pois ela não poderá ser exibida novamente." onClose={requestClose} width="max-w-2xl" footer={<Button onClick={requestClose} className="border-zinc-500 bg-zinc-800 text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700">Concluir</Button>}>
        <div className="space-y-4">
          <div className="rounded-xl border border-amber-900/60 bg-amber-950/20 p-3 text-xs leading-5 text-amber-300"><div className="flex gap-2"><AlertTriangle className="mt-0.5 size-4 shrink-0" /><p><strong>Exibição única.</strong> Armazene a chave em uma variável de ambiente ou secret. O LogHill não poderá recuperá-la depois que esta janela for fechada.</p></div></div>
          <dl className="grid gap-3 sm:grid-cols-2">
            <div><dt className="text-[10px] uppercase tracking-wide text-zinc-600">Nome</dt><dd className="mt-1 text-sm text-zinc-200">{value.sender.name}</dd></div>
            <div><dt className="text-[10px] uppercase tracking-wide text-zinc-600">Descrição</dt><dd className="mt-1 text-sm text-zinc-400">{value.sender.description || "—"}</dd></div>
          </dl>
          <div><div className="mb-1.5 flex items-center justify-between"><p className="text-xs font-medium text-zinc-300">Sender ID</p><button type="button" onClick={() => void copyText(value.sender.id)} className="text-[11px] text-zinc-500 hover:text-zinc-200">Copiar ID</button></div><div className="rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-xs text-zinc-300">{value.sender.id}</div></div>
          <div><p className="mb-1.5 text-xs font-medium text-zinc-300">Sender Key</p><SenderKeyField value={value.credentials.sender_key} onCopied={() => setKeyCopied(true)} />{keyCopied && <p className="mt-1.5 flex items-center gap-1 text-[10px] text-emerald-400"><Check className="size-3" />Chave copiada</p>}</div>
          <div className="grid gap-2 sm:grid-cols-2"><Button onClick={() => void copyText(environment).then(() => setKeyCopied(true))}><Clipboard className="size-4" />Copiar configuração de ambiente</Button><Button onClick={() => void copyText(goExample).then(() => setKeyCopied(true))}><Copy className="size-4" />Copiar exemplo em Go</Button></div>
        </div>
      </ModalFrame>
      {confirmClose && <ModalFrame title="Você ainda não copiou a chave" description="Após fechar esta janela, a chave não poderá ser recuperada. Será necessário gerar uma nova chave." onClose={() => setConfirmClose(false)} width="max-w-md" footer={<><Button onClick={() => setConfirmClose(false)}>Continuar nesta janela</Button><Button onClick={onClose} className="border-red-900 text-red-300 hover:bg-red-950">Fechar mesmo assim</Button></>}><p className="text-sm leading-6 text-zinc-400">Copie a chave e guarde-a em um cofre de segredos antes de continuar.</p></ModalFrame>}
    </>
  );
}

export function CreateSenderDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (sender: Sender) => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [availability, setAvailability] = useState<"idle" | "checking" | "available" | "unavailable">("idle");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [credentials, setCredentials] = useState<CredentialsState>();
  const id = useMemo(() => normalizeSenderID(name), [name]);

  useEffect(() => {
    if (!open || name.trim().length < 3 || !id) { setAvailability("idle"); return; }
    const controller = new AbortController();
    setAvailability("checking");
    const timer = window.setTimeout(() => {
      void api.checkSenderID(id, controller.signal)
        .then((response) => setAvailability(response.available ? "available" : "unavailable"))
        .catch((requestError) => { if (!(requestError instanceof DOMException && requestError.name === "AbortError")) setAvailability("idle"); });
    }, 350);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [id, name, open]);

  const reset = () => { setName(""); setDescription(""); setAvailability("idle"); setError(""); setCredentials(undefined); };
  const close = () => { if (saving) return; reset(); onClose(); };
  const create = async () => {
    if (saving || name.trim().length < 3 || name.trim().length > 80 || description.length > 250 || availability === "unavailable") return;
    setSaving(true); setError("");
    try {
      const response = await api.createSender({ name, description: description.trim() || undefined });
      onCreated(response.sender);
      setCredentials(response);
    } catch (requestError) {
      if (requestError instanceof APIError && requestError.code === "SENDER_ALREADY_EXISTS") setAvailability("unavailable");
      setError(requestError instanceof Error ? requestError.message : "Não foi possível criar o sender.");
    } finally { setSaving(false); }
  };
  if (!open && !credentials) return null;
  if (credentials) return <SenderCredentialsDialog value={credentials} onClose={close} />;
  return (
    <ModalFrame title="Novo sender" description="Cadastre a origem antes de configurar o cliente que enviará os logs." onClose={close} closeDisabled={saving} footer={<><Button onClick={close} disabled={saving}>Cancelar</Button><Button onClick={() => void create()} disabled={saving || availability === "unavailable" || name.trim().length < 3 || name.trim().length > 80} className="border-zinc-500 bg-zinc-800 text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700"><Plus className="size-4" />{saving ? "Criando..." : "Criar sender"}</Button></>}>
      <div className="space-y-4">
        <label className="block text-xs font-medium text-zinc-300">Nome do sender<Input autoFocus value={name} disabled={saving} minLength={3} maxLength={80} onChange={(event) => { setName(event.target.value); setError(""); }} placeholder="Automação Financeira" className="mt-2 w-full" /><span className="mt-1.5 block text-[10px] text-zinc-600">Mínimo de 3 caracteres.</span></label>
        <SenderIDPreview id={id} />
        <SenderAvailabilityIndicator state={availability} />
        <label className="block text-xs font-medium text-zinc-300">Descrição <span className="font-normal text-zinc-600">(opcional)</span><textarea value={description} disabled={saving} maxLength={250} onChange={(event) => setDescription(event.target.value)} placeholder="Processamento de boletos e acordos" className={`mt-2 h-24 w-full resize-none rounded-lg border px-3 py-2 text-sm text-zinc-100 placeholder:text-zinc-600 ${CONTROL_SURFACE}`} /><span className="mt-1 block text-right text-[10px] text-zinc-600">{description.length}/250</span></label>
        {error && <p role="alert" className="rounded-lg border border-red-950 bg-red-950/20 px-3 py-2 text-xs text-red-300">{error}</p>}
      </div>
    </ModalFrame>
  );
}

export type SenderAction = "edit" | "rotate" | "revoke" | "reactivate" | "delete";

export function SenderActionDialogs({ sender, action, onClose, onChanged, onDeleted }: { sender?: Sender; action?: SenderAction; onClose: () => void; onChanged: (sender: Sender) => void; onDeleted: (id: string) => void }) {
  const [name, setName] = useState(sender?.name ?? "");
  const [description, setDescription] = useState(sender?.description ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [dependencies, setDependencies] = useState<SenderDependencies>();
  const [credentials, setCredentials] = useState<CredentialsState>();
  useEffect(() => { setName(sender?.name ?? ""); setDescription(sender?.description ?? ""); setError(""); if (action !== "delete") setDependencies(undefined); }, [action, sender]);
  useEffect(() => {
    if (!sender || action !== "delete") return;
    const key = ["modal", "sender-dependencies", sender.id] as const;
    const cached = queryClient.getQueryData<SenderDependencies>(key);
    if (cached) { setDependencies(cached); return; }
    let active = true;
    setDependencies(undefined);
    void api.senderDependencies(sender.id)
      .then((value) => { if (active) { queryClient.setQueryData(key, value); setDependencies(value); } })
      .catch(() => { if (active) setDependencies({ sender_id: sender.id, alert_rules: 0, events: 0, monitoring_rules: 0 }); });
    return () => { active = false; };
  }, [action, sender]);
  if (credentials) return <SenderCredentialsDialog value={credentials} onClose={() => { setCredentials(undefined); onClose(); }} />;
  if (!sender || !action) return null;

  const execute = async () => {
    setBusy(true); setError("");
    try {
      if (action === "edit") onChanged(await api.updateSender(sender.id, { name, description }));
      if (action === "rotate") { const response = await api.rotateSenderKey(sender.id); setCredentials({ sender, credentials: response.credentials }); }
      if (action === "revoke") onChanged(await api.revokeSender(sender.id));
      if (action === "reactivate") { const response = await api.reactivateSender(sender.id); onChanged(response.sender); setCredentials(response); }
      if (action === "delete") { await api.deleteSender(sender.id, Boolean(dependencies?.alert_rules), Boolean(dependencies?.events), Boolean(dependencies?.monitoring_rules)); onDeleted(sender.id); }
      if (action !== "rotate" && action !== "reactivate") onClose();
    } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Não foi possível concluir a operação."); }
    finally { setBusy(false); }
  };
  const titles = { edit: "Editar informações", rotate: "Gerar nova chave", revoke: "Revogar acesso", reactivate: "Reativar sender", delete: "Excluir sender" };
  const labels = { edit: "Salvar alterações", rotate: "Gerar nova chave", revoke: "Revogar acesso", reactivate: "Reativar e gerar chave", delete: "Excluir sender" };
  return (
    <ModalFrame title={titles[action]} description={action === "edit" ? "O identificador não pode ser alterado após a criação." : undefined} onClose={onClose} closeDisabled={busy} width="max-w-lg" footer={<><Button onClick={onClose} disabled={busy}>Cancelar</Button><Button onClick={() => void execute()} disabled={busy || (action === "edit" && name.trim().length < 3)} className={action === "delete" || action === "revoke" ? "border-red-900 text-red-300 hover:bg-red-950" : "border-zinc-500 bg-zinc-800 text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700"}>{action === "rotate" && <KeyRound className="size-4" />}{action === "revoke" && <ShieldOff className="size-4" />}{action === "delete" && <Trash2 className="size-4" />}{busy ? "Processando..." : labels[action]}</Button></>}>
      {action === "edit" ? <div className="space-y-4"><label className="block text-xs text-zinc-300">Identificador<Input value={sender.id} readOnly aria-readonly className="mt-2 w-full cursor-not-allowed text-zinc-500" /></label><label className="block text-xs text-zinc-300">Nome de exibição<Input value={name} minLength={3} maxLength={80} onChange={(event) => setName(event.target.value)} className="mt-2 w-full" /><span className="mt-1.5 block text-[10px] text-zinc-600">Mínimo de 3 caracteres.</span></label><label className="block text-xs text-zinc-300">Descrição<textarea value={description} maxLength={250} onChange={(event) => setDescription(event.target.value)} className={`mt-2 h-24 w-full resize-none rounded-lg border p-3 text-sm text-zinc-100 ${CONTROL_SURFACE}`} /></label></div> : <div className="space-y-3 text-sm leading-6 text-zinc-400"><p>{action === "rotate" && "A chave atual deixará de funcionar imediatamente. Atualize todas as aplicações que utilizam este sender."}{action === "revoke" && "A chave atual será invalidada. Logs e healthchecks serão recusados, mas os dados e regras existentes serão preservados."}{action === "reactivate" && "Uma nova chave será gerada e exibida uma única vez. A chave anterior não será restaurada."}{action === "delete" && dependencies && (dependencies.alert_rules || dependencies.events || dependencies.monitoring_rules) ? `Este sender está associado a ${dependencies.alert_rules} regra(s) de alerta, ${dependencies.events} evento(s) e ${dependencies.monitoring_rules} regra(s) de monitoramento. Ao continuar, ele será removido dessas configurações; as que ficarem sem senders serão desativadas.` : action === "delete" ? "Os logs e os dados deste sender serão excluídos permanentemente." : ""}</p><code className="block rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-xs text-zinc-300">{sender.id}</code></div>}
      {error && <p role="alert" className="mt-4 rounded-lg border border-red-950 bg-red-950/20 px-3 py-2 text-xs text-red-300">{error}</p>}
    </ModalFrame>
  );
}

export const RotateSenderKeyDialog = SenderActionDialogs;
export const RevokeSenderDialog = SenderActionDialogs;
export const ReactivateSenderDialog = SenderActionDialogs;
