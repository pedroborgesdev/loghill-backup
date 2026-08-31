import { Check, Clipboard, LoaderCircle, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { eventsApi } from "../../api/events";
import { useDebounce } from "../../hooks/useDebounce";
import { Input } from "../ui";
import { eventKeyPattern } from "./eventUtils";


export function EventKeyField({ value, onChange, immutable = false, disabled = false, onAvailabilityChange }: { value: string; onChange: (value: string) => void; immutable?: boolean; disabled?: boolean; onAvailabilityChange?: (available: boolean) => void }) {
  const debounced = useDebounce(value);
  const valid = useMemo(() => eventKeyPattern.test(value), [value]);
  const [available, setAvailable] = useState<boolean | null>(immutable ? true : null);
  const [checking, setChecking] = useState(false);

  useEffect(() => {
    if (immutable) { setAvailable(true); onAvailabilityChange?.(true); return; }
    if (!eventKeyPattern.test(debounced)) { setAvailable(null); onAvailabilityChange?.(false); return; }
    const controller = new AbortController(); setChecking(true);
    void eventsApi.checkKey(debounced, controller.signal).then((result) => { setAvailable(result.available); onAvailabilityChange?.(result.available); }).catch(() => { if (!controller.signal.aborted) { setAvailable(null); onAvailabilityChange?.(false); } }).finally(() => { if (!controller.signal.aborted) setChecking(false); });
    return () => controller.abort();
  }, [debounced, immutable, onAvailabilityChange]);

  const feedback = immutable ? "The key is immutable after creation." : !value ? "Use lowercase letters, numbers, underscores, or hyphens." : !valid ? "The key must be between 3 and 80 characters and cannot contain spaces or accents." : checking ? "Checking availability..." : available ? "Key available." : available === false ? "This key is already in use." : "Unable to check the key.";
  return (
    <div>
      <div className="flex items-center justify-between"><label htmlFor="event-key" className="text-xs font-medium text-zinc-300">Event key</label><button type="button" disabled={!value} onClick={() => void navigator.clipboard.writeText(value)} className="inline-flex items-center gap-1 rounded px-1.5 py-1 text-[10px] text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 disabled:opacity-40"><Clipboard className="size-3" />Copy key</button></div>
      <div className="relative mt-2"><Input id="event-key" value={value} readOnly={immutable} disabled={disabled} maxLength={80} onChange={(event) => onChange(event.target.value)} placeholder="envia_email_sucesso" aria-invalid={Boolean(value && (!valid || available === false))} className="w-full pr-9 font-mono" />{checking ? <LoaderCircle className="absolute right-3 top-1/2 size-4 -translate-y-1/2 animate-spin text-zinc-500" /> : valid && available ? <Check className="absolute right-3 top-1/2 size-4 -translate-y-1/2 text-emerald-400" /> : value && (!valid || available === false) ? <X className="absolute right-3 top-1/2 size-4 -translate-y-1/2 text-red-400" /> : null}</div>
      <p className={`mt-1.5 text-[10px] ${valid && available ? "text-emerald-400" : value && (!valid || available === false) ? "text-red-400" : "text-zinc-600"}`}>{feedback}</p>
      <pre className="mt-3 overflow-x-auto rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-[11px] leading-5 text-zinc-400"><code>{`log.info(\n    "Message sent successfully",\n    event="${value || "event_key"}",\n)`}</code></pre>
    </div>
  );
}
