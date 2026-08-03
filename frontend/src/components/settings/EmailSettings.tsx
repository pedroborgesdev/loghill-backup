import { AlertTriangle, CheckCircle2, RefreshCw, Send } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { emailSettingsApi } from "../../api/emailSettings";
import { useCachedState } from "../../hooks/useCachedState";
import type { EmailSettings as EmailSettingsValue } from "../../types/email";
import { waitForMinimumLoading } from "../../utils/minimumLoading";
import { Button, Input, Skeleton } from "../ui";
import { EmailProviderCard } from "./EmailProviderCard";
import { GmailSettingsForm, type GmailDraft } from "./GmailSettingsForm";
import { OutlookSettingsForm, type OutlookDraft } from "./OutlookSettingsForm";

type Draft = { provider: "outlook" | "gmail"; outlook: OutlookDraft; gmail: GmailDraft };

export function EmailSettings({ onDirtyChange }: { onDirtyChange?: (dirty: boolean) => void }) {
  const [settings, setSettings] = useCachedState<EmailSettingsValue>(["modal", "email-settings"]);
  const [draft, setDraft] = useState<Draft | undefined>(() => settings ? toDraft(settings) : undefined);
  const [loading, setLoading] = useState(!settings); const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false); const [sending, setSending] = useState(false);
  const [recipient, setRecipient] = useState(""); const [error, setError] = useState(""); const [notice, setNotice] = useState("");
  const hasLoaded = useRef(Boolean(settings));
  const load = useCallback(async () => { const first = !hasLoaded.current; const started = performance.now(); setLoading(first); setError(""); try { const value = await emailSettingsApi.get(); if (first) await waitForMinimumLoading(started); hasLoaded.current = true; setSettings(value); setDraft(toDraft(value)); } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Não foi possível carregar a configuração de e-mail."); } finally { setLoading(false); } }, [setSettings]);
  useEffect(() => { void load(); }, [load]);
  const dirty = useMemo(() => Boolean(settings && draft && JSON.stringify(clearSecrets(draft)) !== JSON.stringify(clearSecrets(toDraft(settings)))), [draft, settings]);
  const typedSecret = Boolean(draft?.outlook.client_secret || draft?.gmail.password);
  useEffect(() => onDirtyChange?.(dirty || typedSecret), [dirty, onDirtyChange, typedSecret]);
  if (loading && !settings) return <EmailSettingsSkeleton />;
  if (!settings || !draft) return <div role="alert" className="rounded-xl border border-red-950 bg-red-950/20 p-4 text-xs text-red-300">{error}<Button onClick={() => void load()} className="ml-3 h-8">Tentar novamente</Button></div>;
  const selectedManaged = draft.provider === "outlook" ? settings.outlook.managed_by_environment : settings.gmail.managed_by_environment;
  const providerSaved = draft.provider === settings.provider;
  const permissionError = /Mail\.Send|consentimento administr|não autorizou o envio|escopo permitido/i.test(error);
  const incomplete = draft.provider === "outlook"
    ? draft.outlook.enabled && (!draft.outlook.tenant_id.trim() || !draft.outlook.client_id.trim() || (!settings.outlook.client_secret_configured && !draft.outlook.client_secret) || !draft.outlook.sender_email.trim())
    : draft.gmail.enabled && (!draft.gmail.host.trim() || !draft.gmail.port || !draft.gmail.username.trim() || (!settings.gmail.password_configured && !draft.gmail.password) || !draft.gmail.from.trim());
  const save = async () => { if (saving || incomplete) return; setSaving(true); setError(""); setNotice(""); try { const value = await emailSettingsApi.update({ provider: draft.provider, enabled: draft.provider === "outlook" ? draft.outlook.enabled : draft.gmail.enabled, outlook: { tenant_id: draft.outlook.tenant_id.trim(), client_id: draft.outlook.client_id.trim(), ...(draft.outlook.client_secret ? { client_secret: draft.outlook.client_secret } : {}), sender_email: draft.outlook.sender_email.trim(), sender_name: draft.outlook.sender_name.trim() }, gmail: { host: draft.gmail.host.trim(), port: draft.gmail.port, username: draft.gmail.username.trim(), ...(draft.gmail.password ? { password: draft.gmail.password } : {}), from: draft.gmail.from.trim(), sender_name: draft.gmail.sender_name.trim() } }); setSettings(value); setDraft(toDraft(value)); setNotice("Configuração de e-mail salva."); } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Não foi possível salvar a configuração."); } finally { setSaving(false); } };
  const test = async () => { setTesting(true); setError(""); setNotice(""); try { const value = await emailSettingsApi.testConnection(); setNotice(value.message); await load(); } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Falha ao testar o provedor de e-mail."); } finally { setTesting(false); } };
  const send = async () => { if (!recipient.trim()) { setError("Informe o destinatário do teste."); return; } setSending(true); setError(""); setNotice(""); try { const value = await emailSettingsApi.sendTest(recipient); setNotice(value.message); } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Falha ao enviar o teste."); } finally { setSending(false); } };
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-2"><EmailProviderCard provider="outlook" selected={draft.provider === "outlook"} available onSelect={() => setDraft({ ...draft, provider: "outlook" })} /><EmailProviderCard provider="gmail" selected={draft.provider === "gmail"} available onSelect={() => setDraft({ ...draft, provider: "gmail" })} /></div>
    {draft.provider === "outlook" ? <OutlookSettingsForm value={draft.outlook} secretConfigured={settings.outlook.client_secret_configured} managed={settings.outlook.managed_by_environment} disabled={saving} onChange={(outlook) => { setDraft({ ...draft, outlook }); setError(""); }} /> : <GmailSettingsForm value={draft.gmail} passwordConfigured={settings.gmail.password_configured} managed={settings.gmail.managed_by_environment} disabled={saving} onChange={(gmail) => { setDraft({ ...draft, gmail }); setError(""); }} />}
    {incomplete && <div className="flex gap-2 rounded-lg border border-amber-950 bg-amber-950/20 p-3 text-[11px] text-amber-300"><AlertTriangle className="size-4 shrink-0" />Complete todos os campos e a credencial antes de habilitar o envio.</div>}
    <div className="rounded-xl border border-zinc-800 bg-zinc-950/55 p-4"><p className="text-xs font-medium text-zinc-300">Validação e envio de teste</p><p className="mt-1 text-[10px] leading-5 text-zinc-600">Valida a conexão do provedor selecionado. Para Gmail, use uma senha de aplicativo e SMTP com STARTTLS.</p><div className="mt-3 flex flex-col gap-2 sm:flex-row"><Button onClick={() => void test()} disabled={testing || saving || !providerSaved || !settings.configured}>{testing ? <RefreshCw className="size-4 animate-spin" /> : <CheckCircle2 className="size-4" />}{testing ? "Testando..." : "Testar conexão"}</Button><Input type="email" value={recipient} onChange={(e) => setRecipient(e.target.value)} placeholder="Destinatário do teste" className="min-w-0 flex-1" /><Button onClick={() => void send()} disabled={sending || saving || !providerSaved || !settings.enabled || !settings.configured}><Send className="size-4" />{sending ? "Enviando..." : "Enviar e-mail de teste"}</Button></div></div>
    {error && (permissionError ? <div role="alert" className="rounded-lg border border-amber-900/70 bg-amber-950/20 px-3 py-3 text-xs text-amber-200"><p className="font-medium">Permissão de envio necessária</p><p className="mt-1">{error}</p><p className="mt-2 text-zinc-400">No Microsoft Entra, adicione <strong>Microsoft Graph &gt; Application permissions &gt; Mail.Send</strong> e conceda o consentimento do administrador.</p></div> : <p role="alert" className="rounded-lg border border-red-950 bg-red-950/20 px-3 py-2 text-xs text-red-300">{error}</p>)}{notice && <p role="status" className="rounded-lg border border-emerald-950 bg-emerald-950/20 px-3 py-2 text-xs text-emerald-300">{notice}</p>}
    {!selectedManaged && <div className="flex justify-end gap-2"><Button disabled={!dirty && !typedSecret} onClick={() => setDraft(toDraft(settings))} className="border-transparent bg-transparent">Desfazer</Button><Button disabled={saving || incomplete || (!dirty && !typedSecret)} onClick={() => void save()} className="border-zinc-600 bg-zinc-800">{saving ? "Salvando..." : "Salvar configuração de e-mail"}</Button></div>}
  </div>;
}

function EmailSettingsSkeleton() { return <div aria-label="Carregando configuração de e-mail" role="status" className="space-y-4"><div className="grid gap-3 sm:grid-cols-2"><Skeleton className="h-24 w-full rounded-xl" /><Skeleton className="h-24 w-full rounded-xl" /></div><div className="space-y-4 rounded-xl border border-zinc-800 p-4"><div className="flex items-center justify-between"><div className="space-y-2"><Skeleton className="h-3 w-44" /><Skeleton className="h-2.5 w-64" /></div><Skeleton className="h-6 w-11 rounded-full" /></div><div className="grid gap-3 sm:grid-cols-2"><Skeleton className="h-14 w-full" /><Skeleton className="h-14 w-full" /></div><Skeleton className="h-14 w-full" /><div className="grid gap-3 sm:grid-cols-2"><Skeleton className="h-14 w-full" /><Skeleton className="h-14 w-full" /></div></div><Skeleton className="h-28 w-full rounded-xl" /></div>; }
function toDraft(value: EmailSettingsValue): Draft {
  const gmail = value.gmail ?? { host: "smtp.gmail.com", port: 587, username: "", from: "", sender_name: "LogHill" };
  return { provider: value.provider, outlook: { enabled: value.provider === "outlook" && value.enabled, tenant_id: value.outlook.tenant_id, client_id: value.outlook.client_id, client_secret: "", sender_email: value.outlook.sender_email, sender_name: value.outlook.sender_name }, gmail: { enabled: value.provider === "gmail" && value.enabled, host: gmail.host || "smtp.gmail.com", port: gmail.port || 587, username: gmail.username, password: "", from: gmail.from, sender_name: gmail.sender_name || "LogHill" } };
}
function clearSecrets(value: Draft) { return { ...value, outlook: { ...value.outlook, client_secret: "" }, gmail: { ...value.gmail, password: "" } }; }
