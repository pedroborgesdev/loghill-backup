import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ShellContext } from "../layouts/appShellContext";
import { AlertsPage } from "./AlertsPage";

const emptyResponse = {
  items: [], pagination: { page: 1, page_size: 20, total: 0, total_pages: 0 },
  summary: { total: 0, active: 0, recent_failures: 0 },
  email_provider: { provider: "outlook", enabled: false, configured: false, outlook: { tenant_id: "", client_id: "", client_secret_configured: false, sender_email: "", sender_name: "LogMate", managed_by_environment: false }, providers: [{ id: "outlook", enabled: false, available: true }, { id: "gmail", enabled: false, available: false }], updated_at: "2026-07-31T10:00:00Z", last_test_at: null },
};

describe("AlertsPage", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.includes("/api/v1/senders")) return new Response(JSON.stringify({ items: [], pagination: { page: 1, page_size: 20, total: 0, total_pages: 0 } }), { status: 200 });
      return new Response(JSON.stringify(emptyResponse), { status: 200 });
    }));
  });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it("keeps the empty state and opens the new alert dialog", async () => {
    const openEmailSettings = vi.fn();
    render(<MemoryRouter initialEntries={["/alerts"]}><ShellContext.Provider value={{ refreshToken: 0, refreshing: false, setRefreshing: vi.fn(), streamState: null, setStreamState: vi.fn(), openEmailSettings }}><AlertsPage /></ShellContext.Provider></MemoryRouter>);
    expect(await screen.findByText("No alerts configured")).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "New alert" })[0]);
    expect(screen.getByRole("dialog", { name: "New email alert" })).toBeInTheDocument();
    expect(screen.getByText("No email provider is configured and enabled.")).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Configure email" }).at(-1)!);
    expect(openEmailSettings).toHaveBeenCalled();
  });

  it("preserves the previous list during a refresh", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...emptyResponse, items: [{ id: "alert-1", name: "Critical errors", sender_id: "worker-1", sender_name: "worker", severities: ["ERROR"], recipients: ["dev@example.com"], provider: "outlook", enabled: false, created_at: "2026-07-31T10:00:00Z", updated_at: "2026-07-31T10:00:00Z", last_triggered_at: null, last_delivery_at: null, last_delivery_status: null, last_delivery_error: null, delivery_count: 0, failure_count: 0, test_delivery_count: 0 }], pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 }, summary: { total: 1, active: 0, recent_failures: 0 } }), { status: 200 }))
      .mockImplementation(() => new Promise(() => undefined));
    vi.stubGlobal("fetch", fetchMock);
    render(<MemoryRouter initialEntries={["/alerts"]}><ShellContext.Provider value={{ refreshToken: 0, refreshing: false, setRefreshing: vi.fn(), streamState: null, setStreamState: vi.fn(), openEmailSettings: vi.fn() }}><AlertsPage /></ShellContext.Provider></MemoryRouter>);
    expect(await screen.findAllByText("Critical errors")).not.toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.getAllByText("Critical errors")).not.toHaveLength(0);
  });
});
