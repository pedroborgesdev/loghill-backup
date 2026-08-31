export function EmailProviderCard({ provider, selected, available, onSelect }: { provider: "outlook" | "gmail"; selected: boolean; available: boolean; onSelect: () => void }) {
  const outlook = provider === "outlook";
  return <button type="button" disabled={!available} aria-pressed={selected} onClick={onSelect} className={`relative flex min-h-24 w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${selected ? "border-zinc-500 bg-zinc-800/70" : "border-zinc-800 bg-zinc-950/60 hover:border-zinc-700"}`}>
    <img src={`/providers/${provider}.svg`} alt={`${outlook ? "Outlook" : "Gmail"} logo`} className="size-10 rounded-lg" />
    <span><span className="block text-sm font-medium text-zinc-200">{outlook ? "Outlook" : "Gmail"}</span><span className="mt-0.5 block text-[11px] text-zinc-600">{outlook ? "Microsoft 365 / O365" : "SMTP / app password"}</span><span className="mt-1 block text-[10px] text-emerald-500">Available</span></span>
  </button>;
}
