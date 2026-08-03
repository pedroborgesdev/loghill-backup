import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { ShellContext } from "../layouts/appShellContext";
import { MonitoringPage } from "./MonitoringPage";

it("keeps the monitoring shell and presents the empty state", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items: [], pagination: { page: 1, page_size: 20, total: 0, total_pages: 1, returned: 0 }, summary: { total: 0, active: 0, recent_executions: 0, recent_failures: 0 } }), { status: 200, headers: { "Content-Type": "application/json" } })));
  render(<MemoryRouter initialEntries={["/monitoring"]}><ShellContext.Provider value={{ refreshToken: 0, refreshing: false, setRefreshing: vi.fn(), streamState: null, setStreamState: vi.fn(), openEmailSettings: vi.fn() }}><MonitoringPage /></ShellContext.Provider></MemoryRouter>);
  expect(screen.getByRole("heading", { name: "Monitoramento" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /nova regra/i })).toBeInTheDocument();
  await waitFor(() => expect(screen.getByText("Nenhuma regra de monitoramento")).toBeInTheDocument());
  await userEvent.click(screen.getByRole("button", { name: /nova regra/i }));
  expect(screen.getByRole("dialog", { name: "Nova regra de monitoramento" })).toBeInTheDocument();
  vi.unstubAllGlobals();
});
