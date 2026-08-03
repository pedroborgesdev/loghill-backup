import { Database, HardDrive, Radio, Server } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api";
import {
  ErrorAlert,
  MetricCard,
  Panel,
  Skeleton,
  StatusIndicator,
} from "../components/ui";
import { useAppShell } from "../layouts/appShellContext";
import { useCachedState } from "../hooks/useCachedState";
import type { HealthResponse } from "../types/api";
import { formatDate, formatNumber } from "../utils/format";
import { waitForMinimumLoading } from "../utils/minimumLoading";

export function SystemStatusPage() {
  const { refreshToken, setRefreshing } = useAppShell();
  const [health, setHealth] = useCachedState<HealthResponse>(["view", "system-status"]);
  const healthRef = useRef<HealthResponse | undefined>(undefined);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    const hasPrevious = Boolean(healthRef.current);
    const startedAt = performance.now();
    setRefreshing(Boolean(healthRef.current));
    try {
      const response = await api.health();
      if (!hasPrevious) await waitForMinimumLoading(startedAt);
      healthRef.current = response;
      setHealth(response);
      setError("");
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Falha ao consultar o sistema",
      );
    } finally {
      setRefreshing(false);
    }
  }, [setHealth, setRefreshing]);

  useEffect(() => { void load(); }, [load, refreshToken]);

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-xl font-semibold">Status do sistema</h2>
        <p className="mt-1 text-sm text-zinc-500">
          Disponibilidade da API e do armazenamento.
        </p>
      </div>
      {error && <ErrorAlert message={error} onRetry={() => void load()} />}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="API" value={health?.status ?? "—"} hint="Estado atual" icon={<Server className="size-4" />} loading={!health} />
        <MetricCard label="Uptime" value={health ? `${formatNumber(health.uptime_seconds)}s` : "—"} hint="Desde a inicialização" icon={<Radio className="size-4" />} loading={!health} />
        <MetricCard label="Senders" value={health ? formatNumber(health.senders.total) : "—"} hint={`${health?.senders.online ?? 0} online`} icon={<Database className="size-4" />} loading={!health} />
        <MetricCard label="Storage" value={health?.storage.writable ? "Gravável" : "Indisponível"} hint={health?.storage.path ?? "Verificando"} icon={<HardDrive className="size-4" />} loading={!health} />
      </div>
      <Panel className="p-4">
        <div className="flex items-center justify-between border-b border-zinc-800 pb-3">
          <h3 className="text-sm font-medium">Diagnóstico</h3>
          <StatusIndicator
            status={health?.status === "healthy" ? "online" : "neutral"}
            label={health?.status === "healthy" ? "Operacional" : "Verificando"}
          />
        </div>
        {!health ? (
          <div className="space-y-3 pt-4">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-2/3" />
          </div>
        ) : (
          <dl className="grid gap-4 pt-4 text-xs sm:grid-cols-2">
            <div><dt className="text-zinc-600">Última verificação</dt><dd className="mt-1 text-zinc-300">{formatDate(health.time)}</dd></div>
            <div><dt className="text-zinc-600">Diretório de dados</dt><dd className="mt-1 font-mono text-zinc-300">{health.storage.path}</dd></div>
          </dl>
        )}
      </Panel>
    </div>
  );
}
