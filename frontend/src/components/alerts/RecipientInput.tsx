import { X } from "lucide-react";
import { useId, useState } from "react";

const validEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function RecipientInput({ value, onChange, disabled = false }: { value: string[]; onChange: (value: string[]) => void; disabled?: boolean }) {
  const id = useId();
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  const add = () => {
    const recipient = draft.trim().replace(/,$/, "").toLowerCase();
    if (!recipient) return;
    if (!validEmail.test(recipient)) { setError("Informe um endereço de e-mail válido."); return; }
    if (value.includes(recipient)) { setError("Este destinatário já foi adicionado."); return; }
    if (value.length >= 20) { setError("O limite é de 20 destinatários."); return; }
    onChange([...value, recipient]); setDraft(""); setError("");
  };
  return (
    <div>
      <label htmlFor={id} className="text-xs font-medium text-zinc-300">Destinatários</label>
      <div className={`mt-2 rounded-lg border bg-[#1c1c1f] p-2 transition-colors duration-150 hover:border-zinc-600 focus-within:border-sky-400/70 ${error ? "border-red-900" : "border-zinc-700"}`}>
        <div className="flex flex-wrap gap-1.5">
          {value.map((recipient) => <span key={recipient} className="inline-flex h-7 items-center gap-1 rounded-md border border-zinc-700 bg-zinc-900 pl-2 text-[11px] text-zinc-300">{recipient}<button type="button" disabled={disabled} aria-label={`Remover ${recipient}`} onClick={() => onChange(value.filter((item) => item !== recipient))} className="grid size-7 place-items-center text-zinc-500 hover:text-zinc-100"><X className="size-3" /></button></span>)}
          <input id={id} type="email" disabled={disabled} value={draft} placeholder={value.length ? "Adicionar outro e-mail" : "usuario@empresa.com"} onChange={(event) => { setDraft(event.target.value); setError(""); }} onBlur={add} onKeyDown={(event) => { if (event.key === "Enter" || event.key === ",") { event.preventDefault(); add(); } }} className="h-7 min-w-52 flex-1 bg-transparent px-1 text-xs text-zinc-100 outline-none placeholder:text-zinc-700" />
        </div>
      </div>
      <div className="mt-1.5 flex justify-between text-[10px]"><span role={error ? "alert" : undefined} className={error ? "text-red-400" : "text-zinc-600"}>{error || "Pressione Enter ou vírgula para adicionar."}</span><span className="text-zinc-600">{value.length}/20</span></div>
    </div>
  );
}
