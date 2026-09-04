import { useCallback, useEffect, useRef, useState } from "react";
import type { LogEntry, LogSeverity } from "../types/api";

export type StreamState =
  | "connected"
  | "reconnecting"
  | "paused"
  | "disconnected"
  | "error";

export interface DisplayLogEntry extends LogEntry {
  ui_id: string;
}

export function logSignature(entry: LogEntry) {
  return `${entry.timestamp}|${entry.instance_id ?? ""}|${entry.severity}|${entry.event ?? ""}|${entry.event_occurrence_id ?? ""}|${entry.message}|${JSON.stringify(entry.metadata ?? {})}`;
}

export function prepareLogEntries(
  entries: LogEntry[],
  prefix = "api",
): DisplayLogEntry[] {
  const occurrences = new Map<string, number>();
  return entries.map((entry) => {
    const signature = logSignature(entry);
    const occurrence = occurrences.get(signature) ?? 0;
    occurrences.set(signature, occurrence + 1);
    return {
      ...entry,
      ui_id:
        "ui_id" in entry && typeof entry.ui_id === "string"
          ? entry.ui_id
          : `${prefix}:${signature}:${occurrence}`,
    };
  });
}

export function useLogStream(
  sender: string,
  severities: LogSeverity[],
  paused: boolean,
  instanceID = "",
  max = 1_000,
) {
  const [entries, setEntries] = useState<DisplayLogEntry[]>([]);
  const [state, setState] = useState<StreamState>("disconnected");
  const [pendingCount, setPendingCount] = useState(0);
  const [receivedCount, setReceivedCount] = useState(0);
  const queue = useRef<DisplayLogEntry[]>([]);
  const seen = useRef(new Set<string>());
  const seenOrder = useRef<string[]>([]);
  const sequence = useRef(0);
  const pausedRef = useRef(paused);
  const connectedRef = useRef(false);
  const severityKey = severities.join(",");

  useEffect(() => {
    pausedRef.current = paused;
    if (paused) {
      setState("paused");
      setPendingCount(queue.current.length);
    } else {
      setState(connectedRef.current ? "connected" : "reconnecting");
    }
  }, [paused]);

  useEffect(() => {
    queue.current = [];
    seen.current.clear();
    seenOrder.current = [];
    sequence.current = 0;
    setEntries([]);
    setPendingCount(0);
    setReceivedCount(0);
    if (!sender) {
      connectedRef.current = false;
      setState(pausedRef.current ? "paused" : "disconnected");
      return;
    }
    const params = new URLSearchParams();
    if (severityKey) params.set("severity", severityKey);
    if (instanceID) params.set("instance_id", instanceID);
    const query = params.size ? `?${params}` : "";
    const source = new EventSource(
      `/api/v1/senders/${encodeURIComponent(sender)}/logs/stream${query}`,
      { withCredentials: true },
    );
    connectedRef.current = false;
    setState(pausedRef.current ? "paused" : "reconnecting");

    source.addEventListener("status", () => {
      connectedRef.current = true;
      if (!pausedRef.current) setState("connected");
    });

    source.addEventListener("log", (event) => {
      const entry = JSON.parse((event as MessageEvent<string>).data) as LogEntry;
      const signature = logSignature(entry);
      if (seen.current.has(signature)) return;
      seen.current.add(signature);
      seenOrder.current.push(signature);
      if (seenOrder.current.length > max * 2) {
        const oldest = seenOrder.current.shift();
        if (oldest) seen.current.delete(oldest);
      }
      sequence.current += 1;
      queue.current.push({
        ...entry,
        ui_id: `sse:${sequence.current}:${signature}`,
      });
      if (queue.current.length > max) queue.current.splice(0, queue.current.length - max);
      if (pausedRef.current) setPendingCount(queue.current.length);
    });

    source.onerror = () => {
      connectedRef.current = false;
      if (!pausedRef.current) {
        setState(
          source.readyState === EventSource.CLOSED ? "error" : "reconnecting",
        );
      }
    };

    const batchTimer = window.setInterval(() => {
      if (pausedRef.current || queue.current.length === 0) return;
      const batch = queue.current.splice(0).reverse();
      setPendingCount(0);
      setReceivedCount((current) => current + batch.length);
      setEntries((current) => [...batch, ...current].slice(0, max));
    }, 150);

    return () => {
      window.clearInterval(batchTimer);
      source.close();
      connectedRef.current = false;
    };
  }, [instanceID, max, sender, severityKey]);

  const clear = useCallback(() => {
    queue.current = [];
    seen.current.clear();
    seenOrder.current = [];
    setEntries([]);
    setPendingCount(0);
    setReceivedCount(0);
  }, []);

  return { entries, state, pendingCount, receivedCount, clear };
}
