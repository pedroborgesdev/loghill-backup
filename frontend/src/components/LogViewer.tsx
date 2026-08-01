import {
  ArrowUp,
  Braces,
  ChevronDown,
  ChevronUp,
  Clipboard,
  Copy,
  FileJson,
  Radio,
} from "lucide-react";
import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { DisplayLogEntry, StreamState } from "../hooks/useLogStream";
import { formatDate, formatLogTime } from "../utils/format";
import { isLogScrollKey, shouldFreezeLogViewport } from "../utils/logScroll";
import { severityStyles } from "../utils/severity";
import { Button, EmptyState, IconButton, StatusIndicator } from "./ui";
import { EventBadge } from "./events/EventBadge";

export type LogDensity = "compact" | "comfortable";

function SeverityBadge({
  severity,
  compact,
}: {
  severity: DisplayLogEntry["severity"];
  compact: boolean;
}) {
  return (
    <span
      className={`inline-flex items-center justify-center border font-semibold ${
        compact
          ? "h-4 w-14 rounded-sm text-[9px]"
          : "h-5 w-[62px] rounded text-[10px]"
      } ${severityStyles[severity].badge}`}
    >
      {severity}
    </span>
  );
}

const LogRow = memo(function LogRow({
  entry,
  density,
  metadataExpanded,
  messageExpanded,
  onToggleMetadata,
  onToggleMessage,
}: {
  entry: DisplayLogEntry;
  density: LogDensity;
  metadataExpanded: boolean;
  messageExpanded: boolean;
  onToggleMetadata: () => void;
  onToggleMessage: () => void;
}) {
  const hasMetadata = Boolean(entry.metadata && Object.keys(entry.metadata).length);
  const longMessage = entry.message.length > 140 || entry.message.includes("\n");
  const compact = density === "compact";
  return (
    <article
      className={`border-b border-l-2 border-b-zinc-800/80 bg-[#111113] hover:bg-zinc-900/70 ${severityStyles[entry.severity].line}`}
    >
      <div
        className={`grid min-w-0 items-start font-mono ${
          compact
            ? "grid-cols-[58px_58px_minmax(0,1fr)_26px_26px] gap-1.5 px-2.5 py-0.5 text-[11px] leading-4 sm:grid-cols-[78px_58px_minmax(0,1fr)_26px_26px]"
            : "grid-cols-[64px_68px_minmax(0,1fr)_32px_32px] gap-2 px-3 py-1.5 text-xs leading-5 sm:grid-cols-[86px_68px_minmax(0,1fr)_32px_32px]"
        }`}
      >
        <time
          className={`whitespace-nowrap tabular-nums text-zinc-600 ${
            compact ? "text-[10px]" : "text-[11px]"
          }`}
          title={formatDate(entry.timestamp)}
        >
          <span className="sm:hidden">{formatLogTime(entry.timestamp, true)}</span>
          <span className="hidden sm:inline">{formatLogTime(entry.timestamp)}</span>
        </time>
        <SeverityBadge severity={entry.severity} compact={compact} />
        <div className="min-w-0">
          <div className={`min-w-0 ${entry.event ? "flex items-start gap-2" : ""}`}>
            {entry.event && <EventBadge eventKey={entry.event} />}
            <p
              className={`min-w-0 flex-1 ${
                messageExpanded
                  ? "whitespace-pre-wrap break-words text-zinc-300"
                  : "truncate text-zinc-300"
              }`}
              title={messageExpanded ? undefined : entry.message}
            >
              {entry.message}
            </p>
          </div>
          {longMessage && (
            <button
              type="button"
              onClick={onToggleMessage}
              className="mt-1 inline-flex items-center gap-1 text-[10px] text-zinc-600 hover:text-zinc-300 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
              aria-expanded={messageExpanded}
            >
              {messageExpanded ? <ChevronUp className="size-3" /> : <ChevronDown className="size-3" />}
              {messageExpanded ? "Recolher" : "Expandir"}
            </button>
          )}
        </div>
        <IconButton
          label={hasMetadata ? "Exibir metadata" : "Sem metadata"}
          className={compact ? "!size-6 !rounded" : "!size-7"}
          disabled={!hasMetadata}
          aria-expanded={metadataExpanded}
          onClick={onToggleMetadata}
        >
          <Braces className={compact ? "size-3" : "size-3.5"} />
        </IconButton>
        <IconButton
          label="Copiar mensagem"
          className={compact ? "!size-6 !rounded" : "!size-7"}
          onClick={() => void navigator.clipboard.writeText(entry.message)}
        >
          <Copy className={compact ? "size-3" : "size-3.5"} />
        </IconButton>
      </div>
      {metadataExpanded && hasMetadata && (
        <div className="mx-3 mb-2 rounded-lg border border-zinc-800 bg-[#09090b] p-3">
          <div className="mb-2 flex items-center justify-between gap-2">
            <span className="text-[10px] uppercase tracking-wider text-zinc-600">Metadata</span>
            <div className="flex items-center gap-1">
              <IconButton
                label="Copiar metadata"
                className="size-7"
                onClick={() =>
                  void navigator.clipboard.writeText(
                    JSON.stringify(entry.metadata, null, 2),
                  )
                }
              >
                <Clipboard className="size-3.5" />
              </IconButton>
              <IconButton
                label="Copiar JSON completo"
                className="size-7"
                onClick={() =>
                  void navigator.clipboard.writeText(JSON.stringify(entry, null, 2))
                }
              >
                <FileJson className="size-3.5" />
              </IconButton>
            </div>
          </div>
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-xs leading-5 text-zinc-400">
            {JSON.stringify(entry.metadata, null, 2)}
          </pre>
        </div>
      )}
    </article>
  );
});

export function LogViewer({
  entries,
  density,
  streamState,
  liveCount,
  autoScroll,
  onAutoScrollChange,
}: {
  entries: DisplayLogEntry[];
  density: LogDensity;
  streamState: StreamState;
  liveCount: number;
  autoScroll: boolean;
  onAutoScrollChange: (enabled: boolean) => void;
}) {
  const scrollElement = useRef<HTMLDivElement>(null);
  const rowsElement = useRef<HTMLDivElement>(null);
  const viewportAnchor = useRef<{ key: string; offset: number } | null>(null);
  const [metadataExpanded, setMetadataExpanded] = useState<Set<string>>(new Set());
  const [messagesExpanded, setMessagesExpanded] = useState<Set<string>>(new Set());
  const [unseenLogCount, setUnseenLogCount] = useState(0);
  const previousLiveCount = useRef(liveCount);
  const userInteracting = useRef(false);
  const manualScrollIntent = useRef(false);
  const pinnedToLatest = useRef(true);
  const following = useRef(autoScroll);
  const interactionTimer = useRef<number | null>(null);
  const compactView = density === "compact";
  following.current = autoScroll;

  const captureViewportAnchor = useCallback(() => {
    const container = scrollElement.current;
    const rows = rowsElement.current?.children;
    if (!container || !rows?.length) {
      viewportAnchor.current = null;
      return;
    }

    const scrollTop = container.scrollTop;
    let low = 0;
    let high = rows.length - 1;
    let candidate = 0;

    while (low <= high) {
      const middle = Math.floor((low + high) / 2);
      const row = rows.item(middle) as HTMLElement;
      if (row.offsetTop <= scrollTop) {
        candidate = middle;
        low = middle + 1;
      } else {
        high = middle - 1;
      }
    }

    let row = rows.item(candidate) as HTMLElement;
    if (
      row.offsetTop + row.offsetHeight <= scrollTop &&
      candidate + 1 < rows.length
    ) {
      row = rows.item(candidate + 1) as HTMLElement;
    }

    const key = row.dataset.logKey;
    viewportAnchor.current = key
      ? { key, offset: row.offsetTop - scrollTop }
      : null;
  }, []);

  useLayoutEffect(() => {
    const container = scrollElement.current;
    const rows = rowsElement.current?.children;
    const anchor = viewportAnchor.current;

    if (!autoScroll && container && rows && anchor) {
      let anchorRow: HTMLElement | null = null;
      for (let index = 0; index < rows.length; index += 1) {
        const row = rows.item(index) as HTMLElement;
        if (row.dataset.logKey === anchor.key) {
          anchorRow = row;
          break;
        }
      }

      if (anchorRow) {
        const nextScrollTop = Math.max(0, anchorRow.offsetTop - anchor.offset);
        if (Math.abs(container.scrollTop - nextScrollTop) > 0.5) {
          container.scrollTop = nextScrollTop;
        }
      }
    }

    captureViewportAnchor();
  }, [
    autoScroll,
    captureViewportAnchor,
    density,
    entries,
    messagesExpanded,
    metadataExpanded,
  ]);

  useEffect(
    () => () => {
      if (interactionTimer.current !== null) {
        window.clearTimeout(interactionTimer.current);
      }
    },
    [],
  );

  useEffect(() => {
    const delta = Math.max(0, liveCount - previousLiveCount.current);
    const streamWasReset = liveCount < previousLiveCount.current;
    previousLiveCount.current = liveCount;

    if (streamWasReset || liveCount === 0 || streamState === "paused") {
      setUnseenLogCount(0);
      return;
    }

    if (delta > 0) {
      const shouldFreeze = shouldFreezeLogViewport({
        autoScroll: following.current,
        pinnedToLatest: pinnedToLatest.current,
        userInteracting: userInteracting.current,
      });
      if (shouldFreeze) {
        setUnseenLogCount((current) => current + delta);
      } else {
        setUnseenLogCount(0);
      }
      return;
    }
  }, [autoScroll, entries, liveCount, streamState]);

  const goToLatest = (smooth: boolean) => {
    pinnedToLatest.current = true;
    setUnseenLogCount(0);
    window.requestAnimationFrame(() => {
      scrollElement.current?.scrollTo({
        top: 0,
        behavior: smooth ? "smooth" : "auto",
      });
    });
  };

  const beginUserInteraction = (scrollIntent = false) => {
    userInteracting.current = true;
    if (scrollIntent) {
      manualScrollIntent.current = true;
    }
    if (interactionTimer.current !== null) {
      window.clearTimeout(interactionTimer.current);
    }
  };

  const disableFollow = () => {
    if (!following.current) return;
    following.current = false;
    onAutoScrollChange(false);
  };

  const finishUserInteraction = () => {
    if (interactionTimer.current !== null) {
      window.clearTimeout(interactionTimer.current);
    }
    interactionTimer.current = window.setTimeout(() => {
      userInteracting.current = false;
      manualScrollIntent.current = false;
      pinnedToLatest.current = (scrollElement.current?.scrollTop ?? 0) <= 2;
    }, 180);
  };

  const toggleFollow = () => {
    if (following.current) {
      disableFollow();
      return;
    }
    following.current = true;
    onAutoScrollChange(true);
    goToLatest(false);
  };

  const toggle = (
    setter: React.Dispatch<React.SetStateAction<Set<string>>>,
    id: string,
  ) =>
    setter((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const connection = useMemo(
    () => ({
      connected: { status: "online" as const, label: "Ao vivo" },
      reconnecting: { status: "warning" as const, label: "Reconectando" },
      paused: { status: "neutral" as const, label: "Pausado" },
      disconnected: { status: "offline" as const, label: "Offline" },
      error: { status: "offline" as const, label: "Erro no stream" },
    })[streamState],
    [streamState],
  );

  const consoleToolbar = (
    <div className="flex min-h-11 items-center justify-between gap-4 border-b border-zinc-800 bg-[#1c1c1f] px-4 py-1.5">
      <span className="text-[10px] uppercase tracking-wider text-zinc-600">
        Console
      </span>
      <div className="flex items-center gap-4">
        <StatusIndicator {...connection} />
        <Button
          aria-pressed={autoScroll}
          onClick={toggleFollow}
          className={`h-8 rounded-md px-3 text-xs ${
            autoScroll
              ? "border-cyan-800 bg-cyan-950/70 text-cyan-300 hover:border-cyan-700 hover:bg-cyan-950"
              : "border-zinc-700 bg-zinc-900 text-zinc-400"
          }`}
        >
          <Radio className={`size-3.5 ${autoScroll ? "animate-pulse" : ""}`} />
          Follow
        </Button>
      </div>
    </div>
  );

  if (!entries.length) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        {consoleToolbar}
        <EmptyState
          title="Nenhum log para os filtros selecionados"
          description="Limpe os filtros ou escolha outro período."
        />
      </div>
    );
  }

  return (
    <div className="relative flex h-full min-h-0 flex-col">
      {consoleToolbar}
      <div
        className={`grid shrink-0 items-center border-b border-zinc-700 bg-[#1c1c1f] font-mono uppercase tracking-wider text-zinc-500 ${
          compactView
            ? "h-7 grid-cols-[58px_58px_minmax(0,1fr)_26px_26px] gap-1.5 px-2.5 text-[9px] sm:grid-cols-[78px_58px_minmax(0,1fr)_26px_26px]"
            : "h-8 grid-cols-[64px_68px_minmax(0,1fr)_32px_32px] gap-2 px-3 text-[10px] sm:grid-cols-[86px_68px_minmax(0,1fr)_32px_32px]"
        }`}
      >
        <span>Horário</span><span>Severity</span><span>Mensagem</span>
        <span className="sr-only">Metadata</span><span className="sr-only">Ações</span>
      </div>
      <div
        ref={scrollElement}
        role="log"
        aria-label="Logs"
        tabIndex={0}
        className="min-h-0 flex-1 overflow-auto bg-[#111113]"
        style={{ overflowAnchor: "none" }}
        onPointerDown={(event) =>
          beginUserInteraction(event.target === event.currentTarget)
        }
        onPointerUp={finishUserInteraction}
        onPointerCancel={finishUserInteraction}
        onTouchStart={() => beginUserInteraction()}
        onTouchEnd={finishUserInteraction}
        onTouchMove={() => {
          beginUserInteraction(true);
          disableFollow();
        }}
        onWheel={() => {
          beginUserInteraction(true);
          disableFollow();
          finishUserInteraction();
        }}
        onKeyDown={(event) => {
          if (!isLogScrollKey(event.key)) return;
          beginUserInteraction(true);
          disableFollow();
          finishUserInteraction();
        }}
        onScroll={(event) => {
          pinnedToLatest.current = event.currentTarget.scrollTop <= 2;
          if (manualScrollIntent.current) {
            disableFollow();
          }
          captureViewportAnchor();
        }}
      >
        <div ref={rowsElement} className="relative w-full">
          {entries.map((entry) => (
            <div key={entry.ui_id} data-log-key={entry.ui_id}>
              <LogRow
                entry={entry}
                density={density}
                metadataExpanded={metadataExpanded.has(entry.ui_id)}
                messageExpanded={messagesExpanded.has(entry.ui_id)}
                onToggleMetadata={() => toggle(setMetadataExpanded, entry.ui_id)}
                onToggleMessage={() => toggle(setMessagesExpanded, entry.ui_id)}
              />
            </div>
          ))}
        </div>
      </div>
      <div className="pointer-events-none absolute bottom-3 right-4 flex flex-col items-end gap-2">
        {unseenLogCount > 0 && (
          <button
            type="button"
            className="pointer-events-auto inline-flex h-8 items-center gap-2 rounded-lg border border-zinc-700 bg-zinc-900 px-3 text-xs text-zinc-200 shadow-lg shadow-black/30 hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
            onClick={() => goToLatest(true)}
          >
            <ArrowUp className="size-3.5" />
            {unseenLogCount} novos logs
          </button>
        )}
        <IconButton
          label="Ir para os logs mais recentes"
          className="pointer-events-auto border-zinc-700 bg-zinc-900 shadow-lg shadow-black/30"
          onClick={() => goToLatest(true)}
        >
          <ArrowUp className="size-4" />
        </IconButton>
      </div>
    </div>
  );
}
