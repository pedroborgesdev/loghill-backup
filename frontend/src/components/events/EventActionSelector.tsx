import { Mail, MessageSquareText, Radar, Webhook } from "lucide-react";
import type { EventActionType } from "../../types/event";

const actions: Array<{ value: EventActionType; title: string; description: string; icon: typeof Mail }> = [
  { value: "none", title: "Monitoring only", description: "Use as a trigger without external delivery.", icon: Radar },
  { value: "email", title: "Send email", description: "Send the configured email template.", icon: Mail },
  { value: "webhook", title: "Call webhook", description: "POST the event to a public HTTPS endpoint.", icon: Webhook },
  { value: "sms", title: "Send SMS", description: "Send a short message through Twilio.", icon: MessageSquareText },
];

export function EventActionSelector({ value, disabled, onChange }: { value: EventActionType; disabled?: boolean; onChange: (value: EventActionType) => void }) {
  return (
    <section>
      <p className="text-xs font-medium text-zinc-300">Action</p>
      <div role="radiogroup" aria-label="Event action" className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        {actions.map((action) => {
          const Icon = action.icon;
          const selected = value === action.value;
          return (
            <button key={action.value} type="button" role="radio" aria-checked={selected} disabled={disabled} onClick={() => onChange(action.value)} className={`rounded-xl border p-4 text-left transition-colors disabled:opacity-50 ${selected ? "border-sky-500 bg-sky-950/20" : "border-zinc-700 bg-zinc-900 hover:border-zinc-600"}`}>
              <Icon className={`size-5 ${selected ? "text-sky-400" : "text-zinc-500"}`} />
              <span className="mt-3 block text-sm font-medium text-zinc-100">{action.title}</span>
              <span className="mt-1 block text-[11px] leading-4 text-zinc-500">{action.description}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
