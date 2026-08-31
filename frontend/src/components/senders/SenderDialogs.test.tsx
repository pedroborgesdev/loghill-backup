import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { normalizeSenderID } from "../../utils/senderID";
import { CreateSenderDialog } from "./SenderDialogs";

describe("cadastro administrativo de sender", () => {
  beforeEach(() => {
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn(async () => undefined) } });
  });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it("normaliza a prévia sem hash, sufixo ou timestamp", () => {
    expect(normalizeSenderID("  Automação   Financeira ")).toBe("financial-automation");
    expect(normalizeSenderID("Consulta PJe - TRF3")).toBe("consulta-pje-trf3");
    expect(normalizeSenderID("Cobrança / Santander")).toBe("cobranca-santander");
  });

  it("verifica disponibilidade e cria a configuração por sender_name", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
      const path = String(input);
      if (path.includes("check-id")) return new Response(JSON.stringify({ id: "financial-automation", available: true }), { status: 200 });
      expect(options?.method).toBe("POST");
      return new Response(JSON.stringify({ sender: { id: "financial-automation", name: "Financial Automation", description: "Boletos", status: "never_connected", created_at: "2026-07-31T15:00:00Z", updated_at: "2026-07-31T15:00:00Z", last_activity_at: null, last_healthcheck_at: null, inactive_at: null, compacted_at: null, expires_at: null, expired_at: null, log_line_count: 0, log_file_size: 0 }, credentials: { sender_key: "snd_12345678901234567890123456789012", displayed_once: true } }), { status: 201 });
    });
    vi.stubGlobal("fetch", fetchMock);
    const created = vi.fn();
    render(<CreateSenderDialog open onClose={vi.fn()} onCreated={created} />);
    await userEvent.type(screen.getByPlaceholderText("Financial Automation"), "Financial Automation");
    await userEvent.type(screen.getByPlaceholderText("Invoice and agreement processing"), "Boletos");
    expect(screen.getByText("financial-automation")).toBeInTheDocument();
    expect(await screen.findByText("Identifier available")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Create sender" }));
    expect(await screen.findByRole("dialog", { name: "Sender created" })).toBeInTheDocument();
    expect(screen.queryByText("snd_12345678901234567890123456789012")).not.toBeInTheDocument();
    expect(screen.getByText(/nenhuma chave precisa ser configurada/i)).toBeInTheDocument();
    expect(created).toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Copy environment configuration" }));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      expect.stringContaining("LOGHILL_SENDER_NAME=financial-automation"),
    ));
  });
});
