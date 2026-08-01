import type { LogSeverity } from "../../types/api";

const severities: LogSeverity[] = ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"];
const styles: Record<LogSeverity, string> = {
  TRACE: "border-zinc-700 text-zinc-500",
  DEBUG: "border-slate-700 text-slate-400",
  INFO: "border-blue-900 text-blue-400",
  WARN: "border-amber-900 text-amber-400",
  ERROR: "border-red-900 text-red-400",
  FATAL: "border-rose-900 text-rose-300",
};

export function SeveritySelector({ value, onChange, disabled = false }: { value: LogSeverity[]; onChange: (value: LogSeverity[]) => void; disabled?: boolean }) {
  const toggle = (severity: LogSeverity) => onChange(value.includes(severity) ? value.filter((item) => item !== severity) : [...value, severity]);
  return (
    <fieldset disabled={disabled}>
      <legend className="mb-2 text-xs font-medium text-zinc-300">Níveis de log</legend>
      <div className="flex flex-wrap gap-2">
        {severities.map((severity) => {
          const selected = value.includes(severity);
          return <button key={severity} type="button" aria-pressed={selected} onClick={() => toggle(severity)} className={`h-8 rounded-lg border px-3 font-mono text-[11px] transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${styles[severity]} ${selected ? "bg-zinc-800 ring-1 ring-current" : "bg-zinc-950 opacity-65 hover:opacity-100"}`}>{severity}</button>;
        })}
      </div>
    </fieldset>
  );
}

export function SeverityBadges({ values }: { values: LogSeverity[] }) {
  return <div className="flex flex-wrap gap-1.5">{values.map((severity) => <span key={severity} className={`rounded border bg-zinc-950 px-1.5 py-0.5 font-mono text-[10px] ${styles[severity]}`}>{severity}</span>)}</div>;
}
