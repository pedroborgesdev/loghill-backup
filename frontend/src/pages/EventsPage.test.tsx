import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { expect, it, vi } from "vitest";
import { ShellContext } from "../layouts/appShellContext";
import { EventsPage } from "./EventsPage";

it("lists events without replacing the page shell", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items: [{ id: "evt_1", name: "Processing completed", key: "processing_completed", sender_ids: ["worker"], action_type: "email", recipients: ["dev@example.com"], subject_template: "Finalizado", message_template: "Mensagem", enabled: true, created_at: "2026-07-31T17:00:00Z", updated_at: "2026-07-31T17:00:00Z", trigger_count: 2, delivery_count: 2, failure_count: 0, test_delivery_count: 0 }], pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 }, summary: { total: 1, active: 1, recent_triggered: 1, recent_failures: 0 }, email_provider: { provider: "outlook", enabled: true, configured: true, outlook: {} } }), { status: 200, headers: { "Content-Type": "application/json" } })));
  render(<MemoryRouter initialEntries={["/events"]}><ShellContext.Provider value={{ refreshToken: 0, refreshing: false, setRefreshing: vi.fn(), streamState: null, setStreamState: vi.fn(), openEmailSettings: vi.fn() }}><EventsPage /></ShellContext.Provider></MemoryRouter>);
  await waitFor(() => expect(screen.getAllByText("Processing completed").length).toBeGreaterThan(0));
  expect(screen.getAllByText("processing_completed").length).toBeGreaterThan(0);
  expect(screen.getByText(/matching usa a chave exata/i)).toBeInTheDocument();
  vi.unstubAllGlobals();
});
