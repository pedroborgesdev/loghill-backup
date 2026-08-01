import {
  ArrowDownToLine,
  ChevronDown,
  Filter,
  Edit3,
  KeyRound,
  Pause,
  Play,
  Rows3,
  ShieldOff,
  Zap,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api";
import { LogViewer, type LogDensity } from "../components/LogViewer";
import { DateTimePicker, Listbox } from "../components/controls";
import { SenderActionDialogs, type SenderAction } from "../components/senders/SenderDialogs";
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
import {
  logSignature,
  prepareLogEntries,
  useLogStream,
} from "../hooks/useLogStream";
import { useAppShell } from "../layouts/appShellContext";
import type { LogPage, LogSeverity, Sender } from "../types/api";
import { formatBytes, formatDate, formatNumber } from "../utils/format";
import {
  calculateLivePagination,
  limitLiveEntries,
} from "../utils/livePagination";
import { syncSearchParams } from "../utils/query";
import { severities, severityStyles } from "../utils/severity";

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

export function SenderDetailsPage() {
  const { sender = "" } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const { refreshToken, setRefreshing, setStreamState } = useAppShell();
  const [details, setDetails] = useState<Sender>();
  const [senderAction, setSenderAction] = useState<SenderAction>();
  const [logs, setLogs] = useState<LogPage>();
  const logsRef = useRef<LogPage | undefined>(undefined);
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
  const [autoScroll, setAutoScroll] = useState(
    () => localStorage.getItem("log-auto-scroll") !== "false",
  );
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

  const query = useMemo(() => {
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
    return queryParams.toString();
  }, [debounced, endDate, eventKey, eventMode, order, page, pageSize, selectedKey, startDate]);

  const stream = useLogStream(sender, selected, paused);
  const receivedCountRef = useRef(stream.receivedCount);
  const receivedAtLastLoad = useRef(0);
  receivedCountRef.current = stream.receivedCount;

  useEffect(() => {
    setStreamState(stream.state);
    return () => setStreamState(null);
  }, [setStreamState, stream.state]);

  const load = useCallback(async () => {
    setLoading(true);
    setRefreshing(Boolean(logsRef.current));
    try {
      const [nextDetails, nextLogs] = await Promise.all([
        api.sender(sender),
        api.logs(sender, query),
      ]);
      setDetails(nextDetails);
      logsRef.current = nextLogs;
      setLogs(nextLogs);
      receivedAtLastLoad.current = receivedCountRef.current;
      setError("");
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Não foi possível carregar os logs.",
      );
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [query, sender, setRefreshing]);

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
                <StatusBadge status={details.status} />
              </div>
              <code className="mt-1 block truncate text-xs text-zinc-600">
                {details.id}
              </code>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button onClick={() => setSenderAction("edit")}><Edit3 className="size-4" />Editar</Button>
              {details.status === "revoked" ? <Button onClick={() => setSenderAction("reactivate")}><KeyRound className="size-4" />Reativar</Button> : details.status !== "expired" ? <><Button onClick={() => setSenderAction("rotate")}><KeyRound className="size-4" />Nova chave</Button><Button onClick={() => setSenderAction("revoke")}><ShieldOff className="size-4" />Revogar</Button></> : null}
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
            <div><dt className="text-zinc-600">Logs</dt><dd className="mt-1 font-mono text-zinc-300">{formatNumber(details.log_line_count)}</dd></div>
            <div><dt className="text-zinc-600">Tamanho</dt><dd className="mt-1 font-mono text-zinc-300">{formatBytes(details.log_file_size)}</dd></div>
            <div><dt className="text-zinc-600">Última atividade</dt><dd className="mt-1 text-zinc-300">{formatDate(details.last_activity_at)}</dd></div>
            <div><dt className="text-zinc-600">Healthcheck</dt><dd className="mt-1 text-zinc-300">{formatDate(details.last_healthcheck_at)}</dd></div>
            <div><dt className="text-zinc-600">Compactado</dt><dd className="mt-1 text-zinc-300">{formatDate(details.compacted_at)}</dd></div>
            <div><dt className="text-zinc-600">Expira em</dt><dd className="mt-1 text-zinc-300">{formatDate(details.expires_at)}</dd></div>
          </dl>
        </Panel>
      )}

      {error && <ErrorAlert message={error} onRetry={() => void load()} />}

      <Panel className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div className="relative z-20 shrink-0 border-b border-zinc-800 bg-[#161618]">
          <div className="flex min-h-14 flex-wrap items-center gap-2 p-3">
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="Buscar mensagem, evento ou metadata"
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
              Filtros
              <ChevronDown className={`size-3.5 transition-transform ${advancedOpen ? "rotate-180" : ""}`} />
            </Button>
            <Button onClick={() => setPaused((value) => !value)}>
              {paused ? <Play className="size-4" /> : <Pause className="size-4" />}
              {paused ? "Retomar" : "Pausar"}
              {stream.pendingCount > 0 && (
                <span className="rounded-full bg-zinc-700 px-1.5 text-[10px]">
                  {stream.pendingCount}
                </span>
              )}
            </Button>
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
              {loading && logs && (
                <span className="text-[10px] text-zinc-600">Atualizando...</span>
              )}
              <label className="flex items-center gap-1.5 text-[10px] text-zinc-600">
                <Rows3 className="size-3.5" />
                <Listbox
                  value={density}
                  onChange={(value) => {
                    setDensity(value);
                    localStorage.setItem("log-density", value);
                  }}
                  label="Densidade dos logs"
                  size="compact"
                  className="w-28"
                  options={[
                    { value: "compact" as LogDensity, label: "Compacta" },
                    { value: "comfortable" as LogDensity, label: "Confortável" },
                  ]}
                />
              </label>
            </div>
          </div>

          {advancedOpen && (
            <div className="grid gap-3 border-t border-zinc-800 bg-[#111113] p-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
              <label className="text-[11px] text-zinc-500">
                Data inicial
                <span className="mt-1 block">
                  <DateTimePicker
                    value={startDate}
                    onChange={(value) => updateParam("start_date", value)}
                    label="Data inicial"
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                Data final
                <span className="mt-1 block">
                  <DateTimePicker
                    value={endDate}
                    onChange={(value) => updateParam("end_date", value)}
                    label="Data final"
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                Ordem
                <span className="mt-1 block">
                  <Listbox
                    value={order}
                    onChange={(value) => updateParam("order", value)}
                    label="Ordem dos logs"
                    size="compact"
                    className="w-full"
                    options={[
                      { value: "desc", label: "Mais recentes primeiro" },
                      { value: "asc", label: "Mais antigos primeiro" },
                    ]}
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                Presença de evento
                <span className="mt-1 block">
                  <Listbox
                    value={eventMode}
                    onChange={(value) => updateParam("event", value === "all" ? "" : value)}
                    label="Filtrar logs por evento"
                    size="compact"
                    className="w-full"
                    options={[
                      { value: "all", label: "Todos os logs" },
                      { value: "with", label: "Com evento" },
                      { value: "without", label: "Sem evento" },
                    ]}
                  />
                </span>
              </label>
              <label className="text-[11px] text-zinc-500">
                Chave do evento
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
            {Array.from({ length: 10 }, (_, index) => (
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
        title="Pause os logs em tempo real"
        description="A busca textual e a paginação precisam de uma lista estável. Pause o stream antes de continuar para que novos eventos não alterem os resultados durante a consulta."
        confirmLabel="Pausar e continuar"
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
      <SenderActionDialogs sender={details} action={senderAction} onClose={() => setSenderAction(undefined)} onChanged={setDetails} onDeleted={() => navigate("/senders")} />
    </div>
  );
}
