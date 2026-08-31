import {
  Activity,
  Archive,
  Bell,
  Database,
  Radar,
  Radio,
  RefreshCw,
  ShieldAlert,
  Zap,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";
import { api } from "../api";
import { queryClient } from "../api/queryClient";
import { executionsApi } from "../api/executions";
import { ExecutionDetailsDrawer } from "../components/executions/ExecutionView";
import { ExecutionStatusBadge } from "../components/executions/ExecutionStatusBadge";
import { RecentBadge } from "../components/executions/RecentBadge";
import { SenderTable, SenderTableSkeleton } from "../components/SenderTable";
import { CreateSenderButton, CreateSenderDialog, SenderActionDialogs, type SenderAction } from "../components/senders/SenderDialogs";
import { Listbox } from "../components/controls";
import {
  Button,
  ErrorAlert,
  IconButton,
  MetricCard,
  Pagination,
  Panel,
  SearchInput,
  Skeleton,
} from "../components/ui";
import { useDebounce } from "../hooks/useDebounce";
import { useCachedState } from "../hooks/useCachedState";
import { useAutoRefresh } from "../hooks/useAutoRefresh";
import { useAppShell } from "../layouts/appShellContext";
import type { Sender, SenderPage, Summary } from "../types/api";
import type { ExecutionRecord } from "../types/execution";
import { isRecentExecution } from "../types/execution";
import { formatNumber } from "../utils/format";
import { syncSearchParams } from "../utils/query";
import { waitForMinimumLoading } from "../utils/minimumLoading";
import { CONTROL_OUTLINE } from "../components/controlStyles";

const emptySummary: Summary = {
  senders: { total: 0, never_connected: 0, online: 0, inactive: 0, expired: 0, revoked: 0 },
  instances: { active: 0, inactive: 0 },
  logs: {
    total: 0,
    last_24_hours: 0,
    errors_last_24_hours: 0,
    fatal_last_24_hours: 0,
  },
};
const executionSourceLabels = { alert: "Alert", event: "Event", monitoring: "Monitoring" } as const;

async function visibleActivityCounts() {
  const now = new Date();
  const day = new Date(now.getTime() - 24 * 60 * 60 * 1_000).toISOString();
  const hour = new Date(now.getTime() - 60 * 60 * 1_000).toISOString();
  const total = async (params: Record<string, string>) => {
    const query = new URLSearchParams({ page: "1", page_size: "1", ...params });
    return (await executionsApi.list(query.toString())).pagination.total;
  };
  const [alerts, events, monitoring, failures, running] = await Promise.all([
    total({ source_type: "alert", started_from: day }),
    total({ source_type: "event", started_from: day }),
    total({ source_type: "monitoring", started_from: day }),
    total({ status: "failed", started_from: hour }),
    total({ status: "pending,processing" }),
  ]);
  return {
    alerts_last_24_hours: alerts,
    events_last_24_hours: events,
    monitoring_last_24_hours: monitoring,
    failed_last_hour: failures,
    running,
  };
}

export function DashboardPage() {
  const location = useLocation();
  const showMetrics = location.pathname === "/";
  const [params, setParams] = useSearchParams();
  const { refreshToken, setRefreshing, setStreamState } = useAppShell();
  const [summary, setSummary] = useCachedState<Summary>(["view", "dashboard", "summary"], emptySummary);
  const [data, setData] = useCachedState<SenderPage>(["view", "dashboard", "senders"]);
  const dataRef = useRef<SenderPage | undefined>(data);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [recentExecutions, setRecentExecutions] = useCachedState<ExecutionRecord[]>(["view", "dashboard", "executions"]);
  const [selectedExecution, setSelectedExecution] = useState<ExecutionRecord>();
  const [search, setSearch] = useState(params.get("search") ?? "");
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedSender, setSelectedSender] = useState<Sender>();
  const [senderAction, setSenderAction] = useState<SenderAction>();
  const debounced = useDebounce(search);
  const page = Math.max(1, Number(params.get("page") ?? 1));
  const pageSize = Math.max(1, Number(params.get("page_size") ?? 25));
  const status = params.get("status") ?? "";

  useEffect(() => {
    setStreamState(null);
  }, [setStreamState]);

  const refreshActivity = useCallback(async () => {
    try {
      const [nextSummary, executions, activity] = await Promise.all([
        api.summary(),
        executionsApi.recent("limit=10"),
        visibleActivityCounts(),
      ]);
      setSummary({ ...nextSummary, executions: nextSummary.executions ? { ...nextSummary.executions, ...activity } : undefined });
      setRecentExecutions(executions.items);
    } catch {
      // Keeps the latest valid data when a silent refresh fails.
    }
  }, [setRecentExecutions, setSummary]);

  const load = useCallback(async () => {
    const hasPreviousData = Boolean(dataRef.current);
    const startedAt = performance.now();
    setLoading(true);
    setRefreshing(hasPreviousData);
    try {
      const query = new URLSearchParams({
        page: String(page),
        page_size: String(pageSize),
        group_by: "name",
        sort: "last_activity_at",
        order: "desc",
      });
      if (debounced) query.set("search", debounced);
      if (status) query.set("status", status);
      const [nextSummary, senders, executions, activity] = await Promise.all([
        api.summary(),
        api.senders(query.toString()),
        executionsApi.recent("limit=10"),
        visibleActivityCounts(),
      ]);
      if (!hasPreviousData) await waitForMinimumLoading(startedAt);
      setSummary({ ...nextSummary, executions: nextSummary.executions ? { ...nextSummary.executions, ...activity } : undefined });
      setRecentExecutions(executions.items);
      dataRef.current = senders;
      setData(senders);
      setError("");
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Unable to load senders.",
      );
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [debounced, page, pageSize, setData, setRecentExecutions, setRefreshing, setSummary, status]);

  const [autoRefresh, setAutoRefresh] = useAutoRefresh(() => void load());

  useEffect(() => { void load(); }, [load, refreshToken]);

  useEffect(() => {
    if (!showMetrics) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") void refreshActivity();
    }, 15_000);
    return () => window.clearInterval(timer);
  }, [refreshActivity, showMetrics]);

  useEffect(() => {
    setParams(
      (current) => syncSearchParams(current, debounced),
      { replace: true },
    );
  }, [debounced, setParams]);

  const metrics = [
    { label: "Instances", value: summary.instances?.active ?? 0, hint: "active", icon: <Radio className="size-4 text-emerald-500" /> },
    { label: "Inactive", value: summary.instances?.inactive ?? 0, hint: "awaiting deletion", icon: <Archive className="size-4 text-amber-500" /> },
    { label: "Total logs", value: summary.logs.total, hint: "stored lines", icon: <Database className="size-4" /> },
    { label: "Last 24h", value: summary.logs.last_24_hours, hint: "new entries", icon: <Activity className="size-4" /> },
    { label: "Errors 24h", value: summary.logs.errors_last_24_hours, hint: "severity ERROR", icon: <ShieldAlert className="size-4 text-red-500" /> },
    { label: "Fatal 24h", value: summary.logs.fatal_last_24_hours, hint: "severity FATAL", icon: <ShieldAlert className="size-4 text-rose-500" /> },
  ];
  const activityMetrics = summary.executions ? [
    { label: "Alerts", value: summary.executions.alerts_last_24_hours, to: "/alerts?tab=executions&period=24h", icon: Bell },
    { label: "Events", value: summary.executions.events_last_24_hours, to: "/events?tab=executions&period=24h", icon: Zap },
    { label: "Monitorings", value: summary.executions.monitoring_last_24_hours, to: "/monitoring?tab=executions&period=24h", icon: Radar },
    { label: "Failures 1h", value: summary.executions.failed_last_hour, to: "/monitoring?tab=executions&period=1h&status=failed", icon: ShieldAlert },
    { label: "In progress", value: summary.executions.running, to: "/monitoring?tab=executions&status=processing", icon: RefreshCw },
  ] : [];
  const visibleRecentExecutions = (recentExecutions ?? []).filter((record) => record.status !== "skipped");

  const updateParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.set("page", "1");
    setParams(next);
  };

  return (
    <div className="flex min-h-0 flex-col gap-3 [&>section]:min-h-[11.5rem] lg:h-full lg:overflow-hidden">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold">
            {showMetrics ? "Overview" : "Senders"}
          </h2>
          <p className="mt-1 text-sm text-zinc-500">
            {showMetrics
              ? "Health and volume of connected services."
              : "Inventory of registered log sources."}
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <label className="flex items-center gap-2 text-xs text-zinc-500">
            Refresh
            <Listbox
            value={autoRefresh}
            onChange={setAutoRefresh}
            label="Refresh interval"
            size="compact"
            className="w-28"
            options={[
              { value: 0, label: "Manual" },
              { value: 15, label: "15 seconds" },
              { value: 30, label: "30 seconds" },
              { value: 60, label: "1 minute" },
            ]}
            />
          </label>
        </div>
      </div>

      {showMetrics && <section className="grid shrink-0 gap-3 xl:grid-cols-[minmax(0,3fr)_minmax(22rem,2fr)]"><div className="grid grid-cols-2 gap-2 lg:grid-cols-4">{metrics.map(metric=><MetricCard key={metric.label} label={metric.label} value={formatNumber(metric.value)} hint={metric.hint} icon={metric.icon} loading={!data&&loading} compact/>)}</div><Panel className="flex min-h-0 flex-col overflow-hidden"><div className="flex shrink-0 items-center justify-between border-b border-zinc-800 px-3 py-1"><h3 className="text-xs font-medium">Recent activity</h3><IconButton label="Refresh page" className="size-7" onClick={()=>void load()} disabled={loading}><RefreshCw className={`size-3.5 ${loading?"animate-spin":""}`}/></IconButton></div><div className="grid min-h-0 flex-1 grid-cols-2 grid-rows-3 gap-2 overflow-hidden p-2">{summary.executions ? activityMetrics.map(item=>{const Icon=item.icon;return <Link key={item.label} to={item.to} className={`flex min-h-8 items-center gap-2 rounded-lg border bg-[#111113] px-2.5 py-1 hover:bg-zinc-900 ${CONTROL_OUTLINE}`}><Icon className="size-3.5 shrink-0 text-zinc-500"/><span className="min-w-0 flex-1 truncate text-[11px] text-zinc-400">{item.label}</span><strong className="font-mono text-base tabular-nums">{formatNumber(item.value)}</strong></Link>}) : [1,2,3,4].map(item=><div key={item} className="p-1.5"><Skeleton className="h-full"/></div>)}</div></Panel></section>}

      <div className={`grid min-h-0 flex-1 gap-3 ${showMetrics ? "xl:grid-cols-[minmax(0,3fr)_minmax(22rem,2fr)]" : ""}`}>
      <Panel className="flex min-h-0 flex-col overflow-hidden">
        <div className="flex min-h-14 flex-col gap-2 border-b border-zinc-800 p-3 sm:flex-row sm:items-center">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <div>
              <h3 className="text-sm font-medium text-zinc-200">Senders</h3>
              <p className="hidden text-[11px] text-zinc-600 lg:block">
                Status, activity, and stored volume
              </p>
            </div>
          </div>
          <CreateSenderButton onClick={() => setCreateOpen(true)} compact />
          <SearchInput
            value={search}
            onChange={setSearch}
            placeholder="Search sender by name"
            className="w-full sm:w-72"
          />
          <Listbox
            value={status}
            onChange={(value) => updateParam("status", value)}
            label="Filter by status"
            className="w-full sm:w-44"
            options={[
              { value: "", label: "All statuses" },
              { value: "never_connected", label: "Never connected" },
              { value: "online", label: "Online" },
              { value: "inactive", label: "Inactive" },
              { value: "revoked", label: "Revoked" },
            ]}
          />
          <Button onClick={() => void load()} disabled={loading} className="sm:px-2.5">
            <RefreshCw className={`size-4 ${loading ? "animate-spin" : ""}`} />
            <span className="sm:hidden xl:inline">Refresh</span>
          </Button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
        {error && (
          <div className="p-3">
            <ErrorAlert message={error} onRetry={() => void load()} />
          </div>
        )}
        {!data ? (
          <SenderTableSkeleton />
        ) : (
          <div className="min-w-[48rem]">
            <SenderTable items={data.items} onAction={(sender, action) => { setSelectedSender(sender); setSenderAction(action); }} />
          </div>
        )}
        </div>
        <Pagination
          page={data?.pagination.page ?? page}
          totalPages={data?.pagination.total_pages ?? 1}
          total={data?.pagination.total ?? 0}
          pageSize={pageSize}
          busy={Boolean(data && loading)}
          onPageSizeChange={(value) => updateParam("page_size", String(value))}
          onChange={(value) => {
            const next = new URLSearchParams(params);
            next.set("page", String(value));
            setParams(next);
          }}
        />
      </Panel>
      {showMetrics && <Panel className="flex min-h-0 flex-col overflow-hidden"><div className="flex min-h-14 shrink-0 items-center justify-between border-b border-zinc-800 px-4"><div><h3 className="text-sm font-medium">Latest executions</h3><p className="text-[11px] text-zinc-600">Most recent system activity</p></div><div className="flex items-center gap-2"><Link to="/monitoring?tab=executions" className="text-[10px] text-zinc-500 hover:text-zinc-200">View history</Link><IconButton label="Refresh page" className="size-7" onClick={()=>void load()} disabled={loading}><RefreshCw className={`size-3.5 ${loading?"animate-spin":""}`}/></IconButton></div></div><div className="min-h-0 flex-1 overflow-y-auto">{recentExecutions === undefined ? <div className="space-y-2 p-3">{[1,2,3,4,5].map(item=><Skeleton key={item} className="h-12"/>)}</div> : visibleRecentExecutions.length ? visibleRecentExecutions.map(record=><button key={record.id} onClick={()=>setSelectedExecution(record)} className="flex w-full items-center gap-2 border-b border-zinc-800/70 px-3 py-1.5 text-left text-xs hover:bg-zinc-900/50"><ExecutionStatusBadge status={record.status}/><span className="min-w-0 flex-1"><span className="block truncate font-medium">{record.source_name}</span><span className="mt-0.5 flex items-center gap-1.5 text-[9px] text-zinc-600"><span className="rounded border border-zinc-700 px-1 py-0.5 text-zinc-400">{executionSourceLabels[record.source_type]}</span><span className="truncate">{record.sender_name||record.sender_id}</span></span></span>{isRecentExecution(record.started_at)&&<RecentBadge startedAt={record.started_at}/>}<time className="whitespace-nowrap text-[10px] text-zinc-600">{new Date(record.started_at).toLocaleTimeString("en-US",{hour:"2-digit",minute:"2-digit"})}</time></button>) : <p className="grid h-full place-items-center p-6 text-center text-xs text-zinc-600">No executions recorded.</p>}</div></Panel>}
      </div>
      <CreateSenderDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(sender) => {
          setData((current) => current ? { ...current, items: page === 1 ? [sender, ...current.items].slice(0, pageSize) : current.items, pagination: { ...current.pagination, total: current.pagination.total + 1 } } : current);
          setSummary((current) => ({ ...current, senders: { ...current.senders, total: current.senders.total + 1, never_connected: current.senders.never_connected + 1 } }));
        }}
      />
      <SenderActionDialogs
        sender={selectedSender}
        action={senderAction}
        onClose={() => { setSenderAction(undefined); setSelectedSender(undefined); }}
        onChanged={(sender) => {
          setData((current) => current ? { ...current, items: current.items.map((item) => item.id === sender.id ? sender : item) } : current);
          setSelectedSender(sender);
        }}
        onDeleted={(id) => {
          setData((current) => {
            if (!current) return current;
            const next = {
              ...current,
              items: current.items.filter((item) => item.id !== id),
              pagination: { ...current.pagination, total: Math.max(0, current.pagination.total - 1) },
            };
            dataRef.current = next;
            return next;
          });
          queryClient.removeQueries({ queryKey: ["view", "sender", id] });
          void load();
        }}
      />
      <ExecutionDetailsDrawer record={selectedExecution} onClose={()=>setSelectedExecution(undefined)}/>
    </div>
  );
}
