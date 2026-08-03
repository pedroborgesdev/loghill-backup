import { AlertTriangle, MailCheck, Plus, RefreshCw, ShieldAlert, Zap } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { eventsApi } from "../api/events";
import { Listbox } from "../components/controls";
import { EventDetailsDrawer } from "../components/events/EventDetailsDrawer";
import { EventFormDialog } from "../components/events/EventFormDialog";
import { EventTable } from "../components/events/EventTable";
import { EventTestDialog } from "../components/events/EventTestDialog";
import { Button, ConfirmDialog, ErrorAlert, Input, MetricCard, Pagination, Panel, SearchInput, Skeleton } from "../components/ui";
import { useDebounce } from "../hooks/useDebounce";
import { useCachedState } from "../hooks/useCachedState";
import { useAppShell } from "../layouts/appShellContext";
import type { EventDefinition, EventPage } from "../types/event";
import { formatNumber } from "../utils/format";
import { waitForMinimumLoading } from "../utils/minimumLoading";
import { ExecutionTabs } from "../components/executions/ExecutionTabs";
import { ExecutionView } from "../components/executions/ExecutionView";

function EventDefinitionsPage() {
  const [params, setParams] = useSearchParams(); const { refreshToken, setRefreshing, setStreamState, openEmailSettings } = useAppShell();
  const [data, setData] = useCachedState<EventPage>(["view", "events"]); const dataRef = useRef<EventPage | undefined>(data); const [initialLoading, setInitialLoading] = useState(!data); const [loading, setLoading] = useState(false);
  const [error, setError] = useState(""); const [notice, setNotice] = useState(""); const [search, setSearch] = useState(params.get("search") ?? ""); const [senderFilter, setSenderFilter] = useState(params.get("sender_name") ?? "");
  const debouncedSearch = useDebounce(search); const debouncedSender = useDebounce(senderFilter); const [editing, setEditing] = useState<EventDefinition | "new">(); const [details, setDetails] = useState<EventDefinition>(); const [testing, setTesting] = useState<EventDefinition>(); const [deleting, setDeleting] = useState<EventDefinition>(); const [busyId, setBusyId] = useState("");
  const page = Math.max(1, Number(params.get("page") ?? 1)); const pageSize = Math.max(1, Number(params.get("page_size") ?? 20)); const enabled = params.get("enabled") ?? "all";
  useEffect(() => setStreamState(null), [setStreamState]);
  const load = useCallback(async () => {
    const hasPrevious = Boolean(dataRef.current);
    const startedAt = performance.now();
    setLoading(true);
    setRefreshing(hasPrevious);
    try {
      const query = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
      if (debouncedSearch) query.set("search", debouncedSearch);
      if (debouncedSender) query.set("sender_name", debouncedSender);
      if (enabled !== "all") query.set("enabled", enabled);
      const next = await eventsApi.list(query.toString());
      if (!hasPrevious) await waitForMinimumLoading(startedAt);
      dataRef.current = next;
      setData(next);
      setError("");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Não foi possível carregar os eventos.");
    } finally {
      setLoading(false);
      setInitialLoading(false);
      setRefreshing(false);
    }
  }, [debouncedSearch, debouncedSender, enabled, page, pageSize, setData, setRefreshing]);
  useEffect(() => { void load(); }, [load, refreshToken]);
  useEffect(() => {
    setParams((current) => {
      const next = new URLSearchParams(current);
      if (debouncedSearch) next.set("search", debouncedSearch); else next.delete("search");
      if (debouncedSender) next.set("sender_name", debouncedSender); else next.delete("sender_name");
      next.set("page", "1");
      return next.toString() === current.toString() ? current : next;
    }, { replace: true });
  }, [debouncedSearch, debouncedSender, setParams]);
  const updateFilter = (key: string, value: string) => { const next = new URLSearchParams(params); if (value && value !== "all") next.set(key, value); else next.delete(key); next.set("page", "1"); setParams(next); };
  const toggle = async (event: EventDefinition) => { setBusyId(event.id); setNotice(""); try { await eventsApi.setEnabled(event.id, !event.enabled); setNotice(event.enabled ? "Evento desativado." : "Evento ativado."); await load(); } catch (requestError) { setNotice(requestError instanceof Error ? requestError.message : "Não foi possível alterar o evento."); } finally { setBusyId(""); } };
  const remove = async () => { if (!deleting) return; setBusyId(deleting.id); try { await eventsApi.remove(deleting.id); setDeleting(undefined); setNotice("Evento excluído."); await load(); } catch (requestError) { setNotice(requestError instanceof Error ? requestError.message : "Não foi possível excluir o evento."); } finally { setBusyId(""); } };
  const outlookReady = Boolean(data?.email_provider.configured && data.email_provider.enabled); const summary = data?.summary ?? { total: 0, active: 0, recent_triggered: 0, recent_failures: 0 };
  return <div className="flex min-h-0 flex-col gap-4 lg:h-full lg:overflow-hidden"><div className="flex flex-wrap items-end justify-between gap-3"><div><h2 className="text-xl font-semibold">Eventos</h2><p className="mt-1 text-sm text-zinc-500">Execute ações quando um log informar explicitamente uma chave de evento.</p></div><Button onClick={() => setEditing("new")}><Plus className="size-4" />Novo evento</Button></div><ExecutionTabs source="event"/>{!outlookReady && data && <div className="flex flex-wrap items-center gap-3 rounded-xl border border-amber-950 bg-amber-950/15 px-4 py-3"><AlertTriangle className="size-4 text-amber-500" /><div className="min-w-0 flex-1"><p className="text-xs font-medium text-amber-300">E-mail não configurado</p><p className="text-[11px] text-zinc-500">Eventos podem ser salvos como inativos, mas só poderão ser ativados após configurar o provider.</p></div><Button onClick={(click) => openEmailSettings(click.currentTarget)} className="h-8">Configurar e-mail</Button></div>}{outlookReady && <div className="flex items-center gap-2 rounded-lg border border-emerald-950 bg-emerald-950/15 px-3 py-2 text-xs text-emerald-400"><MailCheck className="size-4" />E-mail pronto para executar eventos.</div>}<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"><MetricCard label="Eventos" value={formatNumber(summary.total)} hint="configurados" icon={<Zap className="size-4" />} loading={initialLoading} /><MetricCard label="Ativos" value={formatNumber(summary.active)} hint="aceitando disparos" icon={<MailCheck className="size-4 text-emerald-500" />} loading={initialLoading} /><MetricCard label="Disparados recentemente" value={formatNumber(summary.recent_triggered)} hint="nas últimas 24 horas" icon={<Zap className="size-4 text-amber-500" />} loading={initialLoading} /><MetricCard label="Falhas recentes" value={formatNumber(summary.recent_failures)} hint="nas últimas 24 horas" icon={<ShieldAlert className="size-4 text-red-500" />} loading={initialLoading} /></div><Panel className="flex min-h-0 flex-1 flex-col overflow-hidden"><div className="shrink-0 border-b border-zinc-800 p-3"><div className="flex flex-col gap-2 xl:flex-row xl:items-center"><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><h3 className="text-sm font-medium text-zinc-200">Eventos configurados</h3>{loading && data && <RefreshCw className="size-3 animate-spin text-zinc-600" />}</div><p className="text-[11px] text-zinc-600">O matching usa a chave exata e os senders associados, independentemente da severity.</p></div><div className="flex flex-col gap-2 sm:flex-row"><SearchInput value={search} onChange={setSearch} placeholder="Buscar evento pelo nome ou chave" className="w-full sm:w-64" /><label className="sr-only" htmlFor="event-sender-filter">Filtrar por sender</label><Input id="event-sender-filter" value={senderFilter} onChange={(change) => setSenderFilter(change.target.value)} placeholder="Nome do sender" className="w-full sm:w-48" /><Listbox value={enabled} onChange={(value) => updateFilter("enabled", value)} label="Status do evento" className="w-full sm:w-36" options={[{ value: "all", label: "Todos" }, { value: "true", label: "Ativos" }, { value: "false", label: "Inativos" }]} /><Button onClick={() => void load()} disabled={loading}><RefreshCw className={`size-4 ${loading ? "animate-spin" : ""}`} />Atualizar</Button></div></div></div>{error && <div className="p-3"><ErrorAlert message={error} onRetry={() => void load()} /></div>}{notice && <p role="status" className="border-b border-zinc-800 px-4 py-2 text-xs text-zinc-300">{notice}</p>}<div className="min-h-0 flex-1 overflow-auto">{initialLoading && !data ? <div className="space-y-2 p-4">{[1,2,3,4,5].map(item=><Skeleton key={item} className="h-12"/>)}</div> : data?.items.length ? <div className={loading ? "opacity-70 transition-opacity" : ""}><EventTable items={data.items} busyId={busyId} onDetails={setDetails} onEdit={setEditing} onToggle={(event) => void toggle(event)} onTest={setTesting} onDelete={setDeleting} /></div> : <div className="grid min-h-64 place-items-center p-6 text-center"><div><Zap className="mx-auto size-7 text-zinc-700" /><p className="mt-3 text-sm font-medium text-zinc-300">Nenhum evento configurado</p><p className="mt-1 max-w-sm text-xs leading-5 text-zinc-600">Crie uma ação explícita para ser chamada pelo parâmetro <code>event</code> dos seus logs.</p><Button onClick={() => setEditing("new")} className="mt-4"><Plus className="size-4" />Novo evento</Button></div></div>}</div><Pagination page={data?.pagination.page ?? page} totalPages={data?.pagination.total_pages ?? 1} total={data?.pagination.total ?? 0} pageSize={pageSize} busy={loading} onPageSizeChange={(value) => { const next = new URLSearchParams(params); next.set("page_size", String(value)); next.set("page", "1"); setParams(next); }} onChange={(value) => { const next = new URLSearchParams(params); next.set("page", String(value)); setParams(next); }} /></Panel>{editing && <EventFormDialog event={editing === "new" ? undefined : editing} outlookReady={outlookReady} onClose={() => setEditing(undefined)} onConfigureOutlook={(trigger) => { setEditing(undefined); openEmailSettings(trigger); }} onSaved={() => { setEditing(undefined); setNotice("Evento salvo com sucesso."); void load(); }} />}<EventDetailsDrawer event={details} onClose={() => setDetails(undefined)} onEdit={(event) => { setDetails(undefined); setEditing(event); }} onTest={(event) => setTesting(event)} /><EventTestDialog event={testing} onClose={() => setTesting(undefined)} onSent={() => setNotice("E-mail de teste enfileirado.")} /><ConfirmDialog open={Boolean(deleting)} title="Excluir evento?" description={deleting ? `O evento “${deleting.name}” e a chave ${deleting.key} serão removidos permanentemente.` : ""} confirmLabel="Excluir evento" onClose={() => setDeleting(undefined)} onConfirm={() => void remove()} /></div>;
}

export function EventsPage(){const[params]=useSearchParams();if(params.get("tab")==="executions")return <ExecutionView source="event"/>;return <EventDefinitionsPage/>}
