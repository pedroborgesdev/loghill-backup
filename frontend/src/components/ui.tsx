import {
  ChevronLeft,
  ChevronRight,
  Search,
  X,
} from "lucide-react";
import type {
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  PropsWithChildren,
  ReactNode,
} from "react";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import type { SenderStatus } from "../types/api";
import { Listbox } from "./controls";

export function Panel({
  children,
  className = "",
}: PropsWithChildren<{ className?: string }>) {
  return (
    <section
      className={`rounded-xl border border-zinc-800 bg-[#161618] ${className}`}
    >
      {children}
    </section>
  );
}

export const Card = Panel;

export function Button({
  className = "",
  type = "button",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type={type}
      className={`inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-lg border border-zinc-700 bg-zinc-900 px-3 text-sm font-medium text-zinc-200 transition-colors duration-150 ease-out hover:border-zinc-600 hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:opacity-50 disabled:text-zinc-500 ${className}`}
      {...props}
    />
  );
}

export function IconButton({
  label,
  className = "",
  tooltipClassName,
  children,
  ...props
}: PropsWithChildren<
  ButtonHTMLAttributes<HTMLButtonElement> & { label: string; tooltipClassName?: string }
>) {
  return (
    <Tooltip label={label} className={tooltipClassName}>
      <button
        type="button"
        aria-label={label}
        className={`inline-grid size-9 shrink-0 place-items-center rounded-lg border border-transparent text-zinc-400 transition-colors duration-150 hover:border-zinc-700 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${className}`}
        {...props}
      >
        {children}
      </button>
    </Tooltip>
  );
}

export function Input({ className = "", ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`h-9 rounded-lg border border-zinc-700 bg-zinc-950 px-3 text-sm text-zinc-100 outline-none placeholder:text-zinc-600 focus:ring-1 focus:ring-white/50 ${className}`}
      {...props}
    />
  );
}

export function SearchInput({
  value,
  onChange,
  placeholder = "Buscar...",
  className = "",
  blocked = false,
  onBlocked,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  blocked?: boolean;
  onBlocked?: () => void;
}) {
  return (
    <label className={`relative block ${className}`}>
      <span className="sr-only">{placeholder}</span>
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-zinc-600"
      />
      <Input
        value={value}
        readOnly={blocked}
        aria-readonly={blocked}
        onFocus={() => {
          if (blocked) onBlocked?.();
        }}
        onChange={(event) => {
          if (blocked) onBlocked?.();
          else onChange(event.target.value);
        }}
        placeholder={placeholder}
        className={`w-full pl-9 pr-9 ${blocked ? "cursor-pointer" : ""}`}
      />
      {value && (
        <button
          type="button"
          aria-label="Limpar busca"
          onClick={() => {
            if (blocked) onBlocked?.();
            else onChange("");
          }}
          className="absolute right-2 top-1/2 grid size-6 -translate-y-1/2 place-items-center rounded text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
        >
          <X className="size-3.5" />
        </button>
      )}
    </label>
  );
}

const statusStyle: Record<SenderStatus, string> = {
  never_connected: "border-sky-900 bg-sky-950/40 text-sky-400",
  online: "border-emerald-900 bg-emerald-950/50 text-emerald-400",
  inactive: "border-amber-900 bg-amber-950/40 text-amber-400",
  archived: "border-zinc-700 bg-zinc-900 text-zinc-400",
  expired: "border-red-900 bg-red-950/40 text-red-400",
  revoked: "border-rose-900 bg-rose-950/40 text-rose-400",
};

const statusLabel: Record<SenderStatus, string> = {
  never_connected: "Nunca conectado",
  online: "Online",
  inactive: "Inativo",
  archived: "Arquivado",
  expired: "Expirado",
  revoked: "Revogado",
};

export function StatusBadge({ status }: { status: SenderStatus }) {
  return (
    <span
      className={`inline-flex h-6 items-center gap-1.5 rounded-full border px-2 text-xs font-medium ${statusStyle[status]}`}
    >
      <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
      {statusLabel[status]}
    </span>
  );
}

export function StatusIndicator({
  status,
  label,
}: {
  status: "online" | "warning" | "offline" | "neutral";
  label: string;
}) {
  const color = {
    online: "bg-emerald-400",
    warning: "bg-amber-400",
    offline: "bg-red-400",
    neutral: "bg-zinc-500",
  }[status];
  return (
    <span className="inline-flex items-center gap-2 whitespace-nowrap text-xs text-zinc-400">
      <span className={`size-1.5 rounded-full ${color}`} aria-hidden="true" />
      {label}
    </span>
  );
}

export function ErrorAlert({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div
      role="alert"
      className="flex min-h-12 items-center justify-between gap-3 rounded-lg border border-red-950 bg-red-950/20 px-4 py-3 text-sm text-red-300"
    >
      <span>{message}</span>
      {onRetry && (
        <Button onClick={onRetry} className="h-8 border-red-900 bg-transparent">
          Tentar novamente
        </Button>
      )}
    </div>
  );
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  onConfirm,
  onClose,
}: {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const titleId = useId();
  const descriptionId = useId();
  const confirmButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    confirmButton.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose, open]);

  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-[120] grid place-items-center p-4">
      <button
        type="button"
        aria-label="Fechar diálogo"
        className="absolute inset-0 bg-black/75"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        className="relative w-full max-w-md rounded-xl border border-zinc-700 bg-[#161618] p-5 shadow-2xl shadow-black/70"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 id={titleId} className="text-base font-semibold text-zinc-100">
              {title}
            </h2>
            <p id={descriptionId} className="mt-2 text-sm leading-6 text-zinc-500">
              {description}
            </p>
          </div>
          <button
            type="button"
            aria-label="Fechar"
            onClick={onClose}
            className="grid size-8 shrink-0 place-items-center rounded-lg text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
          >
            <X className="size-4" />
          </button>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button onClick={onClose} className="border-transparent bg-transparent">
            Cancelar
          </Button>
          <button
            type="button"
            ref={confirmButton}
            onClick={onConfirm}
            className="inline-flex h-9 items-center justify-center rounded-lg border border-zinc-500 bg-zinc-800 px-3 text-sm font-medium text-zinc-100 hover:border-zinc-400 hover:bg-zinc-700 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

export function EmptyState({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div className="grid min-h-48 place-items-center px-4 text-center">
      <div>
        <p className="text-sm font-medium text-zinc-300">{title}</p>
        <p className="mt-1 text-xs text-zinc-500">{description}</p>
      </div>
    </div>
  );
}

export function Skeleton({ className = "" }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`block rounded-md bg-zinc-800/70 ${className}`}
    />
  );
}

const tooltipOpenEvent = "loghill:tooltip-open";

export function Tooltip({
  label,
  children,
  className = "inline-flex",
}: PropsWithChildren<{ label: string; className?: string }>) {
  const id = useId();
  const anchor = useRef<HTMLSpanElement>(null);
  const tooltip = useRef<HTMLSpanElement>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({
    left: 0,
    top: 0,
    ready: false,
  });

  const hide = useCallback(() => setOpen(false), []);
  const show = useCallback(() => {
    if (!label || typeof window === "undefined") return;
    window.dispatchEvent(
      new CustomEvent(tooltipOpenEvent, { detail: id }),
    );
    setPosition((current) => ({ ...current, ready: false }));
    setOpen(true);
  }, [id, label]);

  const updatePosition = useCallback(() => {
    if (!anchor.current || !tooltip.current || typeof window === "undefined") {
      return;
    }

    const margin = 8;
    const gap = 6;
    const anchorRect = anchor.current.getBoundingClientRect();
    const tooltipRect = tooltip.current.getBoundingClientRect();
    const maximumLeft = Math.max(
      margin,
      window.innerWidth - tooltipRect.width - margin,
    );
    const maximumTop = Math.max(
      margin,
      window.innerHeight - tooltipRect.height - margin,
    );
    const preferredTop = anchorRect.bottom + gap;
    const top =
      preferredTop + tooltipRect.height <= window.innerHeight - margin
        ? preferredTop
        : anchorRect.top - tooltipRect.height - gap;

    setPosition({
      left: Math.min(
        Math.max(anchorRect.left + anchorRect.width / 2 - tooltipRect.width / 2, margin),
        maximumLeft,
      ),
      top: Math.min(Math.max(top, margin), maximumTop),
      ready: true,
    });
  }, []);

  useLayoutEffect(() => {
    if (open) updatePosition();
  }, [label, open, updatePosition]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const closeOtherTooltip = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== id) hide();
    };
    window.addEventListener(tooltipOpenEvent, closeOtherTooltip);
    return () => window.removeEventListener(tooltipOpenEvent, closeOtherTooltip);
  }, [hide, id]);

  useEffect(() => {
    if (!open || typeof window === "undefined") return;
    window.addEventListener("resize", hide);
    document.addEventListener("scroll", hide, true);
    return () => {
      window.removeEventListener("resize", hide);
      document.removeEventListener("scroll", hide, true);
    };
  }, [hide, open]);

  return (
    <span
      ref={anchor}
      className={className}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocusCapture={show}
      onBlurCapture={hide}
      onClickCapture={hide}
      onKeyDownCapture={(event) => {
        if (event.key === "Escape") hide();
      }}
    >
      {children}
      {open &&
        label &&
        typeof document !== "undefined" &&
        createPortal(
          <span
            ref={tooltip}
            role="tooltip"
            style={{
              left: position.left,
              top: position.top,
              visibility: position.ready ? "visible" : "hidden",
            }}
            className="pointer-events-none fixed z-[200] max-w-[calc(100vw-16px)] whitespace-normal break-words rounded-md border border-zinc-700 bg-zinc-900 px-2 py-1 text-center text-[11px] leading-4 text-zinc-200 shadow-lg shadow-black/30 [overflow-wrap:anywhere]"
          >
            {label}
          </span>,
          document.body,
        )}
    </span>
  );
}

export function MetricCard({
  label,
  value,
  hint,
  icon,
  loading = false,
}: {
  label: string;
  value: string;
  hint: string;
  icon: ReactNode;
  loading?: boolean;
}) {
  return (
    <Panel className="min-h-[108px] p-4">
      <div className="flex items-start justify-between gap-3">
        <p className="text-xs font-medium text-zinc-500">{label}</p>
        <span className="text-zinc-600">{icon}</span>
      </div>
      {loading ? (
        <>
          <Skeleton className="mt-3 h-7 w-20" />
          <Skeleton className="mt-2 h-3 w-28" />
        </>
      ) : (
        <>
          <p className="mt-2 min-h-7 font-mono text-2xl font-semibold tabular-nums text-zinc-100">
            {value}
          </p>
          <p className="mt-1 truncate text-xs text-zinc-600">{hint}</p>
        </>
      )}
    </Panel>
  );
}

export function Pagination({
  page,
  totalPages,
  total,
  pageSize,
  onChange,
  onPageSizeChange,
  busy = false,
}: {
  page: number;
  totalPages: number;
  total?: number;
  pageSize?: number;
  onChange: (page: number) => void;
  onPageSizeChange?: (size: number) => void;
  busy?: boolean;
}) {
  return (
    <nav
      aria-label="Paginação"
      className="flex min-h-14 flex-wrap items-center justify-between gap-3 border-t border-zinc-800 px-3 py-2 text-xs text-zinc-500"
    >
      <span className="min-w-32">
        {typeof total === "number" ? `${total.toLocaleString("pt-BR")} registros` : ""}
        {busy && <span className="ml-2 text-zinc-400">Atualizando...</span>}
      </span>
      <div className="flex items-center gap-2">
        {pageSize && onPageSizeChange && (
          <Listbox
            value={pageSize}
            onChange={onPageSizeChange}
            label="Registros por página"
            size="compact"
            className="w-[112px]"
            options={[25, 50, 100, 250].map((size) => ({
              value: size,
              label: `${size} / pág.`,
            }))}
          />
        )}
        <button
          type="button"
          aria-label="Página anterior"
          title="Página anterior"
          disabled={page <= 1 || busy}
          onClick={() => onChange(page - 1)}
          className="grid size-8 shrink-0 place-items-center rounded-lg border border-zinc-700 bg-zinc-950 text-zinc-400 transition-colors hover:border-zinc-600 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:opacity-40"
        >
          <ChevronLeft className="size-4" />
        </button>
        <span className="min-w-24 text-center tabular-nums">
          Página {page} de {Math.max(1, totalPages)}
        </span>
        <button
          type="button"
          aria-label="Próxima página"
          title="Próxima página"
          disabled={page >= totalPages || busy}
          onClick={() => onChange(page + 1)}
          className="grid size-8 shrink-0 place-items-center rounded-lg border border-zinc-700 bg-zinc-950 text-zinc-400 transition-colors hover:border-zinc-600 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 disabled:cursor-not-allowed disabled:opacity-40"
        >
          <ChevronRight className="size-4" />
        </button>
      </div>
    </nav>
  );
}

export function Loading() {
  return (
    <div role="status" className="space-y-3 p-4" aria-label="Carregando">
      {Array.from({ length: 5 }, (_, index) => (
        <Skeleton key={index} className="h-12 w-full" />
      ))}
    </div>
  );
}
