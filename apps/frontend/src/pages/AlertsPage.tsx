import { AlertTriangle, Bell, MailCheck, Plus, RefreshCw, ShieldAlert } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { alertsApi } from "../api/alerts";
import { AlertFormDialog } from "../components/alerts/AlertFormDialog";
import { AlertTable } from "../components/alerts/AlertTable";
import { AlertDetailsDrawer } from "../components/alerts/AlertDetailsDrawer";
import { Button, ConfirmDialog, ErrorAlert, MetricCard, Pagination, Panel, SearchInput, Skeleton } from "../components/ui";
import { useDebounce } from "../hooks/useDebounce";
import { useCachedState } from "../hooks/useCachedState";
import { useAppShell } from "../layouts/appShellContext";
import type { AlertPage, EmailAlert } from "../types/alert";
import { formatNumber } from "../utils/format";
import { waitForMinimumLoading } from "../utils/minimumLoading";
import { ExecutionTabs } from "../components/executions/ExecutionTabs";
import { ExecutionView } from "../components/executions/ExecutionView";

function AlertDefinitionsPage() {
  const [params, setParams] = useSearchParams();
  const { refreshToken, setRefreshing, setStreamState, openEmailSettings } = useAppShell();
  const [data, setData] = useCachedState<AlertPage>(["view", "alerts"]);
  const dataRef = useRef<AlertPage | undefined>(data);
  const [initialLoading, setInitialLoading] = useState(!data);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [search, setSearch] = useState(params.get("search") ?? "");
  const debounced = useDebounce(search);
  const [editing, setEditing] = useState<EmailAlert | "new">();
  const [details, setDetails] = useState<EmailAlert>();
  const [deleting, setDeleting] = useState<EmailAlert>();
  const [busyId, setBusyId] = useState("");
  const page = Math.max(1, Number(params.get("page") ?? 1));
  const pageSize = Math.max(1, Number(params.get("page_size") ?? 20));

  useEffect(() => setStreamState(null), [setStreamState]);

  const load = useCallback(async () => {
    const hasPrevious = Boolean(dataRef.current);
    const startedAt = performance.now();
    setLoading(true); setRefreshing(hasPrevious);
    try {
      const query = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
      if (debounced) query.set("search", debounced);
      const next = await alertsApi.list(query.toString());
      if (!hasPrevious) await waitForMinimumLoading(startedAt);
      dataRef.current = next; setData(next); setError("");
    } catch (requestError) { setError(requestError instanceof Error ? requestError.message : "Unable to load alerts."); }
    finally { setLoading(false); setInitialLoading(false); setRefreshing(false); }
  }, [debounced, page, pageSize, setData, setRefreshing]);

  useEffect(() => { void load(); }, [load, refreshToken]);
  useEffect(() => {
    const next = new URLSearchParams(params);
    if (debounced) next.set("search", debounced); else next.delete("search");
    next.set("page", "1");
    if (next.toString() !== params.toString()) setParams(next, { replace: true });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);

  const toggle = async (alert: EmailAlert) => {
    setBusyId(alert.id); setNotice("");
    try { await alertsApi.setEnabled(alert.id, !alert.enabled); setNotice(alert.enabled ? "Alert disabled." : "Alert enabled."); await load(); }
    catch (requestError) { setNotice(requestError instanceof Error ? requestError.message : "Unable to update the alert."); }
    finally { setBusyId(""); }
  };
  const test = async (alert: EmailAlert) => {
    setBusyId(alert.id); setNotice("");
    try { await alertsApi.sendTest(alert.id); setNotice("Test email queued."); }
    catch (requestError) { setNotice(requestError instanceof Error ? requestError.message : "Unable to send the test."); }
    finally { setBusyId(""); }
  };
  const remove = async () => {
    if (!deleting) return;
    setBusyId(deleting.id);
    try { await alertsApi.remove(deleting.id); setDeleting(undefined); setNotice("Alert deleted."); await load(); }
    catch (requestError) { setNotice(requestError instanceof Error ? requestError.message : "Unable to delete the alert."); }
    finally { setBusyId(""); }
  };

  const outlookReady = Boolean(data?.email_provider.configured && data.email_provider.enabled);
  const summary = data?.summary ?? { total: 0, active: 0, recent_failures: 0 };
  return (
    <div className="flex min-h-0 flex-col gap-4 lg:h-full lg:overflow-hidden">
      <div className="flex flex-wrap items-end justify-between gap-3"><div><h2 className="text-xl font-semibold">Email alerts</h2><p className="mt-1 text-sm text-zinc-500">Send email notifications when senders record logs with the selected severities.</p></div><Button onClick={() => setEditing("new")} className="border-zinc-500 bg-zinc-800 text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700"><Plus className="size-4" />New alert</Button></div>
      <ExecutionTabs source="alert"/>

      {!outlookReady && data && <div className="flex flex-wrap items-center gap-3 rounded-xl border border-amber-950 bg-amber-950/15 px-4 py-3"><AlertTriangle className="size-4 text-amber-500" /><div className="min-w-0 flex-1"><p className="text-xs font-medium text-amber-300">Email not configured</p><p className="text-[11px] text-zinc-500">Configure and enable the provider to activate rules and send tests.</p></div><Button onClick={(event) => openEmailSettings(event.currentTarget)} className="h-8">Configure email</Button></div>}
      {outlookReady && <div className="flex items-center gap-2 rounded-lg border border-emerald-950 bg-emerald-950/15 px-3 py-2 text-xs text-emerald-400"><MailCheck className="size-4" />Email configured{data?.email_provider.last_test_status === "success" ? " e validado" : ""}</div>}

      <div className="grid gap-3 sm:grid-cols-3"><MetricCard label="Alerts" value={formatNumber(summary.total)} hint="configured" icon={<Bell className="size-4" />} loading={initialLoading} /><MetricCard label="Active" value={formatNumber(summary.active)} hint="monitoring logs" icon={<MailCheck className="size-4 text-emerald-500" />} loading={initialLoading} /><MetricCard label="Recent failures" value={formatNumber(summary.recent_failures)} hint="last delivery failed" icon={<ShieldAlert className="size-4 text-red-500" />} loading={initialLoading} /></div>

      <Panel className="flex min-h-0 flex-1 flex-col overflow-hidden"><div className="flex min-h-14 shrink-0 flex-col gap-2 border-b border-zinc-800 p-3 sm:flex-row sm:items-center"><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><h3 className="text-sm font-medium text-zinc-200">Alerts configured</h3>{loading && data && <RefreshCw className="size-3 animate-spin text-zinc-600" />}</div><p className="text-[11px] text-zinc-600">An email is sent for each log that matches the rule.</p></div><SearchInput value={search} onChange={setSearch} placeholder="Search alert, sender, or recipient" className="w-full sm:w-80" /><Button onClick={() => void load()} disabled={loading}><RefreshCw className={`size-4 ${loading ? "animate-spin" : ""}`} />Refresh</Button></div>
        {error && <div className="p-3"><ErrorAlert message={error} onRetry={() => void load()} /></div>}
        {notice && <p role="status" className="border-b border-zinc-800 px-4 py-2 text-xs text-zinc-300">{notice}</p>}
        <div className="min-h-0 flex-1 overflow-auto">{initialLoading && !data ? <div className="space-y-2 p-4">{[1,2,3,4,5].map(item=><Skeleton key={item} className="h-12"/>)}</div> : data?.items.length ? <div><AlertTable items={data.items} busyId={busyId} onDetails={setDetails} onEdit={setEditing} onToggle={(alert) => void toggle(alert)} onTest={(alert) => void test(alert)} onDelete={setDeleting} /></div> : <div className="grid min-h-64 place-items-center p-6 text-center"><div><Bell className="mx-auto size-7 text-zinc-700" /><p className="mt-3 text-sm font-medium text-zinc-300">No alerts configured</p><p className="mt-1 max-w-sm text-xs leading-5 text-zinc-600">Create an alert to receive sender errors by email.</p><Button onClick={() => setEditing("new")} className="mt-4"><Plus className="size-4" />New alert</Button></div></div>}
        </div><Pagination page={data?.pagination.page ?? page} totalPages={data?.pagination.total_pages ?? 1} total={data?.pagination.total ?? 0} pageSize={pageSize} busy={loading} onPageSizeChange={(value) => { const next=new URLSearchParams(params); next.set("page_size",String(value)); next.set("page","1"); setParams(next); }} onChange={(value) => { const next=new URLSearchParams(params); next.set("page",String(value)); setParams(next); }} />
      </Panel>
      {editing && <AlertFormDialog alert={editing === "new" ? undefined : editing} outlookReady={outlookReady} onClose={() => setEditing(undefined)} onConfigureOutlook={(trigger) => { setEditing(undefined); openEmailSettings(trigger); }} onSaved={() => { setEditing(undefined); setNotice("Alert saved successfully."); void load(); }} />}
      <AlertDetailsDrawer alert={details} onClose={() => setDetails(undefined)} onEdit={(alert) => { setDetails(undefined); setEditing(alert); }} onTest={(alert) => void test(alert)} />
      <ConfirmDialog open={Boolean(deleting)} title="Delete alert?" description={deleting ? `The “${deleting.name}” alert will be permanently deleted.` : ""} confirmLabel="Delete alert" onClose={() => setDeleting(undefined)} onConfirm={() => void remove()} />
    </div>
  );
}

export function AlertsPage(){const[params]=useSearchParams();if(params.get("tab")==="executions")return <ExecutionView source="alert"/>;return <AlertDefinitionsPage/>}
