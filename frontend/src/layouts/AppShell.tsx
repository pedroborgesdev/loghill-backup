import {
  Activity,
  Bell,
  ChevronRight,
  LayoutDashboard,
  Menu,
  PanelLeftClose,
  RefreshCw,
  Server,
  X,
  Zap,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  NavLink,
  Outlet,
  useLocation,
} from "react-router-dom";
import { api } from "../api";
import { SettingsButton, SettingsDialog } from "../components/SettingsDialog";
import type { SettingsCategory } from "../components/SettingsDialog";
import type { StreamState } from "../hooks/useLogStream";
import { IconButton, StatusIndicator, Tooltip } from "../components/ui";
import { ShellContext } from "./appShellContext";

const navigation = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/senders", label: "Senders", icon: Server, end: false },
  { to: "/alerts", label: "Alertas", icon: Bell, end: false },
  { to: "/events", label: "Eventos", icon: Zap, end: false },
  { to: "/status", label: "Status do sistema", icon: Activity, end: false },
];

function SidebarContent({
  collapsed,
  onCollapse,
  onNavigate,
  backendOnline,
  onOpenSettings,
}: {
  collapsed: boolean;
  onCollapse: () => void;
  onNavigate?: () => void;
  backendOnline: boolean | null;
  onOpenSettings: (trigger: HTMLButtonElement) => void;
}) {
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b border-zinc-800 px-2">
        <span className="grid h-11 w-12 shrink-0 place-items-center overflow-hidden">
          <img
            src="/loghill.png"
            alt="LogHill"
            className="size-full object-contain"
          />
        </span>
        {!collapsed && (
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-zinc-100">LogHill</p>
            <p className="truncate text-[10px] uppercase tracking-wider text-zinc-600">
              Observabilidade
            </p>
          </div>
        )}
      </div>
      <nav aria-label="Navegação principal" className="flex flex-1 flex-col gap-1 p-2">
        {navigation.map(({ to, label, icon: Icon, end }) => (
          <Tooltip key={to} label={collapsed ? label : ""} className="block w-full">
            <NavLink
              to={to}
              end={end}
              onClick={onNavigate}
              className={({ isActive }) =>
                `flex h-10 w-full items-center gap-3 rounded-lg px-3 text-sm transition-colors duration-150 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 ${
                  collapsed ? "justify-center" : ""
                } ${
                  isActive
                    ? "bg-zinc-800 text-zinc-100"
                    : "text-zinc-500 hover:bg-zinc-900 hover:text-zinc-200"
                }`
              }
            >
              <Icon className="size-5 shrink-0" aria-hidden="true" />
              {!collapsed && <span className="truncate">{label}</span>}
            </NavLink>
          </Tooltip>
        ))}
      </nav>
      <div className="flex flex-col gap-2 border-t border-zinc-800 p-2">
        <div className="border-b border-zinc-800 pb-2">
          <SettingsButton collapsed={collapsed} onOpen={onOpenSettings} />
        </div>
        <div
          className={`flex h-10 w-full items-center gap-3 rounded-lg px-3 ${
            collapsed ? "justify-center" : ""
          }`}
        >
          <span
            className={`size-2 shrink-0 rounded-full ${
              backendOnline === null
                ? "bg-zinc-600"
                : backendOnline
                  ? "bg-emerald-400"
                  : "bg-red-400"
            }`}
          />
          {!collapsed && (
            <div className="min-w-0">
              <p className="text-xs text-zinc-400">Backend</p>
              <p className="text-[10px] text-zinc-600">
                {backendOnline === null
                  ? "Verificando"
                  : backendOnline
                    ? "Operacional"
                    : "Indisponível"}
              </p>
            </div>
          )}
        </div>
        <IconButton
          label={collapsed ? "Expandir sidebar" : "Recolher sidebar"}
          tooltipClassName="block w-full"
          onClick={onCollapse}
          className={`hidden w-full border-transparent lg:inline-flex ${
            collapsed ? "justify-center" : "justify-start"
          }`}
        >
          {collapsed ? (
            <ChevronRight className="size-4" />
          ) : (
            <>
              <PanelLeftClose className="size-4" />
              <span className="text-xs">Recolher</span>
            </>
          )}
        </IconButton>
      </div>
    </div>
  );
}

function pageInformation(pathname: string, sender?: string) {
  if (pathname === "/status") return { title: "Status do sistema", breadcrumb: "Sistema" };
  if (pathname === "/alerts") return { title: "Alertas de e-mail", breadcrumb: "Notificações" };
  if (pathname === "/events") return { title: "Eventos", breadcrumb: "Automações" };
  if (pathname.startsWith("/senders/"))
    return { title: "Detalhes do sender", breadcrumb: sender ?? "Sender" };
  if (pathname === "/senders") return { title: "Senders", breadcrumb: "Inventário" };
  return { title: "Dashboard", breadcrumb: "Visão geral" };
}

export function AppShell() {
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem("sidebar-collapsed") === "true",
  );
  const [mobileOpen, setMobileOpen] = useState(false);
  const [backendOnline, setBackendOnline] = useState<boolean | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const [streamState, setStreamState] = useState<StreamState | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTrigger, setSettingsTrigger] = useState<HTMLButtonElement | null>(null);
  const [settingsCategory, setSettingsCategory] = useState<SettingsCategory>("storage");

  const checkBackend = useCallback(async () => {
    try {
      await api.health();
      setBackendOnline(true);
    } catch {
      setBackendOnline(false);
    }
  }, []);

  useEffect(() => {
    void checkBackend();
    const timer = window.setInterval(() => void checkBackend(), 30_000);
    return () => window.clearInterval(timer);
  }, [checkBackend]);

  useEffect(() => setMobileOpen(false), [location.pathname]);

  const toggleCollapsed = () => {
    setCollapsed((current) => {
      localStorage.setItem("sidebar-collapsed", String(!current));
      return !current;
    });
  };

  const refresh = () => {
    setRefreshToken((value) => value + 1);
    void checkBackend();
  };

  const openSettings = (trigger: HTMLButtonElement) => {
    setSettingsTrigger(trigger);
    setSettingsCategory("storage");
    setSettingsOpen(true);
  };

  const openEmailSettings = useCallback((trigger?: HTMLButtonElement) => {
    setSettingsTrigger(trigger ?? null);
    setSettingsCategory("email");
    setSettingsOpen(true);
  }, []);

  const context = useMemo(
    () => ({
      refreshToken,
      refreshing,
      setRefreshing,
      streamState,
      setStreamState,
      openEmailSettings,
    }),
    [openEmailSettings, refreshToken, refreshing, streamState],
  );
  const senderFromPath = location.pathname.startsWith("/senders/")
    ? decodeURIComponent(location.pathname.slice("/senders/".length))
    : undefined;
  const page = pageInformation(location.pathname, senderFromPath);

  return (
    <ShellContext.Provider value={context}>
      <div className="h-[100dvh] overflow-hidden bg-[#09090b] text-zinc-100">
        <aside
          className={`fixed inset-y-0 left-0 z-40 hidden border-r border-zinc-800 bg-[#111113] transition-[width] duration-150 ease-out lg:block ${
            collapsed ? "w-16" : "w-60"
          }`}
        >
          <SidebarContent
            collapsed={collapsed}
            onCollapse={toggleCollapsed}
            backendOnline={backendOnline}
            onOpenSettings={openSettings}
          />
        </aside>

        {mobileOpen && (
          <div className="fixed inset-0 z-50 lg:hidden">
            <button
              className="absolute inset-0 bg-black/70"
              aria-label="Fechar menu"
              onClick={() => setMobileOpen(false)}
            />
            <aside className="relative h-full w-72 border-r border-zinc-800 bg-[#111113]">
              <IconButton
                label="Fechar menu"
                onClick={() => setMobileOpen(false)}
                className="absolute right-2 top-2 z-10"
              >
                <X className="size-5" />
              </IconButton>
              <SidebarContent
                collapsed={false}
                onCollapse={() => setMobileOpen(false)}
                onNavigate={() => setMobileOpen(false)}
                backendOnline={backendOnline}
                onOpenSettings={openSettings}
              />
            </aside>
          </div>
        )}

        {settingsOpen && (
          <SettingsDialog
            returnFocus={settingsTrigger}
            initialCategory={settingsCategory}
            onClose={() => setSettingsOpen(false)}
          />
        )}

        <div
          className={`flex h-[100dvh] min-w-0 flex-col overflow-hidden transition-[padding] duration-150 ease-out ${
            collapsed ? "lg:pl-16" : "lg:pl-60"
          }`}
        >
          <header className="z-30 flex h-14 shrink-0 items-center justify-between border-b border-zinc-800 bg-[#111113]/95 px-3 backdrop-blur-sm sm:px-5">
            <div className="flex min-w-0 items-center gap-3">
              <IconButton
                label="Abrir menu"
                onClick={() => setMobileOpen(true)}
                className="lg:hidden"
              >
                <Menu className="size-5" />
              </IconButton>
              <div className="min-w-0">
                <h1 className="truncate text-sm font-semibold text-zinc-100">{page.title}</h1>
                <p className="truncate text-[11px] text-zinc-600">{page.breadcrumb}</p>
              </div>
            </div>
            <div className="flex items-center gap-3">
              <div className="hidden items-center gap-4 sm:flex">
                <StatusIndicator
                  status={
                    backendOnline === null
                      ? "neutral"
                      : backendOnline
                        ? "online"
                        : "offline"
                  }
                  label={
                    backendOnline === null
                      ? "Verificando"
                      : backendOnline
                        ? "Backend online"
                        : "Backend offline"
                  }
                />
                {streamState && (
                  <StatusIndicator
                    status={
                      streamState === "connected"
                        ? "online"
                        : streamState === "reconnecting"
                          ? "warning"
                          : streamState === "error"
                            ? "offline"
                            : "neutral"
                    }
                    label={
                      {
                        connected: "SSE ao vivo",
                        reconnecting: "Reconectando",
                        paused: "SSE pausado",
                        disconnected: "SSE offline",
                        error: "Erro no SSE",
                      }[streamState]
                    }
                  />
                )}
              </div>
              <IconButton
                label="Atualizar página"
                onClick={refresh}
                disabled={refreshing}
              >
                <RefreshCw
                  className={`size-4 ${refreshing ? "animate-spin" : ""}`}
                />
              </IconButton>
            </div>
          </header>
          <main className="min-h-0 flex-1 overflow-auto p-3 sm:p-4 xl:p-5">
            <Outlet />
          </main>
        </div>
      </div>
    </ShellContext.Provider>
  );
}
