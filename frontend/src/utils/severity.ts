import type { LogSeverity } from "../types/api";

export const severities: LogSeverity[] = [
  "TRACE",
  "DEBUG",
  "INFO",
  "WARN",
  "ERROR",
  "FATAL",
];

export const severityStyles: Record<
  LogSeverity,
  { badge: string; line: string }
> = {
  TRACE: {
    badge: "border-zinc-800 bg-zinc-900 text-zinc-500",
    line: "border-l-zinc-700",
  },
  DEBUG: {
    badge: "border-zinc-700 bg-zinc-900 text-zinc-300",
    line: "border-l-zinc-600",
  },
  INFO: {
    badge: "border-blue-900 bg-blue-950/40 text-blue-400",
    line: "border-l-blue-800",
  },
  WARN: {
    badge: "border-amber-900 bg-amber-950/40 text-amber-400",
    line: "border-l-amber-700",
  },
  ERROR: {
    badge: "border-red-900 bg-red-950/40 text-red-400",
    line: "border-l-red-700",
  },
  FATAL: {
    badge: "border-rose-900 bg-rose-950/50 text-rose-300",
    line: "border-l-rose-600",
  },
};
