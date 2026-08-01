import {
  Activity,
  AlertTriangle,
  Archive,
  Database,
  Radio,
  RefreshCw,
  Server,
  ShieldAlert,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router-dom";
import { api } from "../api";
import { SenderTable, SenderTableSkeleton } from "../components/SenderTable";
import { CreateSenderButton, CreateSenderDialog, SenderActionDialogs, type SenderAction } from "../components/senders/SenderDialogs";
import { Listbox } from "../components/controls";
import {
  Button,
  ErrorAlert,
  MetricCard,
  Pagination,
  Panel,
  SearchInput,
} from "../components/ui";
import { useDebounce } from "../hooks/useDebounce";
import { useAppShell } from "../layouts/appShellContext";
import type { Sender, SenderPage, Summary } from "../types/api";
import { formatNumber } from "../utils/format";
import { syncSearchParams } from "../utils/query";

const emptySummary: Summary = {
  senders: { total: 0, never_connected: 0, online: 0, inactive: 0, expired: 0, revoked: 0 },
  logs: {
    total: 0,
    last_24_hours: 0,
    errors_last_24_hours: 0,
    fatal_last_24_hours: 0,
  },
};

export function DashboardPage() {
  const location = useLocation();
  const showMetrics = location.pathname === "/";
  const [params, setParams] = useSearchParams();
  const { refreshToken, setRefreshing, setStreamState } = useAppShell();
  const [summary, setSummary] = useState(emptySummary);
  const [data, setData] = useState<SenderPage>();
  const dataRef = useRef<SenderPage | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState(params.get("search") ?? "");
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedSender, setSelectedSender] = useState<Sender>();
  const [senderAction, setSenderAction] = useState<SenderAction>();
  const [autoRefresh, setAutoRefresh] = useState(
    () => Number(localStorage.getItem("dashboard-refresh") ?? 30),
  );
  const debounced = useDebounce(search);
  const page = Math.max(1, Number(params.get("page") ?? 1));
  const pageSize = Math.max(1, Number(params.get("page_size") ?? 25));
  const status = params.get("status") ?? "";

  useEffect(() => {
    setStreamState(null);
  }, [setStreamState]);

  const load = useCallback(async () => {
    const hasPreviousData = Boolean(dataRef.current);
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
      const [nextSummary, senders] = await Promise.all([
        api.summary(),
        api.senders(query.toString()),
      ]);
      setSummary(nextSummary);
      dataRef.current = senders;
      setData(senders);
      setError("");
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Não foi possível carregar os senders.",
      );
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [debounced, page, pageSize, setRefreshing, status]);

  useEffect(() => { void load(); }, [load, refreshToken]);

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(() => void load(), autoRefresh * 1_000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, load]);

  useEffect(() => {
    setParams(
      (current) => syncSearchParams(current, debounced),
      { replace: true },
    );
  }, [debounced, setParams]);

  const metrics = [
    { label: "Senders", value: summary.senders.total, hint: "registrados", icon: <Server className="size-4" /> },
    { label: "Online", value: summary.senders.online, hint: `de ${summary.senders.total} senders`, icon: <Radio className="size-4 text-emerald-500" /> },
    { label: "Inativos", value: summary.senders.inactive, hint: "aguardando expiração", icon: <Archive className="size-4 text-amber-500" /> },
    { label: "Expirados", value: summary.senders.expired, hint: "arquivos removidos", icon: <AlertTriangle className="size-4 text-red-500" /> },
    { label: "Total de logs", value: summary.logs.total, hint: "linhas armazenadas", icon: <Database className="size-4" /> },
    { label: "Últimas 24h", value: summary.logs.last_24_hours, hint: "novas entradas", icon: <Activity className="size-4" /> },
    { label: "Erros 24h", value: summary.logs.errors_last_24_hours, hint: "severity ERROR", icon: <ShieldAlert className="size-4 text-red-500" /> },
    { label: "Fatais 24h", value: summary.logs.fatal_last_24_hours, hint: "severity FATAL", icon: <ShieldAlert className="size-4 text-rose-500" /> },
  ];

  const updateParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.set("page", "1");
    setParams(next);
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold">
            {showMetrics ? "Visão geral" : "Senders"}
          </h2>
          <p className="mt-1 text-sm text-zinc-500">
            {showMetrics
              ? "Saúde e volume dos serviços conectados."
              : "Inventário de origens de logs registradas."}
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <CreateSenderButton onClick={() => setCreateOpen(true)} compact />
          <label className="flex items-center gap-2 text-xs text-zinc-500">
            Atualização
            <Listbox
            value={autoRefresh}
            onChange={(value) => {
              setAutoRefresh(value);
              localStorage.setItem("dashboard-refresh", String(value));
            }}
            label="Intervalo de atualização"
            size="compact"
            className="w-28"
            options={[
              { value: 0, label: "Manual" },
              { value: 15, label: "15 segundos" },
              { value: 30, label: "30 segundos" },
              { value: 60, label: "1 minuto" },
            ]}
            />
          </label>
        </div>
      </div>

      {showMetrics && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {metrics.map((metric) => (
            <MetricCard
              key={metric.label}
              label={metric.label}
              value={formatNumber(metric.value)}
              hint={metric.hint}
              icon={metric.icon}
              loading={!data && loading}
            />
          ))}
        </div>
      )}

      <Panel className="overflow-hidden">
        <div className="flex min-h-14 flex-col gap-2 border-b border-zinc-800 p-3 sm:flex-row sm:items-center">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <div>
              <h3 className="text-sm font-medium text-zinc-200">Senders</h3>
              <p className="hidden text-[11px] text-zinc-600 lg:block">
                Status, atividade e volume armazenado
              </p>
            </div>
            {loading && data && (
              <span className="inline-flex items-center gap-1.5 text-[11px] text-zinc-500">
                <RefreshCw className="size-3 animate-spin" />
                Atualizando
              </span>
            )}
          </div>
          <SearchInput
            value={search}
            onChange={setSearch}
            placeholder="Buscar nome ou identificador"
            className="w-full sm:w-72"
          />
          <Listbox
            value={status}
            onChange={(value) => updateParam("status", value)}
            label="Filtrar por status"
            className="w-full sm:w-44"
            options={[
              { value: "", label: "Todos os status" },
              { value: "never_connected", label: "Nunca conectado" },
              { value: "online", label: "Online" },
              { value: "inactive", label: "Inativo" },
              { value: "expired", label: "Expirado" },
              { value: "revoked", label: "Revogado" },
            ]}
          />
          <Button onClick={() => void load()} disabled={loading} className="sm:px-2.5">
            <RefreshCw className={`size-4 ${loading ? "animate-spin" : ""}`} />
            <span className="sm:hidden xl:inline">Atualizar</span>
          </Button>
        </div>

        {error && (
          <div className="p-3">
            <ErrorAlert message={error} onRetry={() => void load()} />
          </div>
        )}
        {!data ? (
          <SenderTableSkeleton />
        ) : (
          <div className={loading ? "opacity-70 transition-opacity duration-150" : ""}>
            <SenderTable items={data.items} onAction={(sender, action) => { setSelectedSender(sender); setSenderAction(action); }} />
          </div>
        )}
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
          setData((current) => current ? { ...current, items: current.items.filter((item) => item.id !== id), pagination: { ...current.pagination, total: Math.max(0, current.pagination.total - 1) } } : current);
          setSummary((current) => ({ ...current, senders: { ...current.senders, total: Math.max(0, current.senders.total - 1) } }));
        }}
      />
    </div>
  );
}
