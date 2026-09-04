import {
  Activity,
  Bell,
  ChevronLeft,
  ChevronRight,
  LayoutDashboard,
  LogOut,
  Menu,
  RefreshCw,
  Server,
  X,
  Zap,
  ScanSearch,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  NavLink,
  Outlet,
  useLocation,
} from "react-router-dom";
import { api } from "../api";
import { alertsApi } from "../api/alerts";
import { eventsApi } from "../api/events";
import { monitoringApi } from "../api/monitoring";
import { queryClient } from "../api/queryClient";
import { useAuth } from "../auth/AuthProvider";
import { SettingsButton, SettingsDialog } from "../components/SettingsDialog";
import { AppFooter } from "../components/AppFooter";
import type { SettingsCategory } from "../components/SettingsDialog";
import type { StreamState } from "../hooks/useLogStream";
import { IconButton, StatusIndicator, Tooltip } from "../components/ui";
import { ShellContext } from "./appShellContext";

const navigation = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/senders", label: "Senders", icon: Server, end: false },
  { to: "/alerts", label: "Alerts", icon: Bell, end: false },
  { to: "/events", label: "Events", icon: Zap, end: false },
  { to: "/monitoring", label: "Monitoring", icon: ScanSearch, end: false },
  { to: "/status", label: "System status", icon: Activity, end: false },
];

function prefetchNavigation(to: string) {
  if (to === "/" || to === "/senders") {
    void queryClient.prefetchQuery({
      queryKey: ["view", "dashboard", "summary"],
      queryFn: api.summary,
    });
    void queryClient.prefetchQuery({
      queryKey: ["view", "dashboard", "senders"],
      queryFn: () => api.senders("page=1&page_size=25&group_by=name&sort=last_activity_at&order=desc"),
    });
  } else if (to === "/alerts") {
    void queryClient.prefetchQuery({
      queryKey: ["view", "alerts"],
      queryFn: () => alertsApi.list("page=1&page_size=20"),
    });
  } else if (to === "/events") {
    void queryClient.prefetchQuery({
      queryKey: ["view", "events"],
      queryFn: () => eventsApi.list("page=1&page_size=20"),
    });
  } else if (to === "/monitoring") {
    void queryClient.prefetchQuery({
      queryKey: ["view", "monitoring"],
      queryFn: () => monitoringApi.list("page=1&page_size=20"),
    });
  } else if (to === "/status") {
    void queryClient.prefetchQuery({
      queryKey: ["view", "system-status"],
      queryFn: api.health,
    });
  }
}

function SidebarContent({
  collapsed,
  onCollapse,
  onNavigate,
  onOpenSettings,
  onLogout,
  authRequired,
}: {
  collapsed: boolean;
  onCollapse: () => void;
  onNavigate?: () => void;
  onOpenSettings: (trigger: HTMLButtonElement) => void;
  onLogout: () => void;
  authRequired: boolean;
}) {
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b border-zinc-800 px-2">
        <span className="grid h-11 w-12 shrink-0 place-items-center overflow-hidden">
          <img
            src="/logmate.png"
            alt="LogMate"
            className="size-full object-contain"
          />
        </span>
        {!collapsed && (
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-zinc-100">LogMate</p>
            <p className="truncate text-[10px] uppercase tracking-wider text-zinc-600">
              Observability
            </p>
          </div>
        )}
      </div>
      <nav aria-label="Main navigation" className="flex flex-1 flex-col gap-1 p-2">
        {navigation.map(({ to, label, icon: Icon, end }) => (
          <Tooltip key={to} label={collapsed ? label : ""} className="block w-full">
            <NavLink
              to={to}
              end={end}
              onClick={onNavigate}
              onFocus={() => prefetchNavigation(to)}
              onMouseEnter={() => prefetchNavigation(to)}
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
      <div className="flex flex-col gap-2 p-2">
        <div>
          <SettingsButton collapsed={collapsed} onOpen={onOpenSettings} />
        </div>
        <IconButton
          label="Sign out"
          tooltipClassName="block w-full"
          onClick={onLogout}
          className={`${authRequired ? "" : "hidden"} w-full border-transparent ${collapsed ? "justify-center" : "justify-start px-5"}`}
        >
          {collapsed ? (
            <LogOut className="size-4" />
          ) : (
            <span className="flex items-center gap-3">
              <LogOut className="size-4" />
              <span className="text-xs">Sign out</span>
            </span>
          )}
        </IconButton>
        <IconButton
          label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          tooltipClassName="block w-full"
          onClick={onCollapse}
          className={`mt-2 hidden w-full border-transparent lg:inline-flex ${
            collapsed ? "justify-center" : "justify-start px-5"
          }`}
        >
          {collapsed ? (
            <ChevronRight className="size-4" />
          ) : (
            <span className="flex items-center gap-3">
              <ChevronLeft className="size-4" />
              <span className="text-xs">Collapse</span>
            </span>
          )}
        </IconButton>
      </div>
    </div>
  );
}

function pageInformation(pathname: string, sender?: string) {
  if (pathname === "/status") return { title: "System status", breadcrumb: "System" };
  if (pathname === "/alerts") return { title: "Email alerts", breadcrumb: "Notifications" };
  if (pathname === "/events") return { title: "Events", breadcrumb: "Automations" };
  if (pathname.startsWith("/monitoring")) return { title: pathname === "/monitoring" ? "Monitoring" : "Rule builder", breadcrumb: "Automations" };
  if (pathname.startsWith("/senders/"))
    return { title: "Sender details", breadcrumb: sender ?? "Sender" };
  if (pathname === "/senders") return { title: "Senders", breadcrumb: "Inventory" };
  return { title: "Dashboard", breadcrumb: "Overview" };
}

export function AppShell() {
  const location = useLocation();
  const { logout, authRequired } = useAuth();
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem("sidebar-collapsed") === "true",
  );
  const [mobileOpen, setMobileOpen] = useState(false);
  const [refreshToken, setRefreshToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const [streamState, setStreamState] = useState<StreamState | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTrigger, setSettingsTrigger] = useState<HTMLButtonElement | null>(null);
  const [settingsCategory, setSettingsCategory] = useState<SettingsCategory>("storage");

  useEffect(() => setMobileOpen(false), [location.pathname]);

  const toggleCollapsed = () => {
    setCollapsed((current) => {
      localStorage.setItem("sidebar-collapsed", String(!current));
      return !current;
    });
  };

  const refresh = () => {
    setRefreshToken((value) => value + 1);
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
      <div className="flex h-[100dvh] flex-col overflow-hidden bg-[#0c0c0f] text-zinc-100">
        <aside
          className={`fixed inset-y-0 bottom-16 left-0 z-40 hidden border-r border-zinc-800 bg-[#111113] transition-[width] duration-150 ease-out lg:block ${
            collapsed ? "w-16" : "w-60"
          }`}
        >
          <SidebarContent
            collapsed={collapsed}
            onCollapse={toggleCollapsed}
            onOpenSettings={openSettings}
            authRequired={authRequired}
            onLogout={() => {
              if (!authRequired) return;
              void logout();
            }}
          />
        </aside>

        {mobileOpen && (
          <div className="fixed inset-x-0 bottom-24 top-0 z-50 sm:bottom-16 lg:hidden">
            <button
              className="absolute inset-0 bg-black/70"
              aria-label="Close menu"
              onClick={() => setMobileOpen(false)}
            />
            <aside className="relative h-full w-72 border-r border-zinc-800 bg-[#111113]">
              <IconButton
                label="Close menu"
                onClick={() => setMobileOpen(false)}
                className="absolute right-2 top-2 z-10"
              >
                <X className="size-5" />
              </IconButton>
              <SidebarContent
                collapsed={false}
                onCollapse={() => setMobileOpen(false)}
                onNavigate={() => setMobileOpen(false)}
                onOpenSettings={openSettings}
                authRequired={authRequired}
                onLogout={() => {
                  if (!authRequired) return;
                  void logout();
                }}
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
          className={`flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden transition-[padding] duration-150 ease-out ${
            collapsed ? "lg:pl-16" : "lg:pl-60"
          }`}
        >
          <header className="z-30 flex h-14 shrink-0 items-center justify-between border-b border-zinc-800 bg-[#111113]/95 px-3 backdrop-blur-sm sm:px-5">
            <div className="flex min-w-0 items-center gap-3">
              <IconButton
                label="Open menu"
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
                        connected: "Live SSE",
                        reconnecting: "Reconnecting",
                        paused: "SSE paused",
                        disconnected: "SSE offline",
                        error: "SSE error",
                      }[streamState]
                    }
                  />
                )}
              </div>
              <IconButton
                label="Refresh page"
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
        <AppFooter />
      </div>
    </ShellContext.Provider>
  );
}
