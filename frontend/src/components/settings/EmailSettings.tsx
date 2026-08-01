import { AlertTriangle, CheckCircle2, RefreshCw, Send } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { emailSettingsApi } from "../../api/emailSettings";
import type { EmailSettings as EmailSettingsValue } from "../../types/email";
import { Button, Input, Skeleton } from "../ui";
import { EmailProviderCard } from "./EmailProviderCard";
import { OutlookSettingsForm, type OutlookDraft } from "./OutlookSettingsForm";

export function EmailSettings({ onDirtyChange }: { onDirtyChange?: (dirty: boolean) => void }) {
  const [settings, setSettings] = useState<EmailSettingsValue>();
  const [draft, setDraft] = useState<OutlookDraft>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [sending, setSending] = useState(false);
  const [recipient, setRecipient] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = useCallback(async () => {
    setLoading(true); setError("");
    try { const value = await emailSettingsApi.get(); setSettings(value); setDraft(toDraft(value)); }
    catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Não foi possível carregar a configuração de e-mail."); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  const dirty = useMemo(() => Boolean(settings && draft && JSON.stringify({ ...draft, client_secret: "" }) !== JSON.stringify(toDraft(settings))), [draft, settings]);
  useEffect(() => onDirtyChange?.(dirty || Boolean(draft?.client_secret)), [dirty, draft?.client_secret, onDirtyChange]);

  const save = async () => {
    if (!draft || saving) return;
    setSaving(true); setError(""); setNotice("");
    try {
      const value = await emailSettingsApi.update({ provider: "outlook", enabled: draft.enabled, outlook: { tenant_id: draft.tenant_id.trim(), client_id: draft.client_id.trim(), ...(draft.client_secret ? { client_secret: draft.client_secret } : {}), sender_email: draft.sender_email.trim(), sender_name: draft.sender_name.trim() } });
      setSettings(value); setDraft(toDraft(value)); setNotice("Configuração de e-mail salva.");
    } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Não foi possível salvar a configuração."); }
    finally { setSaving(false); }
  };
  const test = async () => { setTesting(true); setError(""); setNotice(""); try { const value=await emailSettingsApi.testConnection(); setNotice(value.message); await load(); } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Falha ao testar o Outlook."); } finally { setTesting(false); } };
  const send = async () => { if (!recipient.trim()) { setError("Informe o destinatário do teste."); return; } setSending(true); setError(""); setNotice(""); try { const value=await emailSettingsApi.sendTest(recipient); setNotice(value.message); } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Falha ao enviar o teste."); } finally { setSending(false); } };

  if (loading && !settings) return <div className="space-y-3"><Skeleton className="h-24 w-full" /><Skeleton className="h-72 w-full" /></div>;
  if (!settings || !draft) return <div role="alert" className="rounded-xl border border-red-950 bg-red-950/20 p-4 text-xs text-red-300">{error}<Button onClick={() => void load()} className="ml-3 h-8">Tentar novamente</Button></div>;
  const incomplete = draft.enabled && (!draft.tenant_id.trim() || !draft.client_id.trim() || (!settings.outlook.client_secret_configured && !draft.client_secret) || !draft.sender_email.trim());
  const permissionError = /Mail\.Send|consentimento administr|não autorizou o envio|escopo permitido/i.test(error);
  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2"><EmailProviderCard provider="outlook" selected available onSelect={() => undefined} /><EmailProviderCard provider="gmail" selected={false} available={false} onSelect={() => undefined} /></div>
      <OutlookSettingsForm value={draft} secretConfigured={settings.outlook.client_secret_configured} managed={settings.outlook.managed_by_environment} disabled={saving} onChange={(value) => { setDraft(value); setError(""); setNotice(""); }} />
      {incomplete && <div className="flex gap-2 rounded-lg border border-amber-950 bg-amber-950/20 p-3 text-[11px] text-amber-300"><AlertTriangle className="size-4 shrink-0" />Complete Tenant ID, Client ID, Client Secret e e-mail remetente para habilitar o Outlook.</div>}
      <div className="rounded-xl border border-zinc-800 bg-zinc-950/55 p-4"><p className="text-xs font-medium text-zinc-300">Validação</p><p className="mt-1 text-[10px] leading-5 text-zinc-600">O teste valida as credenciais e verifica a permissão Mail.Send quando ela estiver presente no token. O envio de teste também confirma a mailbox e o escopo do Exchange.</p><div className="mt-3 flex flex-col gap-2 sm:flex-row"><Button onClick={() => void test()} disabled={testing || saving || !settings.configured}>{testing ? <RefreshCw className="size-4 animate-spin" /> : <CheckCircle2 className="size-4" />}{testing ? "Testando..." : "Testar conexão"}</Button><Input type="email" value={recipient} onChange={(event) => setRecipient(event.target.value)} placeholder="Destinatário do teste" className="min-w-0 flex-1" /><Button onClick={() => void send()} disabled={sending || saving || !settings.enabled || !settings.configured}><Send className="size-4" />{sending ? "Enviando..." : "Enviar e-mail de teste"}</Button></div>{settings.last_test_at && <p className={`mt-2 text-[10px] ${settings.last_test_status === "success" ? "text-emerald-500" : "text-red-400"}`}>Último teste: {settings.last_test_status === "success" ? "conexão validada" : settings.last_test_error}</p>}</div>
      {error && (permissionError ? <div role="alert" className="rounded-lg border border-amber-900/70 bg-amber-950/20 px-3 py-3 text-xs text-amber-200"><div className="flex items-start gap-2"><AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-500" /><div><p className="font-medium">Permissão de envio necessária</p><p className="mt-1 leading-5 text-amber-200/80">{error}</p><p className="mt-2 leading-5 text-zinc-400">No Microsoft Entra, adicione <strong className="font-medium text-zinc-300">Microsoft Graph &gt; Application permissions &gt; Mail.Send</strong> e conceda o consentimento do administrador. Confirme também se o remetente possui mailbox no Exchange Online e está dentro do escopo permitido para o aplicativo.</p></div></div></div> : <p role="alert" className="rounded-lg border border-red-950 bg-red-950/20 px-3 py-2 text-xs text-red-300">{error}</p>)}{notice && <p role="status" className="rounded-lg border border-emerald-950 bg-emerald-950/20 px-3 py-2 text-xs text-emerald-300">{notice}</p>}
      {!settings.outlook.managed_by_environment && <div className="flex justify-end gap-2"><Button disabled={!dirty && !draft.client_secret} onClick={() => setDraft(toDraft(settings))} className="border-transparent bg-transparent">Desfazer</Button><Button disabled={saving || incomplete || (!dirty && !draft.client_secret)} onClick={() => void save()} className="border-zinc-600 bg-zinc-800">{saving ? "Salvando..." : "Salvar configuração de e-mail"}</Button></div>}
    </div>
  );
}

function toDraft(value: EmailSettingsValue): OutlookDraft { return { enabled: value.enabled, tenant_id: value.outlook.tenant_id, client_id: value.outlook.client_id, client_secret: "", sender_email: value.outlook.sender_email, sender_name: value.outlook.sender_name }; }
