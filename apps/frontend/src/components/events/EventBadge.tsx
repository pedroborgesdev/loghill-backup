import { Copy, Zap } from "lucide-react";

export function EventBadge({ eventKey, name }: { eventKey: string; name?: string }) {
  return (
    <button type="button" title={name ?? `Event ${eventKey}`} aria-label={`Copy event key ${eventKey}`} onClick={() => void navigator.clipboard.writeText(eventKey)} className="group inline-flex max-w-full items-center gap-1 rounded border border-zinc-700 bg-zinc-900/80 px-1.5 py-0.5 font-sans text-[9px] font-medium text-zinc-400 hover:border-zinc-600 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50">
      <Zap className="size-2.5 shrink-0" /><span className="shrink-0 text-zinc-500">EVENT</span><span className="truncate font-mono">{eventKey}</span><Copy className="size-2.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
    </button>
  );
}
