import { BrowserRouter, Route, Routes } from "react-router-dom";
import { AppShell } from "./layouts/AppShell";
import { DashboardPage } from "./pages/DashboardPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { SenderDetailsPage } from "./pages/SenderDetailsPage";
import { SystemStatusPage } from "./pages/SystemStatusPage";
import { AlertsPage } from "./pages/AlertsPage";
import { EventsPage } from "./pages/EventsPage";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="senders" element={<DashboardPage />} />
          <Route path="senders/:sender" element={<SenderDetailsPage />} />
          <Route path="alerts" element={<AlertsPage />} />
          <Route path="events" element={<EventsPage />} />
          <Route path="status" element={<SystemStatusPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
