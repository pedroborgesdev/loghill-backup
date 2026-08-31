import { Eye, EyeOff, KeyRound } from "lucide-react";
import { useState } from "react";
import { Input, NumberInput } from "../ui";

export interface GmailDraft { host: string; port: number; username: string; password: string; from: string; sender_name: string; enabled: boolean }

export function GmailSettingsForm({ value, passwordConfigured, managed, disabled, onChange }: { value: GmailDraft; passwordConfigured: boolean; managed: boolean; disabled: boolean; onChange: (value: GmailDraft) => void }) {
  const [showPassword, setShowPassword] = useState(false);
  const update = (field: keyof GmailDraft, next: string | number | boolean) => onChange({ ...value, [field]: next });
  const locked = disabled || managed;
  return <div className="space-y-4 rounded-xl border border-zinc-800 bg-zinc-950/55 p-4">
    {managed && <div className="flex items-center gap-2 rounded-lg border border-blue-950 bg-blue-950/20 px-3 py-2 text-[11px] text-blue-300"><KeyRound className="size-3.5" />Configuration managed through environment variables.</div>}
    <button type="button" role="switch" aria-checked={value.enabled} disabled={locked} onClick={() => update("enabled", !value.enabled)} className="flex w-full items-center justify-between rounded-lg text-left focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"><span><span className="block text-xs font-medium text-zinc-200">Enable Gmail delivery</span><span className="mt-1 block text-[10px] text-zinc-600">Uses SMTP with STARTTLS and an app password.</span></span><span className={`relative h-6 w-11 rounded-full ${value.enabled ? "bg-emerald-600" : "bg-zinc-700"}`}><span className={`absolute top-1 size-4 rounded-full bg-white transition-transform ${value.enabled ? "translate-x-6" : "translate-x-1"}`} /></span></button>
    <div className="grid gap-3 sm:grid-cols-[1fr_8rem]"><Field label="SMTP server"><Input disabled={locked} value={value.host} onChange={(e) => update("host", e.target.value)} placeholder="smtp.gmail.com" className="mt-1.5 w-full font-mono text-xs" /></Field><Field label="Port"><NumberInput disabled={locked} min={1} max={65535} value={value.port} onValueChange={(port) => update("port", port)} label="SMTP port" className="mt-1.5 w-full" /></Field></div>
    <Field label="SMTP username"><Input disabled={locked} type="email" value={value.username} onChange={(e) => update("username", e.target.value)} placeholder="your-email@gmail.com" className="mt-1.5 w-full" /></Field>
    <Field label="App password"><div className="relative mt-1.5"><Input disabled={locked} type={showPassword ? "text" : "password"} autoComplete="new-password" value={value.password} onChange={(e) => update("password", e.target.value)} placeholder={passwordConfigured ? "Password configured — type only to replace it" : "Google app password"} className="w-full pr-10 font-mono text-xs" /><button type="button" disabled={locked} aria-label={showPassword ? "Hide password" : "Show entered password"} onClick={() => setShowPassword((current) => !current)} className="absolute right-1 top-1 grid size-7 place-items-center rounded text-zinc-600 hover:bg-zinc-800 hover:text-zinc-300">{showPassword ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}</button></div>{passwordConfigured && !value.password && <span className="mt-1 block text-[10px] text-emerald-500">Password configured. The saved value is never displayed.</span>}</Field>
    <div className="grid gap-3 sm:grid-cols-2"><Field label="Sender email"><Input disabled={locked} type="email" value={value.from} onChange={(e) => update("from", e.target.value)} placeholder="your-email@gmail.com" className="mt-1.5 w-full" /></Field><Field label="Sender name"><Input disabled={locked} maxLength={100} value={value.sender_name} onChange={(e) => update("sender_name", e.target.value)} placeholder="LogHill" className="mt-1.5 w-full" /></Field></div>
  </div>;
}

function Field({ label, children }: React.PropsWithChildren<{ label: string }>) { return <label className="block text-[11px] font-medium text-zinc-400">{label}{children}</label>; }
