import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ShellContext } from "../layouts/appShellContext";
import { DashboardPage } from "./DashboardPage";

const summary = { senders: { total: 0, never_connected: 0, online: 0, inactive: 0, expired: 0, revoked: 0 }, logs: { total: 0, last_24_hours: 0, errors_last_24_hours: 0, fatal_last_24_hours: 0 } };
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
});
