// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { queryClient } from "../api/queryClient";
import { ShellContext } from "../layouts/appShellContext";
import { SenderDetailsPage } from "./SenderDetailsPage";

class EventSourceStub {
  static CLOSED = 2;
  readyState = 0;
  onerror: (() => void) | null = null;
  addEventListener() {}
  close() {}
}

describe("seleção de instances", () => {
  beforeEach(() => {
    queryClient.clear();
    vi.stubGlobal("EventSource", EventSourceStub);
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.endsWith("/instances?page_size=1000")) {
        return new Response(JSON.stringify({
          sender: "worker",
          items: [
            { id: "ins_11111111111111111111111111111111", created_at: "2026-08-28T12:00:00Z", last_activity_at: "2026-08-28T12:01:00Z", log_line_count: 10, log_file_size: 100, status: "online" },
            { id: "ins_22222222222222222222222222222222", created_at: "2026-08-27T12:00:00Z", last_activity_at: "2026-08-27T12:01:00Z", log_line_count: 5, log_file_size: 50, status: "inactive" },
          ],
          pagination: { page: 1, page_size: 1000, total: 2, total_pages: 1 },
        }), { status: 200 });
      }
      return new Response(JSON.stringify({
        id: "worker", name: "Worker", status: "online", created_at: "2026-08-28T12:00:00Z", updated_at: "2026-08-28T12:00:00Z",
        last_activity_at: "2026-08-28T12:01:00Z", last_healthcheck_at: null, inactive_at: null, compacted_at: null, expires_at: null, expired_at: null,
        log_line_count: 15, log_file_size: 150, instance_count: 2,
      }), { status: 200 });
    }));
  });

  afterEach(() => {
    cleanup();
    queryClient.clear();
    vi.unstubAllGlobals();
  });

  it("abre em Active e separa as instances inactive na segunda aba", async () => {
    render(
      <ShellContext.Provider value={{ refreshToken: 0, refreshing: false, setRefreshing: vi.fn(), streamState: null, setStreamState: vi.fn(), openEmailSettings: vi.fn() }}>
        <MemoryRouter initialEntries={["/senders/worker"]}>
          <Routes><Route path="/senders/:sender" element={<SenderDetailsPage />} /></Routes>
        </MemoryRouter>
      </ShellContext.Provider>,
    );

    const activeTab = await screen.findByRole("tab", { name: "Active 1" });
    expect(activeTab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("ins_11111111111111111111111111111111")).toBeInTheDocument();
    expect(screen.queryByText("ins_22222222222222222222222222222222")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "Inactive 1" }));
    expect(screen.getByText("ins_22222222222222222222222222222222")).toBeInTheDocument();
    expect(screen.queryByText("ins_11111111111111111111111111111111")).not.toBeInTheDocument();
  });

  it("atualiza a tela pelo botão da tabela e pelo refresh global", async () => {
    const shellValue = (refreshToken: number) => ({
      refreshToken,
      refreshing: false,
      setRefreshing: vi.fn(),
      streamState: null,
      setStreamState: vi.fn(),
      openEmailSettings: vi.fn(),
    });
    const page = (refreshToken: number) => (
      <ShellContext.Provider value={shellValue(refreshToken)}>
        <MemoryRouter initialEntries={["/senders/worker"]}>
          <Routes><Route path="/senders/:sender" element={<SenderDetailsPage />} /></Routes>
        </MemoryRouter>
      </ShellContext.Provider>
    );
    const view = render(page(0));

    await screen.findByRole("tab", { name: "Active 1" });
    expect(screen.getByRole("button", { name: "Refresh interval" })).toHaveTextContent("30 seconds");
    const callsBeforeTableRefresh = vi.mocked(fetch).mock.calls.length;

    await userEvent.click(screen.getByRole("button", { name: "Atualizar" }));
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.length).toBeGreaterThan(callsBeforeTableRefresh));

    const callsBeforeGlobalRefresh = vi.mocked(fetch).mock.calls.length;

    view.rerender(page(1));

    await waitFor(() => expect(vi.mocked(fetch).mock.calls.length).toBeGreaterThan(callsBeforeGlobalRefresh));
  });
});
