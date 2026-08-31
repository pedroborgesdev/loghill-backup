import { Eye, EyeOff, KeyRound } from "lucide-react";
import { useState } from "react";
import { Input } from "../ui";

export interface OutlookDraft { tenant_id: string; client_id: string; client_secret: string; sender_email: string; sender_name: string; enabled: boolean }

export function OutlookSettingsForm({ value, secretConfigured, managed, disabled, onChange }: { value: OutlookDraft; secretConfigured: boolean; managed: boolean; disabled: boolean; onChange: (value: OutlookDraft) => void }) {
  const [showSecret, setShowSecret] = useState(false);
  const update = (field: keyof OutlookDraft, fieldValue: string | boolean) => onChange({ ...value, [field]: fieldValue });
  const locked = disabled || managed;
  return (
    <div className="space-y-4 rounded-xl border border-zinc-800 bg-zinc-950/55 p-4">
      {managed && <div className="flex items-center gap-2 rounded-lg border border-blue-950 bg-blue-950/20 px-3 py-2 text-[11px] text-blue-300"><KeyRound className="size-3.5" />Configuration managed through environment variables. Fields are locked, but tests remain available.</div>}
      <button type="button" role="switch" aria-checked={value.enabled} disabled={locked} onClick={() => update("enabled", !value.enabled)} className="flex w-full items-center justify-between rounded-lg text-left focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"><span><span className="block text-xs font-medium text-zinc-200">Enable Outlook delivery</span><span className="mt-1 block text-[10px] text-zinc-600">Required to enable new alerts.</span></span><span className={`relative h-6 w-11 rounded-full ${value.enabled ? "bg-emerald-600" : "bg-zinc-700"}`}><span className={`absolute top-1 size-4 rounded-full bg-white transition-transform ${value.enabled ? "translate-x-6" : "translate-x-1"}`} /></span></button>
      <div className="grid gap-3 sm:grid-cols-2"><Field label="Tenant ID"><Input disabled={locked} value={value.tenant_id} onChange={(event) => update("tenant_id", event.target.value)} className="mt-1.5 w-full font-mono text-xs" /></Field><Field label="Client ID"><Input disabled={locked} value={value.client_id} onChange={(event) => update("client_id", event.target.value)} className="mt-1.5 w-full font-mono text-xs" /></Field></div>
      <Field label="Client Secret"><div className="relative mt-1.5"><Input disabled={locked} type={showSecret ? "text" : "password"} autoComplete="new-password" value={value.client_secret} onChange={(event) => update("client_secret", event.target.value)} placeholder={secretConfigured ? "Credential configured — type only to replace it" : "Enter the credential"} className="w-full pr-10 font-mono text-xs" /><button type="button" disabled={locked} aria-label={showSecret ? "Hide credential" : "Show entered credential"} onClick={() => setShowSecret((current) => !current)} className="absolute right-1 top-1 grid size-7 place-items-center rounded text-zinc-600 hover:bg-zinc-800 hover:text-zinc-300 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50">{showSecret ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}</button></div>{secretConfigured && !value.client_secret && <span className="mt-1 block text-[10px] text-emerald-500">Credential configured. The saved value is never displayed.</span>}</Field>
      <div className="grid gap-3 sm:grid-cols-2"><Field label="Sender email"><Input disabled={locked} type="email" value={value.sender_email} onChange={(event) => update("sender_email", event.target.value)} placeholder="logs@company.com" className="mt-1.5 w-full" /></Field><Field label="Sender name"><Input disabled={locked} maxLength={100} value={value.sender_name} onChange={(event) => update("sender_name", event.target.value)} placeholder="LogHill" className="mt-1.5 w-full" /></Field></div>
    </div>
  );
}

function Field({ label, children }: React.PropsWithChildren<{ label: string }>) { return <label className="block text-[11px] font-medium text-zinc-400">{label}{children}</label>; }
