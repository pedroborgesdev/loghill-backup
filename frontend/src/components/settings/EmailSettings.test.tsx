import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EmailSettings } from "./EmailSettings";

const settings = { provider: "outlook", enabled: true, configured: true, outlook: { tenant_id: "tenant", client_id: "client", client_secret_configured: true, sender_email: "logs@example.com", sender_name: "LogHill", managed_by_environment: false }, gmail: { host: "smtp.gmail.com", port: 587, username: "", password_configured: false, from: "", sender_name: "LogHill", managed_by_environment: false }, providers: [{ id: "outlook", enabled: true, available: true }, { id: "gmail", enabled: false, available: true }], updated_at: "2026-07-31T10:00:00Z", last_test_at: null };

describe("EmailSettings", () => {
  beforeEach(() => vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(settings), { status: 200 }))));
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it("shows Outlook and Gmail without exposing the stored secret", async () => {
    render(<EmailSettings />);
    expect(await screen.findByText("Microsoft 365 / O365")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Gmail/ })).toBeEnabled();
    const secret = screen.getByPlaceholderText("Credencial configurada — digite apenas para substituir");
    expect(secret).toHaveValue("");
    expect(screen.getByText(/O valor salvo nunca é exibido/)).toBeInTheDocument();
  });

  it("tests the connection without losing fields", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => String(input).includes("test-connection") ? new Response(JSON.stringify({ success: true, message: "Conexão validada." }), { status: 200 }) : new Response(JSON.stringify(settings), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    render(<EmailSettings />);
    await userEvent.click(await screen.findByRole("button", { name: "Testar conexão" }));
    expect(await screen.findByText("Conexão validada.")).toBeInTheDocument();
    expect(screen.getAllByDisplayValue("tenant")).toHaveLength(1);
  });

  it("allows selecting Gmail and shows the SMTP defaults", async () => {
    render(<EmailSettings />);
    await userEvent.click(await screen.findByRole("button", { name: /Gmail/ }));
    expect(screen.getByDisplayValue("smtp.gmail.com")).toBeInTheDocument();
    expect(screen.getByDisplayValue("587")).toBeInTheDocument();
    expect(screen.getAllByPlaceholderText("seuemail@gmail.com")).toHaveLength(2);
    expect(screen.getByPlaceholderText("Senha de aplicativo do Google")).toHaveValue("");
  });

  it("explains how to fix a forbidden Outlook send", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => String(input).includes("send-test")
      ? new Response(JSON.stringify({ error: { code: "OUTLOOK_SEND_FORBIDDEN", message: "O Outlook autenticou, mas não autorizou o envio. Conceda Mail.Send." } }), { status: 502 })
      : new Response(JSON.stringify(settings), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    render(<EmailSettings />);
    const recipient = await screen.findByPlaceholderText("Destinatário do teste");
    await userEvent.type(recipient, "dev@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Enviar e-mail de teste" }));
    expect(await screen.findByText("Permissão de envio necessária")).toBeInTheDocument();
    expect(screen.getByText(/Application permissions/)).toHaveTextContent("Mail.Send");
    expect(recipient).toHaveValue("dev@example.com");
  });
});
