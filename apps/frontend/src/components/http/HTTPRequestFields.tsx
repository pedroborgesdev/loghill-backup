import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import type { HTTPRequestConfig } from "../../types/event";
import { Listbox } from "../controls";
import { Button, Input } from "../ui";
import { httpMethods } from "./httpRequestModel";

function KeyValueFields({ label, value, onChange, valuePlaceholder }: { label: string; value: Record<string, string>; onChange: (value: Record<string, string>) => void; valuePlaceholder: string }) {
  const [rows, setRows] = useState(() => Object.entries(value).map(([key, itemValue]) => ({ key, value: itemValue })).concat(Object.keys(value).length ? [] : [{ key: "", value: "" }]));
  const update = (next: Array<{ key: string; value: string }>) => {
    setRows(next);
    onChange(Object.fromEntries(next.map((row) => [row.key.trim(), row.value]).filter(([key]) => key)));
  };
  return <fieldset className="space-y-2"><legend className="text-xs font-medium text-zinc-300">{label}</legend>{rows.map((row, index) => <div key={index} className="grid grid-cols-[minmax(0,.8fr)_minmax(0,1.2fr)_auto] gap-2"><Input value={row.key} onChange={(event) => update(rows.map((item, itemIndex) => itemIndex === index ? { ...item, key: event.target.value } : item))} placeholder="Name" className="w-full font-mono text-xs" /><Input value={row.value} onChange={(event) => update(rows.map((item, itemIndex) => itemIndex === index ? { ...item, value: event.target.value } : item))} placeholder={valuePlaceholder} className="w-full font-mono text-xs" /><button type="button" aria-label={`Remove ${label.toLowerCase()} row`} onClick={() => update(rows.filter((_, itemIndex) => itemIndex !== index))} className="grid size-9 place-items-center rounded-lg border border-zinc-800 text-zinc-500 hover:text-red-400"><Trash2 className="size-3.5" /></button></div>)}<Button onClick={() => setRows((current) => [...current, { key: "", value: "" }])} className="h-8"><Plus className="size-3.5" />Add {label.toLowerCase()}</Button></fieldset>;
}

export function HTTPRequestFields({ value, onChange }: { value: HTTPRequestConfig; onChange: (value: HTTPRequestConfig) => void }) {
  const set = <K extends keyof HTTPRequestConfig>(key: K, next: HTTPRequestConfig[K]) => onChange({ ...value, [key]: next });
  return <div className="space-y-4 rounded-xl border border-zinc-800 bg-zinc-950/40 p-4"><div className="grid gap-3 sm:grid-cols-[8rem_minmax(0,1fr)]"><label className="text-xs font-medium text-zinc-300">Method<Listbox value={value.method} onChange={(method) => set("method", method)} label="HTTP method" className="mt-2 w-full" options={httpMethods} /></label><label className="text-xs font-medium text-zinc-300">URL<Input value={value.url} onChange={(event) => set("url", event.target.value)} placeholder="https://api.example.com/resource" className="mt-2 w-full font-mono text-xs" /></label></div><KeyValueFields label="Headers" value={value.headers ?? {}} onChange={(headers) => set("headers", headers)} valuePlaceholder="Value or {{variable}}" /><KeyValueFields label="Cookies" value={value.cookies ?? {}} onChange={(cookies) => set("cookies", cookies)} valuePlaceholder="Value or {{variable}}" /><label className="block text-xs font-medium text-zinc-300">Body<textarea value={value.body ?? ""} onChange={(event) => set("body", event.target.value)} maxLength={65536} placeholder={'{"message":"{{log.message}}"}'} className="mt-2 min-h-32 w-full rounded-lg border border-zinc-700 bg-zinc-900 p-3 font-mono text-xs outline-none" /></label><p className="text-[10px] leading-5 text-zinc-600">The call is queued asynchronously. Response bodies are never downloaded. Values support <code>{"{{rule.name}}"}</code>, <code>{"{{event.name}}"}</code>, <code>{"{{sender.name}}"}</code>, <code>{"{{log.message}}"}</code> and <code>{"{{metadata.key}}"}</code>.</p></div>;
}
