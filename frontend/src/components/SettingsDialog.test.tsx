import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { APIError, type Settings } from "../types/api";
import { SettingsButton, SettingsDialog } from "./SettingsDialog";

vi.mock("../api", () => ({
  api: {
    settings: vi.fn(),
    updateSettings: vi.fn(),
  },
}));

const currentSettings: Settings = {
  log_limit: { value: 10_000, unit: "lines" },
  inactive_preservation: { value: 2_000, unit: "lines" },
  updated_at: "2026-07-31T10:00:00-03:00",
};

const settingsMock = vi.mocked(api.settings);
const updateMock = vi.mocked(api.updateSettings);

function renderDialog(onClose = vi.fn(), returnFocus: HTMLButtonElement | null = null) {
  return {
    onClose,
    ...render(<SettingsDialog onClose={onClose} returnFocus={returnFocus} />),
  };
}

beforeEach(() => {
  settingsMock.mockResolvedValue(currentSettings);
  updateMock.mockResolvedValue({
    success: true,
    settings: {
      log_limit: { value: 9_000, unit: "lines" },
      inactive_preservation: { value: 2_000, unit: "lines" },
    },
    updated_at: "2026-07-31T10:05:00-03:00",
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("configurações do sistema", () => {
  it("exibe texto expandido e tooltip quando o botão está recolhido", async () => {
    const onOpen = vi.fn();
    const { rerender } = render(
      <SettingsButton collapsed={false} onOpen={onOpen} />,
    );
    expect(screen.getByText("Configurações")).toBeInTheDocument();

    rerender(<SettingsButton collapsed onOpen={onOpen} />);
    const button = screen.getByRole("button", { name: "Abrir configurações" });
    expect(screen.queryByText("Configurações")).not.toBeInTheDocument();
    await userEvent.hover(button);
    expect(screen.getByRole("tooltip")).toHaveTextContent("Configurações");
    await userEvent.click(button);
    expect(onOpen).toHaveBeenCalledWith(button);
  });

  it("mantém o skeleton até preencher os campos com a API", async () => {
    let resolveSettings: (value: Settings) => void = () => undefined;
    settingsMock.mockReturnValue(
      new Promise((resolve) => {
        resolveSettings = resolve;
      }),
    );
    renderDialog();

    expect(screen.getByRole("status", { name: "Carregando configurações" })).toBeInTheDocument();
    expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
    resolveSettings(currentSettings);

    expect(await screen.findByDisplayValue("10000")).toBeInTheDocument();
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Geral" })[0]);
    expect(screen.getByRole("heading", { name: "Geral" })).toBeInTheDocument();
    expect(screen.getByText("Limite máximo atual")).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Inatividade" })[0]);
    expect(screen.getByDisplayValue("2000")).toBeInTheDocument();
  });

  it("valida intervalo, relação, zero e troca de unidade sem converter", async () => {
    const user = userEvent.setup();
    renderDialog();
    let maximum = await screen.findByRole("spinbutton", {
      name: "Limite máximo de logs",
    });

    await user.click(
      screen.getByRole("button", { name: "Diminuir Limite máximo de logs" }),
    );
    expect(maximum).toHaveValue(9_999);
    await user.click(
      screen.getByRole("button", { name: "Aumentar Limite máximo de logs" }),
    );
    expect(maximum).toHaveValue(10_000);

    await user.clear(maximum);
    await user.type(maximum, "1000");
    await user.click(screen.getAllByRole("button", { name: "Inatividade" })[0]);
    expect(
      screen.getByText("A quantidade preservada não pode ser maior que o limite máximo."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Salvar alterações" })).toBeDisabled();

    await user.click(
      screen.getAllByRole("button", { name: "Armazenamento de logs" })[0],
    );
    maximum = screen.getByRole("spinbutton", { name: "Limite máximo de logs" });
    await user.clear(maximum);
    await user.type(maximum, "0");
    expect(screen.getByText(/Sem limite: o arquivo poderá continuar crescendo/)).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "Unidade de Limite máximo de logs" }),
    );
    await user.click(screen.getByRole("option", { name: "MB" }));
    expect(maximum).toHaveValue(0);
    expect(screen.getByText(/unidade foi alterada sem converter/i)).toBeInTheDocument();

    await user.clear(maximum);
    await user.type(maximum, "10001");
    expect(screen.getByText("Informe um valor entre 0 e 10.000.")).toBeInTheDocument();

    fireEvent.change(maximum, { target: { value: "1.5" } });
    expect(screen.getByText("O valor deve ser um número inteiro.")).toBeInTheDocument();

    await user.clear(maximum);
    await user.type(maximum, "-1");
    expect(screen.getByText("Informe um valor entre 0 e 10.000.")).toBeInTheDocument();
  });

  it("protege alterações, fecha com Escape e restaura o foco", async () => {
    const user = userEvent.setup();
    const trigger = document.createElement("button");
    trigger.textContent = "Origem";
    document.body.appendChild(trigger);
    trigger.focus();
    const onClose = vi.fn();
    function ClosingHarness() {
      const [open, setOpen] = useState(true);
      return open ? (
        <SettingsDialog
          returnFocus={trigger}
          onClose={() => {
            onClose();
            setOpen(false);
          }}
        />
      ) : null;
    }
    render(<ClosingHarness />);
    const maximum = await screen.findByRole("spinbutton", {
      name: "Limite máximo de logs",
    });
    await user.clear(maximum);
    await user.type(maximum, "9000");

    maximum.focus();
    await user.keyboard("{ArrowDown}{ArrowUp}");
    expect(maximum).toHaveValue(9_000);
    expect(screen.getByRole("button", { name: "Desfazer alterações" }).parentElement).toHaveClass(
      "whitespace-nowrap",
    );

    const saveButton = screen.getByRole("button", { name: "Salvar alterações" });
    saveButton.focus();
    await user.tab();
    expect(screen.getAllByRole("button", { name: "Fechar configurações" })[1]).toHaveFocus();

    await user.keyboard("{Escape}");
    expect(screen.getByRole("alertdialog", { name: "Descartar alterações?" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Continuar editando" }));
    expect(onClose).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    await user.click(screen.getByRole("button", { name: "Descartar" }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog", { name: "Configurações" })).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
    trigger.remove();
  });

  it("salva sem esconder os campos e confirma o sucesso", async () => {
    const user = userEvent.setup();
    renderDialog();
    const maximum = await screen.findByRole("spinbutton", {
      name: "Limite máximo de logs",
    });
    await user.clear(maximum);
    await user.type(maximum, "9000");
    const saveButton = screen.getByRole("button", { name: "Salvar alterações" });
    expect(saveButton).toHaveClass("text-zinc-100");
    expect(saveButton.parentElement).toHaveClass("sm:flex-nowrap", "sm:shrink-0");
    await user.click(saveButton);

    expect(updateMock).toHaveBeenCalledWith({
      log_limit: { value: 9_000, unit: "lines" },
      inactive_preservation: { value: 2_000, unit: "lines" },
    });
    expect(maximum).toBeInTheDocument();
    expect(await screen.findByText("Configurações salvas com sucesso.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Salvar alterações" })).toBeDisabled();
  });

  it("preserva o valor digitado quando a API rejeita o salvamento", async () => {
    const user = userEvent.setup();
    updateMock.mockRejectedValue(
      new APIError(422, "INVALID_SETTINGS", "Valor rejeitado pelo servidor.", "log_limit.value"),
    );
    renderDialog();
    const maximum = await screen.findByRole("spinbutton", {
      name: "Limite máximo de logs",
    });
    await user.clear(maximum);
    await user.type(maximum, "9000");
    await user.click(screen.getByRole("button", { name: "Salvar alterações" }));

    await waitFor(() => expect(screen.getAllByText("Valor rejeitado pelo servidor.")).not.toHaveLength(0));
    expect(maximum).toHaveValue(9000);
    expect(screen.getByRole("dialog", { name: "Configurações" })).toBeInTheDocument();
  });
});
