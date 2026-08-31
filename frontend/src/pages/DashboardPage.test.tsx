// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ShellContext } from "../layouts/appShellContext";
import { DashboardPage } from "./DashboardPage";

const summary = { senders: { total: 4, never_connected: 0, online: 1, inactive: 2, expired: 1, revoked: 0 }, instances: { active: 3, inactive: 2 }, logs: { total: 0, last_24_hours: 0, errors_last_24_hours: 0, fatal_last_24_hours: 0 } };
const senderPage = { items: [], pagination: { page: 1, page_size: 25, total: 0, total_pages: 0 } };

describe("entrada reutilizável de cadastro", () => {
  beforeEach(() => vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(String(input).includes("summary") ? summary : senderPage), { status: 200 }))));
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  for (const path of ["/", "/senders"]) {
    it(`mostra Novo sender em ${path}`, async () => {
      render(<MemoryRouter initialEntries={[path]}><ShellContext.Provider value={{ refreshToken: 0, refreshing: false, setRefreshing: vi.fn(), streamState: null, setStreamState: vi.fn(), openEmailSettings: vi.fn() }}><DashboardPage /></ShellContext.Provider></MemoryRouter>);
      expect(await screen.findByRole("button", { name: "Novo sender" })).toBeInTheDocument();
    });
  }

  it("mostra contadores de instâncias em vez de status de senders", async () => {
    render(<MemoryRouter initialEntries={["/"]}><ShellContext.Provider value={{ refreshToken: 0, refreshing: false, setRefreshing: vi.fn(), streamState: null, setStreamState: vi.fn(), openEmailSettings: vi.fn() }}><DashboardPage /></ShellContext.Provider></MemoryRouter>);
    const active = await screen.findByText("Instâncias");
    const inactive = screen.getByText("Inativas");
    await waitFor(() => expect(active.parentElement?.parentElement).toHaveTextContent("3"));
    expect(inactive.parentElement?.parentElement).toHaveTextContent("2");
    expect(screen.queryByText("Expirados")).not.toBeInTheDocument();
  });
});
