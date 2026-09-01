import { Check, ChevronLeft, ChevronRight, Search, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../../api";
import { useDebounce } from "../../hooks/useDebounce";
import { useCachedState } from "../../hooks/useCachedState";
import { waitForMinimumLoading } from "../../utils/minimumLoading";
import type { Sender } from "../../types/api";
import { StatusBadge } from "../ui";

export type SenderOption = Pick<Sender, "id" | "name" | "status">;

function unavailable(sender: SenderOption) {
  return sender.status === "expired" || sender.status === "revoked";
}

export function ThemeCheckbox({ checked, mixed = false }: { checked: boolean; mixed?: boolean }) {
  return (
    <span aria-hidden="true" className={`grid size-4 shrink-0 place-items-center rounded border transition-colors ${checked || mixed ? "border-zinc-500 bg-zinc-800 text-zinc-100" : "border-zinc-700 bg-zinc-950 text-transparent"}`}>
      {mixed ? <span className="h-0.5 w-2 bg-zinc-100" /> : <Check className="size-3" />}
    </span>
  );
}

export function SenderCheckboxItem({ sender, checked, disabled, onChange }: { sender: SenderOption; checked: boolean; disabled?: boolean; onChange: () => void }) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={`${checked ? "Remove" : "Select"} ${sender.name}`}
      disabled={disabled || unavailable(sender)}
      onClick={onChange}
      className={`flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left outline-none transition-colors hover:bg-zinc-900 focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:opacity-40 ${checked ? "bg-zinc-900/80" : ""}`}
    >
      <ThemeCheckbox checked={checked} />
      <span className="min-w-0 flex-1"><span className="block truncate text-xs text-zinc-200">{sender.name}</span><span className="block truncate font-mono text-[10px] text-zinc-600">{sender.id}</span></span>
      <StatusBadge status={sender.status} />
    </button>
  );
}

export function SelectedSenderTags({ value, disabled, onRemove }: { value: SenderOption[]; disabled?: boolean; onRemove: (id: string) => void }) {
  if (!value.length) return null;
  const visible = value.slice(0, 3);
  return (
    <div className="mt-2 max-h-20 overflow-y-auto rounded-lg border border-zinc-800 bg-zinc-950/50 p-2">
      <div className="flex flex-wrap gap-1.5">
        {visible.map((sender) => <span key={sender.id} className="inline-flex max-w-56 items-center gap-1 rounded-md border border-zinc-700 bg-zinc-900 px-2 py-1 text-[10px] text-zinc-300"><span className="truncate">{sender.name}</span><button type="button" aria-label={`Remove ${sender.name}`} disabled={disabled} onClick={() => onRemove(sender.id)} className="rounded text-zinc-600 hover:text-zinc-100"><X className="size-3" /></button></span>)}
        {value.length > visible.length && <span className="rounded-md border border-zinc-800 px-2 py-1 text-[10px] text-zinc-500">+{value.length - visible.length} selected</span>}
      </div>
    </div>
  );
}

export function SenderMultiSelect({ value, onChange, disabled = false, required = false }: { value: SenderOption[]; onChange: (senders: SenderOption[]) => void; disabled?: boolean; required?: boolean }) {
  const [query, setQuery] = useState("");
  const debounced = useDebounce(query);
  const [items = [], setItems] = useCachedState<Sender[]>(["modal", "sender-select"], []);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const hasLoadedItems = useRef(items.length > 0);
  const selected = useMemo(() => new Map(value.map((sender) => [sender.id, sender])), [value]);

  useEffect(() => setPage(1), [debounced]);
  useEffect(() => {
    const controller = new AbortController();
    const firstLoad = !hasLoadedItems.current;
    const startedAt = performance.now();
    setLoading(true);
    const params = new URLSearchParams({ page: String(page), page_size: "20", order: "desc" });
    if (debounced) params.set("search", debounced);
    void api.senders(params.toString(), controller.signal)
      .then(async (response) => { if (firstLoad) await waitForMinimumLoading(startedAt); if (!controller.signal.aborted) { hasLoadedItems.current = true; setItems(response.items); setTotalPages(Math.max(1, response.pagination.total_pages)); setError(""); } })
      .catch((requestError) => { if (!(requestError instanceof DOMException && requestError.name === "AbortError")) setError(requestError instanceof Error ? requestError.message : "Unable to search senders."); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [debounced, page, setItems]);

  const toggle = (sender: SenderOption) => onChange(selected.has(sender.id) ? value.filter((current) => current.id !== sender.id) : [...value, sender]);
  const availableVisible = items.filter((sender) => !unavailable(sender));
  const selectedVisible = availableVisible.filter((sender) => selected.has(sender.id));
  const allVisible = availableVisible.length > 0 && selectedVisible.length === availableVisible.length;
  const mixed = selectedVisible.length > 0 && !allVisible;
  const toggleAll = () => {
    if (allVisible) { const visibleIDs = new Set(availableVisible.map((sender) => sender.id)); onChange(value.filter((sender) => !visibleIDs.has(sender.id))); return; }
    const next = new Map(value.map((sender) => [sender.id, sender]));
    availableVisible.forEach((sender) => next.set(sender.id, sender));
    onChange(Array.from(next.values()));
  };

  return (
    <div>
      <div className="flex items-center justify-between"><label className="text-xs font-medium text-zinc-300">Senders <span className="font-normal text-zinc-600">({required ? "required" : "optional"})</span></label><span className="text-[10px] tabular-nums text-zinc-500">{value.length} selected</span></div>
      <SelectedSenderTags value={value} disabled={disabled} onRemove={(id) => onChange(value.filter((sender) => sender.id !== id))} />
      <div className="mt-2 overflow-hidden rounded-lg border border-zinc-700 bg-[#1c1c1f] transition-colors duration-150 hover:border-zinc-600 focus-within:border-sky-400/70">
        <label className="relative block border-b border-zinc-800"><span className="sr-only">Search senders</span><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-zinc-600" /><input disabled={disabled} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search sender by name" className="h-9 w-full bg-transparent pl-9 pr-3 text-xs text-zinc-200 outline-none placeholder:text-zinc-700" />{loading && items.length > 0 && <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[9px] text-zinc-600">Refreshing...</span>}</label>
        <button type="button" role="checkbox" aria-checked={mixed ? "mixed" : allVisible} aria-label="Select all visible results" disabled={disabled || !availableVisible.length} onClick={toggleAll} className="flex h-9 w-full items-center gap-2 border-b border-zinc-800 px-2.5 text-[10px] text-zinc-400 hover:bg-zinc-900 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50 disabled:opacity-40"><ThemeCheckbox checked={allVisible} mixed={mixed} />Select all visible results</button>
        <div aria-label="Monitored senders" className="h-52 overflow-y-auto p-1">
          {loading && !items.length ? <div className="space-y-1 p-1" aria-label="Loading senders">{Array.from({ length: 4 }, (_, index) => <div key={index} className="h-11 animate-pulse rounded-lg bg-zinc-900" />)}</div> : error ? <p role="alert" className="p-3 text-xs text-red-400">{error}</p> : !items.length ? <p className="p-3 text-xs text-zinc-600">No senders found.</p> : items.map((sender) => <SenderCheckboxItem key={sender.id} sender={sender} checked={selected.has(sender.id)} disabled={disabled} onChange={() => toggle(sender)} />)}
        </div>
        <div className="flex h-9 items-center justify-between border-t border-zinc-800 px-2 text-[10px] text-zinc-600"><button type="button" aria-label="Previous sender page" disabled={disabled || page <= 1 || loading} onClick={() => setPage((current) => current - 1)} className="grid size-7 place-items-center rounded hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 disabled:opacity-30"><ChevronLeft className="size-3.5" /></button><span>Page {page} of {totalPages}</span><button type="button" aria-label="Next sender page" disabled={disabled || page >= totalPages || loading} onClick={() => setPage((current) => current + 1)} className="grid size-7 place-items-center rounded hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 disabled:opacity-30"><ChevronRight className="size-3.5" /></button></div>
      </div>
      {value.some((sender) => sender.status === "inactive") && <p className="mt-1.5 text-[10px] text-amber-500">Inactive senders can be monitored, but may not send new logs.</p>}
    </div>
  );
}

export const SenderSelect = SenderMultiSelect;
