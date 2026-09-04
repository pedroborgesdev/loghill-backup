import { ChevronLeft, Mail, PanelRightClose } from "lucide-react";
import type { ReactNode } from "react";
import type { EmailAlert } from "../../types/alert";
import type { EventDefinition, HTTPRequestConfig } from "../../types/event";
import { Listbox } from "../controls";
import { HTTPRequestFields } from "../http/HTTPRequestFields";
import { emptyHTTPRequest } from "../http/httpRequestModel";
import { Button, Input } from "../ui";
import type { MonitoringBlock } from "./blockModel";
import { blockProblem, blockTitle } from "./blockModel";

const severities = ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"].map((value) => ({ value, label: value }));
const weekdays = [
  { value: "monday", label: "Monday" },
  { value: "tuesday", label: "Tuesday" },
  { value: "wednesday", label: "Wednesday" },
  { value: "thursday", label: "Thursday" },
  { value: "friday", label: "Friday" },
  { value: "saturday", label: "Saturday" },
  { value: "sunday", label: "Sunday" },
];
const operators: Record<string, { value: string; label: string }[]> = {
  message: [{ value: "contains", label: "contains" }, { value: "not_contains", label: "does not contain" }, { value: "equals", label: "equals" }, { value: "not_equals", label: "not equals" }, { value: "starts_with", label: "starts with" }, { value: "ends_with", label: "ends with" }, { value: "matches_regex", label: "matches regex (RE2)" }, { value: "not_matches_regex", label: "does not match regex (RE2)" }],
  severity: [{ value: "equals", label: "is" }, { value: "not_equals", label: "is not" }, { value: "in", label: "is in" }, { value: "not_in", label: "is not in" }],
  event_triggered: [{ value: "triggered", label: "is triggered" }, { value: "not_triggered", label: "is not triggered" }, { value: "previously_triggered", label: "was triggered" }, { value: "not_previously_triggered", label: "was not triggered" }],
  sender_status: [{ value: "became", label: "becomes" }],
  alert_triggered: [{ value: "triggered", label: "is triggered" }, { value: "not_triggered", label: "is not triggered" }, { value: "previously_triggered", label: "was triggered" }, { value: "not_previously_triggered", label: "was not triggered" }],
  metadata: [{ value: "exists", label: "exists" }, { value: "not_exists", label: "not exists" }, { value: "equals", label: "equals" }, { value: "not_equals", label: "not equals" }, { value: "contains", label: "contains" }, { value: "gt", label: "greater than" }, { value: "gte", label: "greater than or equal to" }, { value: "lt", label: "less than" }, { value: "lte", label: "less than or equal to" }],
  time: [{ value: "between", label: "is between" }, { value: "not_between", label: "is not between" }, { value: "after", label: "is after" }, { value: "before", label: "is before" }],
  weekday: [{ value: "equals", label: "is" }, { value: "not_equals", label: "is not" }],
  date: [{ value: "between", label: "is between" }, { value: "after", label: "is after" }, { value: "before", label: "is before" }],
};

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block text-xs text-zinc-400"><span className="mb-2 block">{label}</span>{children}</label>;
}

interface Props {
  block?: MonitoringBlock;
  events: EventDefinition[];
  alerts: EmailAlert[];
  collapsed: boolean;
  onToggle: () => void;
  onUpdate: (block: MonitoringBlock) => void;
}

export function BlockConfigurationPanel({ block, events, alerts, collapsed, onToggle, onUpdate }: Props) {
  if (collapsed) {
    return <aside className="hidden w-14 shrink-0 border-l border-zinc-800 bg-[#111113] lg:flex lg:justify-center lg:pt-3"><button type="button" aria-label="Show configuration" onClick={onToggle} className="grid size-9 place-items-center rounded-lg border border-zinc-800 text-zinc-400"><ChevronLeft className="size-4" /></button></aside>;
  }
  if (!block) {
    return <aside className="hidden min-h-0 w-80 min-w-80 flex-col border-l border-zinc-800 bg-[#111113] lg:flex"><header className="flex items-center justify-between border-b border-zinc-800 p-3"><h3 className="text-xs font-semibold">Configuration</h3><button type="button" onClick={onToggle} className="text-zinc-500"><PanelRightClose className="size-4" /></button></header><div className="grid flex-1 place-items-center p-6 text-center text-xs text-zinc-600">Select a block to configure it.</div></aside>;
  }

  const updateCondition = (change: Partial<NonNullable<MonitoringBlock["condition"]>>) => onUpdate({ ...block, condition: { ...block.condition!, ...change } });
  const setValue = (key: string, value: unknown) => updateCondition({ value: { ...block.condition!.value, [key]: value } });
  const setConfig = (key: string, value: unknown) => onUpdate({ ...block, action: { ...block.action!, config: { ...block.action!.config, [key]: value } } });
  const problem = blockProblem(block);

  return <aside className="absolute inset-y-0 right-0 z-20 flex w-[min(88vw,22rem)] flex-col border-l border-zinc-700 bg-[#111113] shadow-2xl lg:static lg:w-80 lg:min-w-80 lg:shadow-none">
    <header className="flex items-center justify-between border-b border-zinc-800 p-3"><div><p className="text-[10px] uppercase text-zinc-600">Configure block</p><h3 className="text-sm font-medium">{blockTitle(block)}</h3></div><button type="button" aria-label="Hide configuration" onClick={onToggle} className="text-zinc-500"><PanelRightClose className="size-4" /></button></header>
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
      {problem && <p role="alert" className="rounded-lg border border-red-950 bg-red-950/20 p-2 text-xs text-red-400">{problem}</p>}
      {block.children && <>
        <p className="rounded-lg border border-cyan-950 bg-cyan-950/20 p-3 text-[11px] leading-5 text-cyan-200">Every block in this group is evaluated with the selected operator. Groups may be nested up to five levels.</p>
        <Field label="Combine children with"><Listbox value={block.groupOperator ?? "and"} onChange={(groupOperator) => onUpdate({ ...block, groupOperator: groupOperator as "and" | "or" })} label="Group operator" className="w-full" options={[{ value: "and", label: "AND — every condition must match" }, { value: "or", label: "OR — at least one condition must match" }]} /></Field>
        <button type="button" role="switch" aria-checked={block.negated} onClick={() => onUpdate({ ...block, negated: !block.negated })} className="flex w-full items-center justify-between rounded-lg border border-zinc-800 p-3 text-left text-xs"><span><b className="block">Negate the entire group</b><small className="text-zinc-600">Applies NOT after evaluating its children.</small></span><span className={`relative h-5 w-9 rounded-full ${block.negated ? "bg-sky-700" : "bg-zinc-700"}`}><span className={`absolute top-1 size-3 rounded-full bg-white ${block.negated ? "translate-x-5" : "translate-x-1"}`} /></span></button>
      </>}
      {block.condition && block.type !== "log_received" && <>
        {block.type !== "sender_status" && block.type !== "wait_until" && <Field label="Operator"><Listbox value={block.condition.operator} onChange={(operator) => updateCondition({ operator })} label="Condition operator" className="w-full" options={operators[block.type] ?? []} /></Field>}
        {block.type === "event_triggered" && <Field label="Event"><Listbox value={String(block.condition.value.event_key ?? "")} onChange={(value) => setValue("event_key", value)} label="Event" className="w-full" options={[{ value: "", label: "Select" }, ...events.filter((event) => event.enabled).map((event) => ({ value: event.key, label: event.name }))]} /></Field>}
        {block.type === "alert_triggered" && <Field label="Alert"><Listbox value={String(block.condition.value.alert_id ?? "")} onChange={(value) => setValue("alert_id", value)} label="Alert" className="w-full" options={[{ value: "", label: "Select" }, ...alerts.filter((alert) => alert.enabled).map((alert) => ({ value: alert.id, label: alert.name }))]} /></Field>}
        {block.type === "sender_status" && <Field label="When it becomes"><Listbox value={String(block.condition.value.status ?? "online")} onChange={(value) => setValue("status", value)} label="Sender status" className="w-full" options={[{ value: "online", label: "Active" }, { value: "inactive", label: "Inactive" }]} /></Field>}
        {block.type === "message" && <Field label={block.condition.operator.includes("regex") ? "RE2 pattern" : "Text"}><Input value={String(block.condition.value.text ?? "")} onChange={(event) => setValue("text", event.target.value)} maxLength={block.condition.operator.includes("regex") ? 500 : undefined} placeholder={block.condition.operator.includes("regex") ? "^ERROR\\s+.*timeout$" : "ECONNRESET"} className="w-full font-mono" />{block.condition.operator.includes("regex") && <small className="mt-2 block leading-4 text-zinc-600">Uses Go RE2 syntax without backtracking or lookaround.</small>}</Field>}
        {block.type === "severity" && <Field label="Severity"><Listbox value={String(block.condition.value.severity ?? "ERROR")} onChange={(value) => setValue("severity", value)} label="Severity" className="w-full" options={severities} /></Field>}
        {block.type === "metadata" && <><Field label="Path"><Input value={String(block.condition.value.path ?? "")} onChange={(event) => setValue("path", event.target.value)} placeholder="process.number" className="w-full" /></Field>{!block.condition.operator.includes("exist") && <Field label="Value"><Input value={String(block.condition.value.value ?? "")} onChange={(event) => setValue("value", event.target.value)} className="w-full" /></Field>}</>}
        {block.type === "time" && <><Field label="Start time"><Input type="time" value={String(block.condition.value.start ?? "")} onChange={(event) => setValue("start", event.target.value)} className="w-full" /></Field>{block.condition.operator.includes("between") && <Field label="End time"><Input type="time" value={String(block.condition.value.end ?? "")} onChange={(event) => setValue("end", event.target.value)} className="w-full" /></Field>}</>}
        {block.type === "weekday" && <Field label="Weekday"><Listbox value={String(block.condition.value.weekday ?? "monday")} onChange={(value) => setValue("weekday", value)} label="Weekday" className="w-full" options={weekdays} /></Field>}
        {block.type === "date" && <><Field label="Start date"><Input type="date" value={String(block.condition.value.start ?? "")} onChange={(event) => setValue("start", event.target.value)} className="w-full" /></Field>{block.condition.operator === "between" && <Field label="End date"><Input type="date" value={String(block.condition.value.end ?? "")} onChange={(event) => setValue("end", event.target.value)} className="w-full" /></Field>}</>}
        {block.type === "wait_until" && <>
          <p className="rounded-lg border border-amber-900/60 bg-amber-950/20 p-3 text-[11px] leading-5 text-amber-200">The flow remains pending and continues at the next occurrence of this weekday and time.</p>
          <Field label="Continue on"><Listbox value={String(block.condition.value.weekday ?? "monday")} onChange={(value) => setValue("weekday", value)} label="Wait until weekday" className="w-full" options={weekdays} /></Field>
          <Field label="At"><Input type="time" value={String(block.condition.value.time ?? "09:00")} onChange={(event) => setValue("time", event.target.value)} className="w-full" /></Field>
          <Field label="Timezone"><Input value={String(block.condition.value.timezone ?? "UTC")} onChange={(event) => setValue("timezone", event.target.value)} placeholder="America/Sao_Paulo" className="w-full" /></Field>
        </>}
        {block.type !== "wait_until" && <button type="button" role="switch" aria-checked={block.negated} onClick={() => onUpdate({ ...block, negated: !block.negated, condition: { ...block.condition!, negated: !block.negated } })} className="flex w-full items-center justify-between rounded-lg border border-zinc-800 p-3 text-left text-xs"><span><b className="block">Negate condition</b><small className="text-zinc-600">Displays the NOT marker.</small></span><span className={`relative h-5 w-9 rounded-full ${block.negated ? "bg-sky-700" : "bg-zinc-700"}`}><span className={`absolute top-1 size-3 rounded-full bg-white ${block.negated ? "translate-x-5" : "translate-x-1"}`} /></span></button>}
      </>}
      {block.type === "trigger_event" && <><Field label="Target event"><Listbox value={String(block.action?.config.event_id ?? "")} onChange={(value) => setConfig("event_id", value)} label="Target event" className="w-full" options={[{ value: "", label: "Select" }, ...events.filter((event) => event.enabled).map((event) => ({ value: event.id, label: event.name }))]} /></Field><Field label="Message"><Input value={String(block.action?.config.message ?? "")} onChange={(event) => setConfig("message", event.target.value)} className="w-full" /></Field><Field label="Severity"><Listbox value={String(block.action?.config.severity ?? "INFO")} onChange={(value) => setConfig("severity", value)} label="Generated severity" className="w-full" options={severities} /></Field></>}
      {block.type === "send_http" && <HTTPRequestFields value={{ ...emptyHTTPRequest(), ...(block.action?.config as unknown as HTTPRequestConfig) }} onChange={(config) => onUpdate({ ...block, action: { ...block.action!, config: { ...config } } })} />}
      {block.type === "send_email" && <><Field label="Recipients"><Input value={Array.isArray(block.action?.config.recipients) ? block.action.config.recipients.join(", ") : ""} onChange={(event) => setConfig("recipients", event.target.value.split(",").map((value) => value.trim()).filter(Boolean))} placeholder="support@company.com" className="w-full" /></Field><Field label="Subject"><Input value={String(block.action?.config.subject ?? "")} onChange={(event) => setConfig("subject", event.target.value)} maxLength={200} className="w-full" /></Field><Field label="Message"><textarea value={String(block.action?.config.message ?? "")} onChange={(event) => setConfig("message", event.target.value)} maxLength={10000} className="min-h-36 w-full rounded-lg border border-zinc-700 bg-zinc-900 p-3 text-sm outline-none" /></Field><div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3"><p className="flex items-center gap-2 text-xs font-medium"><Mail className="size-3.5" />Preview</p><p className="mt-2 text-xs font-medium">{String(block.action?.config.subject || "No subject")}</p><p className="mt-1 whitespace-pre-wrap text-[11px] text-zinc-500">{String(block.action?.config.message || "No message")}</p></div><div className="text-[10px] leading-5 text-zinc-600">Variables: <code>{"{{rule.name}}"}</code>, <code>{"{{sender.name}}"}</code>, <code>{"{{log.message}}"}</code>, <code>{"{{metadata.key}}"}</code></div><Button disabled className="w-full">Send test after saving</Button></>}
    </div>
  </aside>;
}
