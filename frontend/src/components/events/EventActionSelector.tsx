import { Mail } from "lucide-react";

export function EventActionSelector() {
  return <section><p className="text-xs font-medium text-zinc-300">Ação</p><div role="radio" aria-checked="true" className="mt-2 flex items-center gap-3 rounded-xl border border-zinc-600 bg-zinc-900 p-4 ring-1 ring-white/10"><span className="grid size-10 place-items-center rounded-lg border border-zinc-700 bg-zinc-950"><Mail className="size-5 text-zinc-300" /></span><span><span className="block text-sm font-medium text-zinc-100">Enviar e-mail</span><span className="mt-1 block text-[11px] text-zinc-500">Usa a configuração global do Outlook e a fila de notificações.</span></span><span className="ml-auto rounded-full border border-zinc-700 px-2 py-1 text-[9px] uppercase tracking-wide text-zinc-500">Outlook</span></div></section>;
}
