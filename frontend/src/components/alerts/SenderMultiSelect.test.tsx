import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import type { Sender } from "../../types/api";
import { AlertDetailsDrawer } from "./AlertDetailsDrawer";
import { CompactAlertTable } from "./AlertTable";
import { SenderMultiSelect, type SenderOption } from "./SenderSelect";
import type { EmailAlert } from "../../types/alert";

const senders: Sender[] = [
  { id: "financeiro", name: "Financeiro", status: "online", created_at: "2026-07-31T10:00:00Z", updated_at: "2026-07-31T10:00:00Z", last_activity_at: "2026-07-31T10:00:00Z", last_healthcheck_at: null, inactive_at: null, compacted_at: null, expires_at: null, expired_at: null, log_line_count: 1, log_file_size: 10 },
  { id: "acordos", name: "Acordos", status: "inactive", created_at: "2026-07-31T10:00:00Z", updated_at: "2026-07-31T10:00:00Z", last_activity_at: "2026-07-31T10:00:00Z", last_healthcheck_at: null, inactive_at: null, compacted_at: null, expires_at: null, expired_at: null, log_line_count: 1, log_file_size: 10 },
  { id: "antigo", name: "Antigo", status: "expired", created_at: "2026-07-31T10:00:00Z", updated_at: "2026-07-31T10:00:00Z", last_activity_at: null, last_healthcheck_at: null, inactive_at: null, compacted_at: null, expires_at: null, expired_at: null, log_line_count: 0, log_file_size: 0 },
  { id: "revogado", name: "Revogado", status: "revoked", created_at: "2026-07-31T10:00:00Z", updated_at: "2026-07-31T10:00:00Z", last_activity_at: null, last_healthcheck_at: null, inactive_at: null, compacted_at: null, expires_at: null, expired_at: null, log_line_count: 0, log_file_size: 0 },
];

describe("alerts with multiple senders", () => {
  beforeEach(() => vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items: senders, pagination: { page: 1, page_size: 20, total: senders.length, total_pages: 1 } }), { status: 200 }))));
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it("uses themed checkboxes, selects visible senders, and preserves inactive ones", async () => {
    function Fixture() { const [value, setValue] = useState<SenderOption[]>([]); return <SenderMultiSelect value={value} onChange={setValue} />; }
    render(<Fixture />);
    expect(await screen.findByRole("checkbox", { name: "Select Financeiro" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Select Antigo" })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Select Revogado" })).toBeDisabled();
    await userEvent.click(screen.getByRole("checkbox", { name: "Select all visible results" }));
    expect(screen.getByText("2 selected")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Remove Financeiro" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByText("Inactive senders can be monitored, but may not send new logs.")).toBeInTheDocument();
    await userEvent.type(screen.getByPlaceholderText("Search sender by name"), "financeiro");
    expect(screen.getByText("2 selected")).toBeInTheDocument();
  });

  it("shows a compact table and opens the details drawer", async () => {
    const alert: EmailAlert = { id: "alert-1", name: "Errors financeiros", sender_ids: ["financeiro", "acordos"], sender_names: ["Financeiro", "Acordos"], severities: ["ERROR", "FATAL"], recipients: ["dev@example.com", "ops@example.com"], provider: "outlook", enabled: true, created_at: "2026-07-31T10:00:00Z", updated_at: "2026-07-31T11:00:00Z", last_triggered_at: null, last_delivery_at: null, last_delivery_status: null, last_delivery_error: null, delivery_count: 0, failure_count: 0, test_delivery_count: 0 };
    const details = vi.fn();
    const props = { items: [alert], busyId: "", onDetails: details, onEdit: vi.fn(), onToggle: vi.fn(), onTest: vi.fn(), onDelete: vi.fn() };
    render(<CompactAlertTable {...props} />);
    expect(screen.getAllByText("Errors financeiros")).not.toHaveLength(0);
    expect(screen.queryByText("Created:")).not.toBeInTheDocument();
    await userEvent.click(screen.getAllByText("Errors financeiros")[0]);
    expect(details).toHaveBeenCalledWith(alert);
    cleanup();
    render(<AlertDetailsDrawer alert={alert} onClose={vi.fn()} onEdit={vi.fn()} onTest={vi.fn()} />);
    expect(screen.getByRole("dialog", { name: "Errors financeiros" })).toBeInTheDocument();
    expect(screen.getByText("Financeiro")).toBeInTheDocument();
    expect(screen.getByText("Acordos")).toBeInTheDocument();
    expect(screen.getByText("Last updated")).toBeInTheDocument();
  });
});
