import { Lock } from "lucide-react";
import { Tooltip } from "../ui";

export function EmailProviderCard({ provider, selected, available, onSelect }: { provider: "outlook" | "gmail"; selected: boolean; available: boolean; onSelect: () => void }) {
  const outlook = provider === "outlook";
  return (
    <Tooltip label={available ? "" : "O suporte ao Gmail será adicionado futuramente."}>
    <button type="button" disabled={!available} aria-pressed={selected} onClick={onSelect} className={`relative flex min-h-24 w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${selected ? "border-zinc-500 bg-zinc-800/70" : "border-zinc-800 bg-zinc-950/60 hover:border-zinc-700"} disabled:cursor-not-allowed disabled:opacity-45`}>
      <img src={`/providers/${provider}.svg`} alt={`Logo do ${outlook ? "Outlook" : "Gmail"}`} className="size-10 rounded-lg" />
      <span><span className="block text-sm font-medium text-zinc-200">{outlook ? "Outlook" : "Gmail"}</span><span className="mt-0.5 block text-[11px] text-zinc-600">{outlook ? "Microsoft 365 / O365" : "Indisponível"}</span><span className={`mt-1 block text-[10px] ${available ? "text-emerald-500" : "text-zinc-500"}`}>{available ? "Disponível" : "Em breve"}</span></span>
      {!available && <span className="absolute right-2 top-2 inline-flex items-center gap-1 rounded-full border border-zinc-700 bg-zinc-900 px-2 py-1 text-[9px] text-zinc-500"><Lock className="size-2.5" />Em breve</span>}
    </button>
    </Tooltip>
  );
}
