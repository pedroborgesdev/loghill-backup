import {
  ArrowLeft,
  ArrowDownToLine,
  Bell,
  ChevronDown,
  Filter,
  Edit3,
  KeyRound,
  Pause,
  Play,
  Rows3,
  RefreshCw,
  Server,
  ShieldOff,
  Trash2,
  Zap,
} from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api";
import { queryClient } from "../api/queryClient";
import { LogViewer, type LogDensity } from "../components/LogViewer";
import { DateTimePicker, Listbox } from "../components/controls";
import { SenderActionDialogs, type SenderAction } from "../components/senders/SenderDialogs";
import { SenderAssociationsDialog, type AssociationKind } from "../components/senders/SenderAssociationsDialog";
import { preloadSenderAssociations } from "../components/senders/senderAssociationsData";
import {
  Button,
  ConfirmDialog,
  ErrorAlert,
  Input,
  Panel,
  Pagination,
  SearchInput,
  Skeleton,
  StatusBadge,
} from "../components/ui";
import { useDebounce } from "../hooks/useDebounce";
import { useCachedState } from "../hooks/useCachedState";
import { useAutoRefresh } from "../hooks/useAutoRefresh";
import {
  logSignature,
  prepareLogEntries,
  useLogStream,
} from "../hooks/useLogStream";
import { useAppShell } from "../layouts/appShellContext";
import type { LogPage, LogSeverity, Sender, SenderInstance, SenderPage } from "../types/api";
import { formatBytes, formatDate, formatNumber } from "../utils/format";
import {
  calculateLivePagination,
  limitLiveEntries,
} from "../utils/livePagination";
import { syncSearchParams } from "../utils/query";
import { severities, severityStyles } from "../utils/severity";
import { waitForMinimumLoading } from "../utils/minimumLoading";

function SenderHeaderSkeleton() {
  return (
    <Panel className="shrink-0 p-4">
      <div className="space-y-3">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-4 w-72" />
        <div className="grid grid-cols-2 gap-3 pt-2 sm:grid-cols-4">
          {Array.from({ length: 4 }, (_, index) => (
            <Skeleton key={index} className="h-10 w-full" />
          ))}
        </div>
      </div>
    </Panel>
  );
}

function instanceOptionLabel(instance: SenderInstance, senderName: string) {
  return `${senderName} · ${instance.id.slice(-6)}`;
}
function instanceStatus(instance: SenderInstance): "online" | "inactive" {
  if (instance.status === "online" || instance.status === "inactive") return instance.status;
  return instance.last_activity_at ? "online" : "inactive";
}

export function SenderDetailsPage() {
  const { sender = "" } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const { refreshToken, setRefreshing, setStreamState } = useAppShell();
  const [details, setDetails] = useCachedState<Sender>(["view", "sender", sender, "details"]);
  const [senderAction, setSenderAction] = useState<SenderAction>();
  const [associationKind, setAssociationKind] = useState<AssociationKind>();
  const [deletingInstance, setDeletingInstance] = useState<SenderInstance>();
  const [deletingInstanceBusy, setDeletingInstanceBusy] = useState(false);
  const [logs, setLogs] = useCachedState<LogPage>(["view", "sender", sender, "logs"]);
  const [instances, setInstances] = useCachedState<SenderInstance[]>(["view", "sender", sender, "instances"], []);
  const logsRef = useRef<LogPage | undefined>(undefined);
  const autoSelectedQuery = useRef("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState(params.get("search") ?? "");
  const [paused, setPaused] = useState(false);
  const [pauseDialogOpen, setPauseDialogOpen] = useState(false);
  const pendingAction = useRef<(() => void) | null>(null);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [density, setDensity] = useState<LogDensity>(
    () => (localStorage.getItem("log-density") as LogDensity) || "compact",
  );
  const [autoScroll, setAutoScroll] = useState(true);
  const debounced = useDebounce(search);
  const selected = (
    params.get("severity")?.split(",").filter(Boolean) ?? []
  ) as LogSeverity[];
  const selectedKey = selected.join(",");
  const page = Math.max(1, Number(params.get("page") ?? 1));
  const pageSize = Math.max(1, Number(params.get("page_size") ?? 100));
  const order = params.get("order") ?? "desc";
  const startDate = params.get("start_date") ?? "";
  const endDate = params.get("end_date") ?? "";
  const eventMode = params.get("event") ?? "all";
  const eventKey = params.get("event_key") ?? "";
  const instanceID = params.get("instance_id") ?? "";
  const choosingInstance = instances.length > 1 && !instances.some((instance) => instance.id === instanceID);
  const selectedInstance = instances.find((instance) => instance.id === instanceID);

  // Só os logs começam vazios; cabeçalho/instances reaproveitam prefetch/cache.
  useLayoutEffect(() => {
    logsRef.current = undefined;
    autoSelectedQuery.current = "";
    setLogs(undefined);
    setLoading(true);
    setError("");
    if (!queryClient.getQueryData<Sender>(["view", "sender", sender, "details"])) {
      const dashboard = queryClient.getQueryData<SenderPage>(["view", "dashboard", "senders"]);
      const seeded = dashboard?.items.find((item) => item.id === sender);
      if (seeded) setDetails(seeded);
    }
  }, [sender, setDetails, setLogs]);

  const buildQuery = useCallback(
    (forInstance: string) => {
      const queryParams = new URLSearchParams({
        page: String(page),
        page_size: String(pageSize),
        order,
      });
      if (selectedKey) queryParams.set("severity", selectedKey);
      if (debounced) queryParams.set("search", debounced);
      if (startDate) queryParams.set("start_date", new Date(startDate).toISOString());
      if (endDate) queryParams.set("end_date", new Date(endDate).toISOString());
      if (eventMode !== "all") queryParams.set("event", eventMode);
      if (eventKey) queryParams.set("event_key", eventKey);
      if (forInstance) queryParams.set("instance_id", forInstance);
      return queryParams.toString();
    },
    [debounced, endDate, eventKey, eventMode, order, page, pageSize, selectedKey, startDate],
  );

  const query = useMemo(() => buildQuery(instanceID), [buildQuery, instanceID]);

  const stream = useLogStream(choosingInstance ? "" : sender, selected, paused, instanceID);
  const receivedCountRef = useRef(stream.receivedCount);
  const receivedAtLastLoad = useRef(0);
  receivedCountRef.current = stream.receivedCount;

  useEffect(() => {
    setStreamState(stream.state);
    return () => setStreamState(null);
  }, [setStreamState, stream.state]);

  const load = useCallback(async () => {
    // A seleção automática da única instância reescreve a URL; sem isso o novo
    // instance_id dispararia um segundo fetch idêntico ao que acabou de chegar.
    if (autoSelectedQuery.current === query) {
      autoSelectedQuery.current = "";
      return;
    }
    autoSelectedQuery.current = "";
    const hasPrevious = Boolean(
      logsRef.current ||
        queryClient.getQueryData(["view", "sender", sender, "details"]) ||
        queryClient.getQueryData(["view", "sender", sender, "instances"]),
    );
    const startedAt = performance.now();
    setLoading(true);
    setRefreshing(hasPrevious);
    try {
      const [nextDetails, nextInstances] = await Promise.all([
        queryClient.fetchQuery({
          queryKey: ["view", "sender", sender, "details"],
          queryFn: () => api.sender(sender),
          staleTime: 0,
        }),
        queryClient.fetchQuery({
          queryKey: ["view", "sender", sender, "instances"],
          queryFn: async () => (await api.senderInstances(sender)).items,
          staleTime: 0,
        }),
      ]);
      setDetails(nextDetails);
      setInstances(nextInstances);
      if (nextInstances.length > 1 && !nextInstances.some((instance) => instance.id === instanceID)) {
        logsRef.current = undefined;
        setLogs(undefined);
        setError("");
        return;
      }
      const activeInstance =
        instanceID || (nextInstances.length === 1 ? nextInstances[0].id : "");
      const activeQuery = buildQuery(activeInstance);
      const nextLogs = await api.logs(sender, activeQuery);
      if (!hasPrevious) await waitForMinimumLoading(startedAt);
      logsRef.current = nextLogs;
      setLogs(nextLogs);
      if (activeInstance) queryClient.setQueryData(["view", "sender", sender, "instance-logs", activeInstance], nextLogs);
      receivedAtLastLoad.current = receivedCountRef.current;
      setError("");
      if (activeInstance !== instanceID) {
        autoSelectedQuery.current = activeQuery;
        setParams((current) => { const next = new URLSearchParams(current); next.set("instance_id", activeInstance); return next; }, { replace: true });
      }
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Unable to load logs.",
      );
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [buildQuery, instanceID, query, sender, setDetails, setInstances, setLogs, setParams, setRefreshing]);

  const [autoRefresh, setAutoRefresh] = useAutoRefresh(() => void load());

  useEffect(() => { void load(); }, [load, refreshToken]);

  useEffect(() => {
    setParams(
      (current) => syncSearchParams(current, debounced),
      { replace: true },
    );
  }, [debounced, setParams]);

  const liveEntriesVisible =
    !paused &&
    page === 1 &&
    order === "desc" &&
    !debounced &&
    !startDate &&
    !endDate &&
    eventMode === "all" &&
    !eventKey;

  const combined = useMemo(() => {
    const apiEntries = prepareLogEntries(logs?.items ?? [], `api:${sender}`);
    if (!liveEntriesVisible) return apiEntries;
    const liveSignatures = new Set(stream.entries.map(logSignature));
    return [
      ...stream.entries,
      ...apiEntries.filter((entry) => !liveSignatures.has(logSignature(entry))),
    ];
  }, [
    liveEntriesVisible,
    logs?.items,
    sender,
    stream.entries,
  ]);

  const visibleEntries = useMemo(
    () =>
      liveEntriesVisible
        ? limitLiveEntries(combined, pageSize)
        : combined,
    [combined, liveEntriesVisible, pageSize],
  );

  const displayedPagination = liveEntriesVisible
    ? calculateLivePagination({
        baseTotal: logs?.pagination.total ?? 0,
        receivedCount: stream.receivedCount,
        receivedAtLastLoad: receivedAtLastLoad.current,
        pageSize,
      })
    : {
        total: logs?.pagination.total ?? 0,
        totalPages: logs?.pagination.total_pages ?? 1,
      };

  const updateParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.set("page", "1");
    setParams(next);
  };

  const selectInstance = (id: string) => {
    const next = new URLSearchParams(params);
    next.set("instance_id", id);
    next.set("page", "1");
    const cached = queryClient.getQueryData<LogPage>(["view", "sender", sender, "instance-logs", id]);
    logsRef.current = cached;
    setLogs(cached);
    setParams(next);
  };

  const deleteInstance = async () => {
    if (!deletingInstance || deletingInstanceBusy) return;
    const deletedID = deletingInstance.id;
    setDeletingInstanceBusy(true);
    try {
      await api.deleteSenderInstance(sender, deletedID);
      setDeletingInstance(undefined);
      // O fetchQuery do load reaproveita cache fresco (staleTime 20s); sem invalidar a tabela fica com a instância excluída.
      queryClient.removeQueries({ queryKey: ["view", "sender", sender, "instances"] });
      queryClient.removeQueries({ queryKey: ["view", "sender", sender, "details"] });
      queryClient.removeQueries({ queryKey: ["view", "sender", sender, "instance-logs", deletedID] });
      const remaining = instances.filter((instance) => instance.id !== deletedID);
      setInstances(remaining);
      setDetails((current) => current ? { ...current, instance_count: Math.max(0, (current.instance_count ?? 1) - 1) } : current);
      const dashboard = queryClient.getQueryData<SenderPage>(["view", "dashboard", "senders"]);
      if (dashboard) {
        queryClient.setQueryData(["view", "dashboard", "senders"], {
          ...dashboard,
          items: dashboard.items.map((item) => item.id === sender
            ? { ...item, instance_count: Math.max(0, (item.instance_count ?? 1) - 1) }
            : item),
        });
      }
      const nextInstanceID = remaining.length === 1 ? remaining[0].id : instanceID === deletedID ? "" : instanceID;
      if (nextInstanceID !== instanceID) {
        logsRef.current = undefined;
        setLogs(undefined);
        setParams((current) => {
          const next = new URLSearchParams(current);
          if (nextInstanceID) next.set("instance_id", nextInstanceID);
          else next.delete("instance_id");
          next.set("page", "1");
          return next;
        }, { replace: true });
        // O useEffect de load reage à mudança de instance_id.
        return;
      }
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Unable to delete the instance.");
    } finally {
      setDeletingInstanceBusy(false);
    }
  };

  const autoRefreshControl = (
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
  );

  if (choosingInstance) {
    const instanceStatusTab = params.get("instance_status") === "inactive" ? "inactive" : "active";
    const activeInstances = instances.filter((instance) => instance.status === "online");
    const inactiveInstances = instances.filter((instance) => instance.status !== "online");
    const filteredInstances = instanceStatusTab === "active" ? activeInstances : inactiveInstances;
    const instancePage = Math.max(1, Number(params.get("instance_page") ?? 1));
    const instancePageSize = Math.max(1, Number(params.get("instance_page_size") ?? 25));
    const totalPages = Math.max(1, Math.ceil(filteredInstances.length / instancePageSize));
    const visibleInstances = filteredInstances.slice((instancePage - 1) * instancePageSize, instancePage * instancePageSize);
    const updateInstancePage = (nextPage: number, nextPageSize = instancePageSize) => {
      const next = new URLSearchParams(params);
      next.set("instance_page", String(nextPage));
      next.set("instance_page_size", String(nextPageSize));
      setParams(next);
    };
    const selectInstanceStatusTab = (status: "active" | "inactive") => {
      const next = new URLSearchParams(params);
      next.set("instance_status", status);
      next.set("instance_page", "1");
      setParams(next);
    };
    return <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3"><div><button type="button" onClick={() => navigate("/senders")} className="mb-2 inline-flex items-center gap-1.5 text-xs text-zinc-500 hover:text-zinc-200"><ArrowLeft className="size-3.5" />Back to senders</button><h2 className="text-xl font-semibold">Instances de {details?.name ?? sender}</h2><p className="mt-1 text-sm text-zinc-500">Select which sender execution you want to view.</p></div><div className="flex flex-wrap items-center justify-end gap-2">{autoRefreshControl}<div className="rounded-lg border border-zinc-800 bg-[#161618] px-3 py-2 text-xs text-zinc-500"><span className="font-mono text-zinc-200">{instances.length}</span> identified instances</div></div></div>
      {error && <ErrorAlert message={error} onRetry={() => void load()} />}
      <Panel className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div className="shrink-0 border-b border-zinc-800 px-4 pt-3">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-medium">Instances registradas</h3>
              <p className="mt-1 text-[11px] text-zinc-600">Each row represents an independent startup of the same sender.</p>
            </div>
            <Button onClick={() => void load()} disabled={loading} className="sm:px-2.5">
              <RefreshCw className={`size-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
          </div>
          <div className="mt-3 flex gap-1" role="tablist" aria-label="Instance status">
            <button type="button" role="tab" aria-selected={instanceStatusTab === "active"} onClick={() => selectInstanceStatusTab("active")} className={`border-b-2 px-3 pb-2 text-xs font-medium transition-colors ${instanceStatusTab === "active" ? "border-emerald-400 text-zinc-100" : "border-transparent text-zinc-500 hover:text-zinc-300"}`}>Active <span className="ml-1 text-[10px] text-zinc-500">{activeInstances.length}</span></button>
            <button type="button" role="tab" aria-selected={instanceStatusTab === "inactive"} onClick={() => selectInstanceStatusTab("inactive")} className={`border-b-2 px-3 pb-2 text-xs font-medium transition-colors ${instanceStatusTab === "inactive" ? "border-zinc-300 text-zinc-100" : "border-transparent text-zinc-500 hover:text-zinc-300"}`}>Inactive <span className="ml-1 text-[10px] text-zinc-500">{inactiveInstances.length}</span></button>
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-auto"><table className="w-full text-left text-xs"><thead className="sticky top-0 z-10 border-b border-zinc-800 bg-[#1c1c1f] text-zinc-500"><tr><th className="w-36 px-4 py-3">Status</th><th className="px-4 py-3">Instance</th><th className="w-44 px-4 py-3">Started at</th><th className="w-44 px-4 py-3">Last activity</th><th className="w-32 px-4 py-3">Logs</th><th className="w-24 px-4 py-3 text-right">Actions</th></tr></thead><tbody>{visibleInstances.length === 0 ? <tr><td colSpan={6} className="h-32 px-4 text-center text-zinc-500">No {instanceStatusTab === "active" ? "active" : "inactive"} instances.</td></tr> : visibleInstances.map((instance) => <tr key={instance.id} tabIndex={0} onClick={() => selectInstance(instance.id)} onKeyDown={(event) => { if (event.key === "Enter") selectInstance(instance.id); }} className="h-14 cursor-pointer border-b border-zinc-800/70 hover:bg-zinc-900/50"><td className="px-4"><StatusBadge status={instanceStatus(instance)} /></td><td className="px-4"><div className="flex items-center gap-3"><span className="grid size-8 shrink-0 place-items-center rounded-lg border border-zinc-700 bg-zinc-900"><Server className="size-4 text-sky-400" /></span><span className="min-w-0"><strong className="block text-xs font-medium text-zinc-200">{details?.name ?? sender}</strong><code className="mt-0.5 block truncate text-[10px] text-zinc-600">{instance.id}</code></span></div></td><td className="px-4 text-zinc-400">{instance.legacy ? "Unavailable" : formatDate(instance.created_at)}</td><td className="px-4 text-zinc-400">{formatDate(instance.last_activity_at)}</td><td className="px-4 font-mono text-zinc-300">{formatNumber(instance.log_line_count)}</td><td className="px-4"><button type="button" aria-label={`Delete instance ${instance.id}`} title="Delete instance" disabled={deletingInstanceBusy} onClick={(event) => { event.stopPropagation(); setDeletingInstance(instance); }} onKeyDown={(event) => event.stopPropagation()} className="ml-auto grid size-8 place-items-center rounded-lg border border-zinc-700 text-zinc-500 hover:border-red-900 hover:bg-red-950/20 hover:text-red-400 disabled:opacity-50"><Trash2 className="size-4" /></button></td></tr>)}</tbody></table></div>
        <Pagination page={Math.min(instancePage, totalPages)} totalPages={totalPages} total={filteredInstances.length} pageSize={instancePageSize} onPageSizeChange={(value) => updateInstancePage(1, value)} onChange={(value) => updateInstancePage(value)} />
      </Panel>
      <ConfirmDialog open={Boolean(deletingInstance)} title="Delete instance?" description={deletingInstance ? `All logs from instance ${deletingInstance.id} will be permanently deleted. The sender and other instances will not be changed.` : ""} confirmLabel={deletingInstanceBusy ? "Deleting..." : "Delete instance"} onClose={() => !deletingInstanceBusy && setDeletingInstance(undefined)} onConfirm={() => void deleteInstance()} />
    </div>;
  }

  const toggleSeverity = (severity: LogSeverity) => {
    const values = selected.includes(severity)
      ? selected.filter((value) => value !== severity)
      : [...selected, severity];
    updateParam("severity", values.join(","));
  };

  const requirePaused = (action?: () => void) => {
    if (paused) {
      action?.();
      return;
    }
    pendingAction.current = action ?? null;
    setPauseDialogOpen(true);
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {!details ? (
        <SenderHeaderSkeleton />
      ) : (
        <Panel className="shrink-0 p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h2 className="truncate text-xl font-semibold">{details.name}</h2>
                <StatusBadge status={selectedInstance ? instanceStatus(selectedInstance) : details.status} />
              </div>
              <code className="mt-1 block truncate text-xs text-zinc-600">
                {selectedInstance?.id ?? details.id}
              </code>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {autoRefreshControl}
              {loading && <RefreshCw className="size-4 animate-spin text-zinc-500" aria-label="Atualizando sender" />}
              <Button onMouseEnter={() => preloadSenderAssociations("alerts")} onFocus={() => preloadSenderAssociations("alerts")} onClick={() => setAssociationKind("alerts")}><Bell className="size-4" />Alerts</Button>
              <Button onMouseEnter={() => preloadSenderAssociations("events")} onFocus={() => preloadSenderAssociations("events")} onClick={() => setAssociationKind("events")}><Zap className="size-4" />Events</Button>
              <Button onClick={() => setSenderAction("edit")}><Edit3 className="size-4" />Edit</Button>
              {details.status === "revoked" ? <Button onClick={() => setSenderAction("reactivate")}><KeyRound className="size-4" />Reactivate</Button> : details.status !== "expired" ? <Button onClick={() => setSenderAction("revoke")}><ShieldOff className="size-4" />Revoke</Button> : null}
              <a
              href={api.downloadURL(sender, query)}
              className="inline-flex h-9 items-center gap-2 rounded-lg border border-zinc-700 bg-zinc-900 px-3 text-sm font-medium text-zinc-200 hover:border-zinc-600 hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
            >
              <ArrowDownToLine className="size-4" />
              Exportar
              </a>
            </div>
          </div>
          <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 border-t border-zinc-800 pt-3 text-xs sm:grid-cols-3 xl:grid-cols-6">
            <div><dt className="text-zinc-600">Logs</dt><dd className="mt-1 font-mono text-zinc-300">{formatNumber(selectedInstance?.log_line_count ?? details.log_line_count)}</dd></div>
            <div><dt className="text-zinc-600">Size</dt><dd className="mt-1 font-mono text-zinc-300">{formatBytes(selectedInstance?.log_file_size ?? details.log_file_size)}</dd></div>
            <div><dt className="text-zinc-600">Last activity</dt><dd className="mt-1 text-zinc-300">{formatDate(selectedInstance?.last_activity_at ?? details.last_activity_at)}</dd></div>
            <div><dt className="text-zinc-600">Healthcheck</dt><dd className="mt-1 text-zinc-300">{formatDate(selectedInstance?.last_healthcheck_at ?? details.last_healthcheck_at)}</dd></div>
            <div><dt className="text-zinc-600">Started at</dt><dd className="mt-1 text-zinc-300">{selectedInstance?.legacy ? "Unavailable" : formatDate(selectedInstance?.created_at ?? details.created_at)}</dd></div>
            <div><dt className="text-zinc-600">Sender</dt><dd className="mt-1 font-mono text-zinc-300">{details.id}</dd></div>
          </dl>
        </Panel>
      )}

      {error && <ErrorAlert message={error} onRetry={() => void load()} />}

      <Panel className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
        <div className="relative z-20 shrink-0 border-b border-zinc-800 bg-[#161618]">
          <div className="flex min-h-14 flex-wrap items-center gap-2 p-3">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Search message, event, or metadata"
              className="min-w-52 flex-1"
              blocked={!paused}
              onBlocked={() => requirePaused()}
            />
            <Button
              aria-expanded={advancedOpen}
              onClick={() => setAdvancedOpen((value) => !value)}
              className={advancedOpen ? "border-zinc-500 bg-zinc-800" : ""}
            >
              <Filter className="size-4" />
              Filters
              <ChevronDown className={`size-3.5 transition-transform ${advancedOpen ? "rotate-180" : ""}`} />
            </Button>
            <Button onClick={() => setPaused((value) => !value)}>
              {paused ? <Play className="size-4" /> : <Pause className="size-4" />}
              {paused ? "Resume" : "Pause"}
              {stream.pendingCount > 0 && (
                <span className="rounded-full bg-zinc-700 px-1.5 text-[10px]">
                  {stream.pendingCount}
                </span>
              )}
            </Button>
            {instances.length > 1 && instanceID && <Listbox value={instanceID} onChange={selectInstance} label="Sender instance" className="w-56" options={instances.map((instance) => ({ value: instance.id, label: instanceOptionLabel(instance, details?.name ?? sender) }))} />}
          </div>

          <div className="flex flex-wrap gap-1.5 border-t border-zinc-800/70 px-3 py-2">
            {severities.map((severity) => (
              <button
                key={severity}
                type="button"
                aria-pressed={selected.includes(severity)}
                onClick={() => toggleSeverity(severity)}
                className={`h-6 rounded border px-2 font-mono text-[10px] font-medium focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${
                  selected.includes(severity)
                    ? severityStyles[severity].badge
                    : "border-zinc-800 bg-zinc-950 text-zinc-600 hover:border-zinc-700"
                }`}
              >
                {severity}
              </button>
            ))}
            <div className="ml-auto flex items-center gap-2">
              <label className="flex items-center gap-1.5 text-[10px] text-zinc-600">
                <Rows3 className="size-3.5" />
                <Listbox
                  value={density}
                  onChange={(value) => {
                    setDensity(value);
                    localStorage.setItem("log-density", value);
                  }}
                  label="Log density"
                  size="compact"
                  className="w-28"
                  options={[
                    { value: "compact" as LogDensity, label: "Compact" },
                    { value: "comfortable" as LogDensity, label: "Comfortable" },
                  ]}
                />
              </label>
            </div>
          </div>

          {advancedOpen && (
            <div className="grid gap-3 border-t border-zinc-800 bg-[#111113] p-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
              <label className="text-[11px] text-zinc-500">
                Start date
                <span className="mt-1 block">
                  <DateTimePicker
                    value={startDate}
                    onChange={(value) => updateParam("start_date", value)}
                    label="Start date"
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                End date
                <span className="mt-1 block">
                  <DateTimePicker
                    value={endDate}
                    onChange={(value) => updateParam("end_date", value)}
                    label="End date"
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                Ordem
                <span className="mt-1 block">
                  <Listbox
                    value={order}
                    onChange={(value) => updateParam("order", value)}
                    label="Log order"
                    size="compact"
                    className="w-full"
                    options={[
                      { value: "desc", label: "Newest first" },
                      { value: "asc", label: "Oldest first" },
                    ]}
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                Event presence
                <span className="mt-1 block">
                  <Listbox
                    value={eventMode}
                    onChange={(value) => updateParam("event", value === "all" ? "" : value)}
                    label="Filter logs by event"
                    size="compact"
                    className="w-full"
                    options={[
                      { value: "all", label: "All logs" },
                      { value: "with", label: "With event" },
                      { value: "without", label: "Without event" },
                    ]}
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                Event key
                <span className="relative mt-1 block">
                  <Zap className="absolute left-3 top-1/2 z-10 size-3.5 -translate-y-1/2 text-zinc-600" />
                  <Input
                    value={eventKey}
                    onChange={(event) => updateParam("event_key", event.target.value)}
                    placeholder="envia_email_sucesso"
                    className="w-full pl-9 font-mono text-xs"
                  />
                </span>
              </label>
            </div>
          )}
        </div>

        {!logs ? (
          <div className="min-h-0 flex-1 space-y-px overflow-hidden bg-zinc-800">
            {Array.from({ length: 24 }, (_, index) => (
              <div key={index} className="flex h-11 items-center gap-3 bg-[#111113] px-3">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-5 w-14" />
                <Skeleton className="h-3 flex-1" />
              </div>
            ))}
          </div>
        ) : (
          <div
            className={`min-h-0 flex-1 ${
              loading ? "opacity-75 transition-opacity duration-150" : ""
            }`}
          >
            <LogViewer
              entries={visibleEntries}
              density={density}
              streamState={stream.state}
              liveCount={liveEntriesVisible ? stream.receivedCount : 0}
              autoScroll={autoScroll}
              onAutoScrollChange={(enabled) => {
                setAutoScroll(enabled);
                localStorage.setItem("log-auto-scroll", String(enabled));
              }}
            />
          </div>
        )}

        <Pagination
          page={logs?.pagination.page ?? page}
          totalPages={displayedPagination.totalPages}
          total={displayedPagination.total}
          pageSize={pageSize}
          busy={Boolean(logs && loading)}
          onPageSizeChange={(value) =>
            requirePaused(() => updateParam("page_size", String(value)))
          }
          onChange={(value) =>
            requirePaused(() => {
              const next = new URLSearchParams(params);
              next.set("page", String(value));
              setParams(next);
            })
          }
        />
      </Panel>
      <ConfirmDialog
        open={pauseDialogOpen}
        title="Pause live logs"
        description="Text search and pagination require a stable list. Pause the stream before continuing so new events do not change the results during the query."
        confirmLabel="Pause e continuar"
        onClose={() => {
          pendingAction.current = null;
          setPauseDialogOpen(false);
        }}
        onConfirm={() => {
          setPaused(true);
          setPauseDialogOpen(false);
          const action = pendingAction.current;
          pendingAction.current = null;
          action?.();
        }}
      />
      <SenderActionDialogs sender={details} action={senderAction} onClose={() => setSenderAction(undefined)} onChanged={setDetails} onDeleted={(id) => {
        const dashboard = queryClient.getQueryData<SenderPage>(["view", "dashboard", "senders"]);
        if (dashboard) {
          queryClient.setQueryData(["view", "dashboard", "senders"], {
            ...dashboard,
            items: dashboard.items.filter((item) => item.id !== id),
            pagination: { ...dashboard.pagination, total: Math.max(0, dashboard.pagination.total - 1) },
          });
        }
        queryClient.removeQueries({ queryKey: ["view", "sender", id] });
        navigate("/senders");
      }} />
      <SenderAssociationsDialog kind={associationKind} senderId={sender} senderAvailable={Boolean(details && details.status !== "expired" && details.status !== "revoked")} onClose={() => setAssociationKind(undefined)} />
    </div>
  );
}
