import { Database, HardDrive, Radio, Server } from "lucide-react";
import { useEffect } from "react";
import { api } from "../api";
import {
  ErrorAlert,
  MetricCard,
  Panel,
  Skeleton,
  StatusIndicator,
} from "../components/ui";
import { useAppShell } from "../layouts/appShellContext";
import { useAppQuery } from "../hooks/useAppQuery";
import { formatDate, formatNumber } from "../utils/format";

export function SystemStatusPage() {
  const { refreshToken, setRefreshing, setStreamState } = useAppShell();
  const query = useAppQuery(["view", "system-status"], api.health, {
    refetchOnMount: "always",
  });

  useEffect(() => setStreamState(null), [setStreamState]);
  useEffect(() => {
    if (refreshToken > 0) void query.refetch();
  }, [query, refreshToken]);
  useEffect(() => {
    setRefreshing(query.isFetching && Boolean(query.data));
  }, [query.data, query.isFetching, setRefreshing]);

  const health = query.data;
  const error = query.error?.message ?? "";

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-xl font-semibold">System status</h2>
        <p className="mt-1 text-sm text-zinc-500">
          API and storage availability.
        </p>
      </div>
      {error && <ErrorAlert message={error} onRetry={() => void query.refetch()} />}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="API" value={health?.status ?? "—"} hint="Estado atual" icon={<Server className="size-4" />} loading={!health} />
        <MetricCard label="Uptime" value={health ? `${formatNumber(health.uptime_seconds)}s` : "—"} hint="Since startup" icon={<Radio className="size-4" />} loading={!health} />
        <MetricCard label="Senders" value={health ? formatNumber(health.senders.total) : "—"} hint={`${health?.senders.online ?? 0} online`} icon={<Database className="size-4" />} loading={!health} />
        <MetricCard label="Storage" value={health?.storage.writable ? "Writable" : "Unavailable"} hint={health?.storage.path ?? "Checking"} icon={<HardDrive className="size-4" />} loading={!health} />
      </div>
      <Panel className="p-4">
        <div className="flex items-center justify-between border-b border-zinc-800 pb-3">
          <h3 className="text-sm font-medium">Weekdaygnostics</h3>
          <StatusIndicator
            status={health?.status === "healthy" ? "online" : "neutral"}
            label={health?.status === "healthy" ? "Operational" : "Checking"}
          />
        </div>
        {!health ? (
          <div className="space-y-3 pt-4">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-2/3" />
          </div>
        ) : (
          <dl className="grid gap-4 pt-4 text-xs sm:grid-cols-2">
            <div><dt className="text-zinc-600">Last check</dt><dd className="mt-1 text-zinc-300">{formatDate(health.time)}</dd></div>
            <div><dt className="text-zinc-600">Date directory</dt><dd className="mt-1 font-mono text-zinc-300">{health.storage.path}</dd></div>
          </dl>
        )}
      </Panel>
    </div>
  );
}
