import {
  ArrowLeft,
  ChevronRight,
  Copy,
  Edit3,
  ExternalLink,
  KeyRound,
  Layers3,
  MoreHorizontal,
  ShieldOff,
  Trash2,
} from "lucide-react";
import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { createPortal } from "react-dom";
import { Link, useNavigate } from "react-router-dom";
import type { Sender } from "../types/api";
import type { SenderAction } from "./senders/SenderDialogs";
import {
  formatDate,
  formatNumber,
  relativeDate,
} from "../utils/format";
import {
  groupSenders,
  type SenderGroup,
} from "../utils/senders";
import { EmptyState, IconButton, Skeleton, StatusBadge } from "./ui";

function activateRow(
  event: KeyboardEvent<HTMLElement>,
  action: () => void,
) {
  if (event.target !== event.currentTarget) return;
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    action();
  }
}

function Actions({ sender, onAction }: { sender: Sender; onAction?: (sender: Sender, action: SenderAction) => void }) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({ left: 8, top: 8 });

  useLayoutEffect(() => {
    if (!open || !container.current) return;
    const bounds = container.current.getBoundingClientRect();
    const width = 208;
    const height = sender.status === "revoked" ? 190 : 250;
    setPosition({
      left: Math.max(8, Math.min(bounds.right - width, window.innerWidth - width - 8)),
      top: bounds.bottom + height <= window.innerHeight - 8 ? bounds.bottom + 4 : Math.max(8, bounds.top - height - 4),
    });
  }, [open, sender.status]);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => {
      const target = event.target as Node;
      if (!container.current?.contains(target) && !menu.current?.contains(target)) setOpen(false);
    };
    const closeOnScroll = () => setOpen(false);
    document.addEventListener("mousedown", close);
    document.addEventListener("scroll", closeOnScroll, true);
    return () => { document.removeEventListener("mousedown", close); document.removeEventListener("scroll", closeOnScroll, true); };
  }, [open]);

  return (
    <div
      ref={container}
      className="relative"
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    >
      <IconButton
        label={`Ações de ${sender.id}`}
        className="size-8"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <MoreHorizontal className="size-4" />
      </IconButton>
      {open && createPortal(
        <div
          ref={menu}
          role="menu"
          className="fixed z-[195] w-52 rounded-lg border border-zinc-700 bg-zinc-900 p-1 text-xs shadow-xl shadow-black/40"
          style={position}
        >
          <Link
            role="menuitem"
            to={`/senders/${sender.id}`}
            className="flex items-center gap-2 rounded-md px-3 py-2 text-zinc-300 hover:bg-zinc-800"
          >
            <ExternalLink className="size-3.5" />
            Abrir logs
          </Link>
          <button type="button" role="menuitem" onClick={() => { void navigator.clipboard.writeText(sender.id); setOpen(false); }} className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-zinc-300 hover:bg-zinc-800"><Copy className="size-3.5" />Copiar ID</button>
          <button type="button" role="menuitem" onClick={() => { setOpen(false); onAction?.(sender, "edit"); }} className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-zinc-300 hover:bg-zinc-800"><Edit3 className="size-3.5" />Editar informações</button>
          {sender.status === "revoked" ? (
            <button type="button" role="menuitem" onClick={() => { setOpen(false); onAction?.(sender, "reactivate"); }} className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-zinc-300 hover:bg-zinc-800"><KeyRound className="size-3.5" />Reativar acesso</button>
          ) : sender.status !== "expired" ? (
            <>
              <button type="button" role="menuitem" onClick={() => { setOpen(false); onAction?.(sender, "rotate"); }} className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-zinc-300 hover:bg-zinc-800"><KeyRound className="size-3.5" />Gerar nova chave</button>
              <button type="button" role="menuitem" onClick={() => { setOpen(false); onAction?.(sender, "revoke"); }} className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-amber-300 hover:bg-zinc-800"><ShieldOff className="size-3.5" />Revogar acesso</button>
            </>
          ) : null}
          <button type="button" role="menuitem" onClick={() => { setOpen(false); onAction?.(sender, "delete"); }} className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-red-400 hover:bg-zinc-800"><Trash2 className="size-3.5" />Excluir sender</button>
        </div>
      , document.body)}
    </div>
  );
}

export function SenderTableSkeleton() {
  return (
    <div className="min-h-[380px] divide-y divide-zinc-800">
      {Array.from({ length: 7 }, (_, index) => (
        <div
          key={index}
          className="grid h-[52px] grid-cols-[92px_minmax(180px,1.5fr)_130px_90px_90px_90px_40px] items-center gap-3 px-3"
        >
          <Skeleton className="h-5 w-16" />
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-4 w-12" />
          <Skeleton className="h-4 w-14" />
          <Skeleton className="h-4 w-10" />
          <Skeleton className="size-7" />
        </div>
      ))}
    </div>
  );
}

function InstanceChooser({
  group,
  onBack,
  onAction,
}: {
  group: SenderGroup;
  onBack: () => void;
  onAction?: (sender: Sender, action: SenderAction) => void;
}) {
  const navigate = useNavigate();

  return (
    <>
      <div className="flex min-h-12 items-center gap-2 border-b border-zinc-800 bg-[#18181b] px-3">
        <IconButton label="Voltar aos grupos" className="size-8" onClick={onBack}>
          <ArrowLeft className="size-4" />
        </IconButton>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-zinc-200">
            {group.name}
          </p>
          <p className="text-[11px] text-zinc-600">
            Escolha uma das {group.items.length} instâncias
          </p>
        </div>
      </div>

      <div className="hidden min-h-[332px] overflow-auto md:block">
        <table className="w-full table-fixed text-left text-xs">
          <thead className="sticky top-0 z-10 bg-[#1c1c1f] text-zinc-500">
            <tr className="h-10 border-b border-zinc-800">
              <th className="w-[150px] px-3 font-medium">Status</th>
              <th className="px-3 font-medium">Instância</th>
              <th className="w-[150px] px-3 font-medium">Última atividade</th>
              <th className="w-[100px] px-3 text-right font-medium">Logs</th>
              <th className="w-[100px] px-3 text-right font-medium">Erros recentes</th>
              <th className="w-12 px-2">
                <span className="sr-only">Ações</span>
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800/80">
            {group.items.map((sender) => (
              <tr
                key={sender.id}
                role="link"
                tabIndex={0}
                aria-label={`Abrir instância ${sender.id}`}
                onClick={() => navigate(`/senders/${sender.id}`)}
                onKeyDown={(event) =>
                  activateRow(event, () => navigate(`/senders/${sender.id}`))
                }
                className="h-[52px] cursor-pointer hover:bg-zinc-900/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50"
              >
                <td className="px-3">
                  <StatusBadge status={sender.status} />
                </td>
                <td className="min-w-0 px-3">
                  <span
                    className="block truncate font-mono font-medium text-zinc-200 hover:text-white"
                    title={sender.id}
                  >
                    {sender.id}
                  </span>
                  <span className="block truncate text-[10px] text-zinc-600">
                    Criado em {formatDate(sender.created_at)}
                  </span>
                </td>
                <td
                  className="px-3 text-zinc-400"
                  title={formatDate(sender.last_activity_at)}
                >
                  {relativeDate(sender.last_activity_at)}
                </td>
                <td className="px-3 text-right font-mono tabular-nums text-zinc-300">
                  {formatNumber(sender.log_line_count)}
                </td>
                <td className="px-3 text-right font-mono tabular-nums text-zinc-500">
                  {formatNumber(sender.recent_error_count ?? 0)}
                </td>
                <td className="px-2">
                  <Actions sender={sender} onAction={onAction} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="divide-y divide-zinc-800 md:hidden">
        {group.items.map((sender) => (
          <article
            key={sender.id}
            role="link"
            tabIndex={0}
            aria-label={`Abrir instância ${sender.id}`}
            onClick={() => navigate(`/senders/${sender.id}`)}
            onKeyDown={(event) =>
              activateRow(event, () => navigate(`/senders/${sender.id}`))
            }
            className="cursor-pointer p-3 hover:bg-zinc-900/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <span
                  className="block truncate font-mono text-xs font-medium text-zinc-200"
                >
                  {sender.id}
                </span>
                <span className="text-[10px] text-zinc-600">
                  {relativeDate(sender.last_activity_at)}
                </span>
              </div>
              <StatusBadge status={sender.status} />
            </div>
            <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
              <div>
                <p className="text-zinc-600">Logs</p>
                <p className="mt-0.5 font-mono">
                  {formatNumber(sender.log_line_count)}
                </p>
              </div>
              <div>
                <p className="text-zinc-600">Erros recentes</p>
                <p className="mt-0.5 font-mono">
                  {formatNumber(sender.recent_error_count ?? 0)}
                </p>
              </div>
            </div>
          </article>
        ))}
      </div>
    </>
  );
}

export function SenderTable({ items, onAction }: { items: Sender[]; onAction?: (sender: Sender, action: SenderAction) => void }) {
  const navigate = useNavigate();
  const groups = useMemo(() => groupSenders(items), [items]);
  const [selectedGroupKey, setSelectedGroupKey] = useState<string>();
  const selectedGroup = groups.find((group) => group.key === selectedGroupKey);

  useEffect(() => {
    if (selectedGroupKey && !selectedGroup) setSelectedGroupKey(undefined);
  }, [selectedGroup, selectedGroupKey]);

  if (!items.length) {
    return (
      <EmptyState
        title="Nenhum sender encontrado"
        description="Tente alterar os filtros ou a busca."
      />
    );
  }

  if (selectedGroup && selectedGroup.items.length > 1) {
    return (
      <InstanceChooser
        group={selectedGroup}
        onBack={() => setSelectedGroupKey(undefined)}
        onAction={onAction}
      />
    );
  }

  return (
    <>
      <div className="hidden min-h-[380px] overflow-auto md:block">
        <table className="w-full table-fixed text-left text-xs">
          <thead className="sticky top-0 z-10 bg-[#1c1c1f] text-zinc-500">
            <tr className="h-10 border-b border-zinc-800">
              <th className="w-[150px] px-3 font-medium">Status</th>
              <th className="px-3 font-medium">Sender</th>
              <th className="w-[150px] px-3 font-medium">Última atividade</th>
              <th className="w-[100px] px-3 text-right font-medium">Logs</th>
              <th className="w-[100px] px-3 text-right font-medium">Erros recentes</th>
              <th className="w-[130px] px-3 text-right font-medium">Instâncias</th>
              <th className="w-12 px-2">
                <span className="sr-only">Ações</span>
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800/80">
            {groups.map((group) => {
              const multiple = group.items.length > 1;
              const instanceCount = group.items.reduce((total, item) => total + (item.instance_count ?? 0), 0);
              const sender = group.items[0];
              const openGroup = () => {
                if (multiple) setSelectedGroupKey(group.key);
                else navigate(`/senders/${sender.id}`);
              };
              return (
                <tr
                  key={group.key}
                  role={multiple ? "button" : "link"}
                  tabIndex={0}
                  aria-label={
                    multiple
                      ? `Abrir grupo ${group.name} com ${group.items.length} instâncias`
                      : `Abrir sender ${sender.name}`
                  }
                  onClick={openGroup}
                  onKeyDown={(event) => activateRow(event, openGroup)}
                  className="h-[52px] cursor-pointer hover:bg-zinc-900/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50"
                >
                  <td className="px-3">
                    <StatusBadge status={group.status} />
                  </td>
                  <td className="min-w-0 px-3">
                    {multiple ? (
                      <div className="block w-full min-w-0 text-left">
                        <span className="block truncate font-medium text-zinc-200 hover:text-white">
                          {group.name}
                        </span>
                        <span className="inline-flex items-center gap-1 text-[10px] text-cyan-600">
                          <Layers3 className="size-3" />
                          {group.items.length} instâncias
                        </span>
                      </div>
                    ) : (
                      <>
                        <span
                          className="block truncate font-medium text-zinc-200 hover:text-white"
                          title={sender.name}
                        >
                          {sender.name}
                        </span>
                        <code
                          className="block truncate text-[10px] text-zinc-600"
                          title={sender.id}
                        >
                          {sender.id}
                        </code>
                      </>
                    )}
                  </td>
                  <td
                    className="px-3 text-zinc-400"
                    title={formatDate(group.lastActivityAt)}
                  >
                    {relativeDate(group.lastActivityAt)}
                  </td>
                  <td className="px-3 text-right font-mono tabular-nums text-zinc-300">
                    {formatNumber(group.logLineCount)}
                  </td>
                  <td className="px-3 text-right font-mono tabular-nums text-zinc-500">
                    {formatNumber(group.recentErrorCount)}
                  </td>
                  <td className="px-3 text-right">
                    {instanceCount > 1 ? (
                      <span className="ml-auto inline-flex items-center gap-1.5 rounded-full border border-sky-900/80 bg-sky-950/25 px-2 py-1 text-[10px] font-medium text-sky-400" title={`${instanceCount} instâncias conectadas a este sender`}>
                        <Layers3 className="size-3" />
                        {formatNumber(instanceCount)} instâncias
                      </span>
                    ) : (
                      <span className="font-mono tabular-nums text-zinc-500">{formatNumber(instanceCount)}</span>
                    )}
                  </td>
                  <td className="px-2">
                    {multiple ? (
                      <span
                        aria-hidden="true"
                        className="grid size-8 place-items-center text-zinc-500"
                      >
                        <ChevronRight className="size-4" />
                      </span>
                    ) : (
                      <Actions sender={sender} onAction={onAction} />
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="divide-y divide-zinc-800 md:hidden">
        {groups.map((group) => {
          const multiple = group.items.length > 1;
          const instanceCount = group.items.reduce((total, item) => total + (item.instance_count ?? 0), 0);
          const sender = group.items[0];
          const openGroup = () => {
            if (multiple) setSelectedGroupKey(group.key);
            else navigate(`/senders/${sender.id}`);
          };
          return (
            <article
              key={group.key}
              role={multiple ? "button" : "link"}
              tabIndex={0}
              aria-label={
                multiple
                  ? `Abrir grupo ${group.name} com ${group.items.length} instâncias`
                  : `Abrir sender ${sender.name}`
              }
              onClick={openGroup}
              onKeyDown={(event) => activateRow(event, openGroup)}
              className="cursor-pointer p-3 hover:bg-zinc-900/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-white/50"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  {multiple ? (
                    <div className="block min-w-0 text-left">
                      <span className="block truncate text-sm font-medium text-zinc-200">
                        {group.name}
                      </span>
                      <span className="inline-flex items-center gap-1 text-[10px] text-cyan-600">
                          <Layers3 className="size-3" />
                          {group.items.length} instâncias
                        </span>
                    </div>
                  ) : (
                    <>
                      <span
                        className="block truncate text-sm font-medium text-zinc-200"
                      >
                        {sender.name}
                      </span>
                      <code className="block truncate text-[10px] text-zinc-600">
                        {sender.id}
                      </code>
                    </>
                  )}
                </div>
                <StatusBadge status={group.status} />
              </div>
              <div className="mt-3 grid grid-cols-4 gap-2 text-xs">
                <div>
                  <p className="text-zinc-600">Atividade</p>
                  <p className="mt-0.5 text-zinc-400">
                    {relativeDate(group.lastActivityAt)}
                  </p>
                </div>
                <div>
                  <p className="text-zinc-600">Logs</p>
                  <p className="mt-0.5 font-mono">
                    {formatNumber(group.logLineCount)}
                  </p>
                </div>
                <div>
                  <p className="text-zinc-600">Erros recentes</p>
                  <p className="mt-0.5 font-mono">
                    {formatNumber(group.recentErrorCount)}
                  </p>
                </div>
                <div><p className="text-zinc-600">Instâncias</p>{instanceCount > 1 ? <p className="mt-0.5 inline-flex items-center gap-1 text-sky-400"><Layers3 className="size-3" />{formatNumber(instanceCount)}</p> : <p className="mt-0.5 font-mono">{formatNumber(instanceCount)}</p>}</div>
              </div>
            </article>
          );
        })}
      </div>
    </>
  );
}
