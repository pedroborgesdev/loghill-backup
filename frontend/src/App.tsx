import { BrowserRouter, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { AuthProvider, useAuth } from "./auth/AuthProvider";
import { AppShell } from "./layouts/AppShell";
import { DashboardPage } from "./pages/DashboardPage";
import { LoginPage } from "./pages/LoginPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { SenderDetailsPage } from "./pages/SenderDetailsPage";
import { SystemStatusPage } from "./pages/SystemStatusPage";
import { AlertsPage } from "./pages/AlertsPage";
import { EventsPage } from "./pages/EventsPage";
import { MonitoringPage } from "./pages/MonitoringPage";
import { Skeleton } from "./components/ui";

function ProtectedApp() {
  const { state } = useAuth();
  const location = useLocation();
  if (state === "loading") {
    return (
      <div className="grid min-h-[100dvh] place-items-center bg-[#0c0c0f]">
        <div className="w-72 space-y-3">
          <Skeleton className="mx-auto size-24 rounded-full" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-2/3 mx-auto" />
        </div>
      </div>
    );
  }
  if (state === "anonymous") {
    return location.pathname === "/login"
      ? <LoginPage />
      : <Navigate to="/login" replace />;
  }
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<DashboardPage />} />
        <Route path="senders" element={<DashboardPage />} />
        <Route path="senders/:sender" element={<SenderDetailsPage />} />
        <Route path="alerts" element={<AlertsPage />} />
        <Route path="events" element={<EventsPage />} />
        <Route path="monitoring" element={<MonitoringPage />} />
        <Route path="status" element={<SystemStatusPage />} />
        <Route path="login" element={<Navigate to="/" replace />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <ProtectedApp />
      </AuthProvider>
    </BrowserRouter>
  );
}
