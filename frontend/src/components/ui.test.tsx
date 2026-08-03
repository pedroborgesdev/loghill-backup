import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import {
  ConfirmDialog,
  IconButton,
  Input,
  SearchInput,
  StatusBadge,
} from "./ui";
import { restoreFocusWithoutTooltip } from "../utils/tooltipFocus";

describe("componentes de interface", () => {
  it("só marca o mínimo inválido depois de editar e sair do campo", async () => {
    function Fixture() {
      const [value, setValue] = useState("");
      return <Input aria-label="Nome" value={value} minLength={3} onChange={(event) => setValue(event.target.value)} />;
    }
    const user = userEvent.setup();
    render(<Fixture />);
    const input = screen.getByRole("textbox", { name: "Nome" });

    expect(input).not.toHaveAttribute("aria-invalid");
    await user.click(input);
    await user.tab();
    expect(input).not.toHaveAttribute("aria-invalid");

    await user.click(input);
    await user.type(input, "ab");
    expect(input).not.toHaveAttribute("aria-invalid");
    await user.tab();
    expect(input).toHaveAttribute("aria-invalid", "true");

    await user.click(input);
    await user.type(input, "c");
    expect(input).not.toHaveAttribute("aria-invalid");
  });

  it("exibe o status com texto acessível", () => {
    render(<StatusBadge status="online" />);
    expect(screen.getByText("Online")).toBeInTheDocument();
  });

  it("permite limpar uma busca sem alterar a largura do campo", async () => {
    const onChange = vi.fn();
    render(
      <SearchInput
        value="worker"
        onChange={onChange}
        placeholder="Buscar sender"
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Limpar busca" }));
    expect(onChange).toHaveBeenCalledWith("");
  });

  it("bloqueia a busca ao vivo e solicita uma ação antes da edição", async () => {
    const onChange = vi.fn();
    const onBlocked = vi.fn();
    render(
      <SearchInput
        value=""
        onChange={onChange}
        blocked
        onBlocked={onBlocked}
        placeholder="Buscar logs"
      />,
    );
    const input = screen.getByRole("textbox", { name: "Buscar logs" });
    await userEvent.click(input);
    await userEvent.type(input, "erro");
    expect(onBlocked).toHaveBeenCalled();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("confirma a pausa por um diálogo acessível", async () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        open
        title="Pause os logs"
        description="A lista precisa estar estável."
        confirmLabel="Pausar e continuar"
        onClose={vi.fn()}
        onConfirm={onConfirm}
      />,
    );
    expect(screen.getByRole("dialog", { name: "Pause os logs" })).toBeVisible();
    await userEvent.click(
      screen.getByRole("button", { name: "Pausar e continuar" }),
    );
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("fecha a descrição do botão no clique e limita-a à tela", async () => {
    const user = userEvent.setup();
    render(
      <IconButton label="Uma descrição de botão muito longa">
        A
      </IconButton>,
    );
    const button = screen.getByRole("button", {
      name: "Uma descrição de botão muito longa",
    });

    await user.hover(button);
    const tooltip = screen.getByRole("tooltip");
    expect(tooltip).toHaveClass("fixed");
    expect(tooltip).toHaveClass("z-[300]");
    expect(tooltip).toHaveClass("max-w-[calc(100vw-16px)]");

    await user.click(button);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });

  it("mantém somente uma descrição de botão aberta", () => {
    render(
      <>
        <IconButton label="Primeira descrição">A</IconButton>
        <IconButton label="Segunda descrição">B</IconButton>
      </>,
    );

    fireEvent.mouseEnter(
      screen.getByRole("button", { name: "Primeira descrição" }),
    );
    expect(screen.getByText("Primeira descrição")).toBeInTheDocument();

    fireEvent.focus(
      screen.getByRole("button", { name: "Segunda descrição" }),
    );
    expect(screen.queryByText("Primeira descrição")).not.toBeInTheDocument();
    expect(screen.getByText("Segunda descrição")).toBeInTheDocument();
  });

  it("não reabre a descrição ao restaurar o foco depois de um modal", () => {
    render(<IconButton label="Editar regra">A</IconButton>);
    const button = screen.getByRole("button", { name: "Editar regra" });
    fireEvent.focus(button);
    expect(screen.getByRole("tooltip")).toBeInTheDocument();
    fireEvent.blur(button);

    restoreFocusWithoutTooltip(button);

    expect(button).toHaveFocus();
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });
});
