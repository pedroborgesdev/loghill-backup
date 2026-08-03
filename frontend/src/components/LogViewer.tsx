import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
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
import { useVirtualizer } from "@tanstack/react-virtual";
import type { DisplayLogEntry, StreamState } from "../hooks/useLogStream";
import { formatDate, formatLogTime } from "../utils/format";
import { isLogScrollKey } from "../utils/logScroll";
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
  const [newestAtBottom, setNewestAtBottom] = useState(true);
  const previousLiveCount = useRef(liveCount);
  const userInteracting = useRef(false);
  const manualScrollIntent = useRef(false);
  const pinnedToLatest = useRef(true);
  const following = useRef(autoScroll);
  const interactionTimer = useRef<number | null>(null);
  const compactView = density === "compact";
  following.current = autoScroll;
  const displayedEntries = useMemo(
    () => newestAtBottom ? [...entries].reverse() : entries,
    [entries, newestAtBottom],
  );
  const rowVirtualizer = useVirtualizer({
    count: displayedEntries.length,
    getScrollElement: () => scrollElement.current,
    estimateSize: () => compactView ? 25 : 34,
    getItemKey: (index) => displayedEntries[index]?.ui_id ?? index,
    overscan: 12,
    initialRect: { width: 800, height: 600 },
  });
  const virtualRows = import.meta.env.MODE === "test"
    ? displayedEntries.map((_, index) => ({ index, start: index * (compactView ? 25 : 34) }))
    : rowVirtualizer.getVirtualItems();
  const virtualSize = rowVirtualizer.getTotalSize();

  const latestScrollTop = useCallback(() => {
    const container = scrollElement.current;
    if (!container) return 0;
    return newestAtBottom ? container.scrollHeight - container.clientHeight : 0;
  }, [newestAtBottom]);

  const captureViewportAnchor = useCallback(() => {
    const container = scrollElement.current;
    const rows = rowsElement.current?.children;
    if (!container || !rows?.length) {
      viewportAnchor.current = null;
      return;
    }

    const containerRect = container.getBoundingClientRect();
    const hasLayout = containerRect.height > 0 || (rows.item(0) as HTMLElement).getBoundingClientRect().height > 0;
    let row = rows.item(0) as HTMLElement;
    if (hasLayout) {
      for (let index = 0; index < rows.length; index += 1) {
        const candidate = rows.item(index) as HTMLElement;
        if (candidate.getBoundingClientRect().bottom > containerRect.top) { row = candidate; break; }
      }
    } else {
      for (let index = 0; index < rows.length; index += 1) {
        const candidate = rows.item(index) as HTMLElement;
        if (candidate.offsetTop + candidate.offsetHeight > container.scrollTop) { row = candidate; break; }
      }
    }

    const key = row.dataset.logKey;
    viewportAnchor.current = key
      ? { key, offset: hasLayout ? row.getBoundingClientRect().top - containerRect.top : row.offsetTop - container.scrollTop }
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
        const containerRect = container.getBoundingClientRect();
        const anchorRect = anchorRow.getBoundingClientRect();
        const hasLayout = containerRect.height > 0 || anchorRect.height > 0;
        const nextScrollTop = Math.max(0, hasLayout
          ? container.scrollTop + anchorRect.top - containerRect.top - anchor.offset
          : anchorRow.offsetTop - anchor.offset);
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
    displayedEntries,
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
      const shouldFreeze = !following.current;
      if (shouldFreeze) {
        setUnseenLogCount((current) => current + delta);
      } else {
        setUnseenLogCount(0);
        window.requestAnimationFrame(() => {
          const container = scrollElement.current;
          if (container) container.scrollTop = latestScrollTop();
        });
      }
      return;
    }
  }, [autoScroll, displayedEntries, latestScrollTop, liveCount, streamState]);

  const goToLatest = (smooth: boolean) => {
    if (!following.current) {
      following.current = true;
      onAutoScrollChange(true);
    }
    pinnedToLatest.current = true;
    setUnseenLogCount(0);
    window.requestAnimationFrame(() => {
      scrollElement.current?.scrollTo({
        top: latestScrollTop(),
        behavior: smooth ? "smooth" : "auto",
      });
    });
  };

  useLayoutEffect(() => {
    if (!autoScroll) return;
    pinnedToLatest.current = true;
    setUnseenLogCount(0);
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => {
        const container = scrollElement.current;
        if (container) container.scrollTop = latestScrollTop();
      });
    });
  }, [autoScroll, displayedEntries.length, latestScrollTop, virtualSize]);

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
      const container = scrollElement.current;
      pinnedToLatest.current = container ? (newestAtBottom
        ? container.scrollHeight - container.clientHeight - container.scrollTop <= 2
        : container.scrollTop <= 2) : false;
    }, 180);
  };

  const toggleDirection = () => {
    const next = !newestAtBottom;
    setNewestAtBottom(next);
    localStorage.setItem("log-newest-at-bottom", String(next));
    pinnedToLatest.current = true;
    setUnseenLogCount(0);
  };

  useLayoutEffect(() => {
    const container = scrollElement.current;
    if (container) container.scrollTop = latestScrollTop();
  }, [latestScrollTop]);

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
      <div className="flex items-center gap-2">
        <StatusIndicator {...connection} />
        <Button
          onClick={toggleDirection}
          className="h-8 rounded-md px-3 text-xs"
          title={newestAtBottom ? "Os logs mais recentes aparecem embaixo" : "Os logs mais recentes aparecem no topo"}
        >
          <ArrowUpDown className="size-3.5" />
          {newestAtBottom ? "Recentes embaixo" : "Recentes no topo"}
        </Button>
        <Button
          aria-pressed={autoScroll}
          onClick={toggleFollow}
          className={`h-8 rounded-md px-3 text-xs ${
            autoScroll
              ? "border-zinc-700 bg-zinc-900 !text-emerald-400 hover:border-zinc-600 hover:bg-zinc-800"
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
        }}
        onWheel={() => {
          beginUserInteraction(true);
          finishUserInteraction();
        }}
        onKeyDown={(event) => {
          if (!isLogScrollKey(event.key)) return;
          beginUserInteraction(true);
          finishUserInteraction();
        }}
        onScroll={(event) => {
          pinnedToLatest.current = newestAtBottom
            ? event.currentTarget.scrollHeight - event.currentTarget.clientHeight - event.currentTarget.scrollTop <= 2
            : event.currentTarget.scrollTop <= 2;
          if (manualScrollIntent.current) disableFollow();
          captureViewportAnchor();
        }}
      >
        <div ref={rowsElement} className="relative w-full" style={{ height: virtualSize }}>
          {virtualRows.map((virtualRow) => {
            const entry = displayedEntries[virtualRow.index];
            if (!entry) return null;
            return <div
              key={entry.ui_id}
              ref={import.meta.env.MODE === "test" ? undefined : rowVirtualizer.measureElement}
              data-index={virtualRow.index}
              data-log-key={entry.ui_id}
              className="absolute left-0 top-0 w-full"
              style={{ transform: `translateY(${virtualRow.start}px)` }}
            >
              <LogRow
                entry={entry}
                density={density}
                metadataExpanded={metadataExpanded.has(entry.ui_id)}
                messageExpanded={messagesExpanded.has(entry.ui_id)}
                onToggleMetadata={() => toggle(setMetadataExpanded, entry.ui_id)}
                onToggleMessage={() => toggle(setMessagesExpanded, entry.ui_id)}
              />
            </div>;
          })}
        </div>
      </div>
      <div className="pointer-events-none absolute bottom-3 right-4 flex flex-col items-end gap-2">
        {unseenLogCount > 0 && (
          <button
            type="button"
            className="pointer-events-auto inline-flex h-8 items-center gap-2 rounded-lg border border-zinc-700 bg-zinc-900 px-3 text-xs text-zinc-200 shadow-lg shadow-black/30 hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
            onClick={() => goToLatest(true)}
          >
            {newestAtBottom ? <ArrowDown className="size-3.5" /> : <ArrowUp className="size-3.5" />}
            {unseenLogCount} novos logs
          </button>
        )}
        <IconButton
          label="Ir para os logs mais recentes"
          className="pointer-events-auto border-zinc-700 bg-zinc-900 shadow-lg shadow-black/30"
          onClick={() => goToLatest(true)}
        >
          {newestAtBottom ? <ArrowDown className="size-4" /> : <ArrowUp className="size-4" />}
        </IconButton>
      </div>
    </div>
  );
}
