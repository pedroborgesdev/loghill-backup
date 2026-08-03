import type { EmailProviderType } from "../../types/alert";

export function EmailProviderBadge({ provider = "outlook" }: { provider?: EmailProviderType }) {
  const label = provider === "gmail" ? "Gmail" : "Outlook";
  return <span className="inline-flex items-center gap-1.5 rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-[10px] text-zinc-400"><img src={`/providers/${provider}.svg`} alt={`Logo do ${label}`} className="size-3.5 rounded-sm" />{label}</span>;
}
