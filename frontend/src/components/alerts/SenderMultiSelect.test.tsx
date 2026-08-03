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

describe("alertas com múltiplos senders", () => {
  beforeEach(() => vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items: senders, pagination: { page: 1, page_size: 20, total: senders.length, total_pages: 1 } }), { status: 200 }))));
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it("usa checkboxes do tema, seleciona visíveis e preserva inativos", async () => {
    function Fixture() { const [value, setValue] = useState<SenderOption[]>([]); return <SenderMultiSelect value={value} onChange={setValue} />; }
    render(<Fixture />);
    expect(await screen.findByRole("checkbox", { name: "Selecionar Financeiro" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Selecionar Antigo" })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Selecionar Revogado" })).toBeDisabled();
    await userEvent.click(screen.getByRole("checkbox", { name: "Selecionar todos os resultados visíveis" }));
    expect(screen.getByText("2 selecionados")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Remover Financeiro" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByText("Senders inativos podem ser monitorados, mas talvez não enviem novos logs.")).toBeInTheDocument();
    await userEvent.type(screen.getByPlaceholderText("Buscar sender pelo nome"), "financeiro");
    expect(screen.getByText("2 selecionados")).toBeInTheDocument();
  });

  it("mostra tabela compacta e abre o drawer de detalhes", async () => {
    const alert: EmailAlert = { id: "alert-1", name: "Erros financeiros", sender_ids: ["financeiro", "acordos"], sender_names: ["Financeiro", "Acordos"], severities: ["ERROR", "FATAL"], recipients: ["dev@example.com", "ops@example.com"], provider: "outlook", enabled: true, created_at: "2026-07-31T10:00:00Z", updated_at: "2026-07-31T11:00:00Z", last_triggered_at: null, last_delivery_at: null, last_delivery_status: null, last_delivery_error: null, delivery_count: 0, failure_count: 0, test_delivery_count: 0 };
    const details = vi.fn();
    const props = { items: [alert], busyId: "", onDetails: details, onEdit: vi.fn(), onToggle: vi.fn(), onTest: vi.fn(), onDelete: vi.fn() };
    render(<CompactAlertTable {...props} />);
    expect(screen.getAllByText("Erros financeiros")).not.toHaveLength(0);
    expect(screen.queryByText("Criado:")).not.toBeInTheDocument();
    await userEvent.click(screen.getAllByText("Erros financeiros")[0]);
    expect(details).toHaveBeenCalledWith(alert);
    cleanup();
    render(<AlertDetailsDrawer alert={alert} onClose={vi.fn()} onEdit={vi.fn()} onTest={vi.fn()} />);
    expect(screen.getByRole("dialog", { name: "Erros financeiros" })).toBeInTheDocument();
    expect(screen.getByText("Financeiro")).toBeInTheDocument();
    expect(screen.getByText("Acordos")).toBeInTheDocument();
    expect(screen.getByText("Última alteração")).toBeInTheDocument();
  });
});
